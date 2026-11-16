package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// request makes one request with headers and returns the raw answer.
func (c *client) request(method, path string, body any, headers ...string) (*http.Response, []byte) {
	c.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		r = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

// row reads a row's fields as the JSON each was sent as.
func row(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("not a row: %s", raw)
	}
	return out
}

func str(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// page lists a collection and returns the IDs in the page, and next.
func (c *client) page(path string) ([]string, string) {
	c.t.Helper()
	resp, raw := c.request("GET", path, nil)
	if resp.StatusCode != 200 {
		c.t.Fatalf("GET %s = %d %s", path, resp.StatusCode, raw)
	}
	var p struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Next string `json:"next"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		c.t.Fatal(err)
	}
	ids := make([]string, len(p.Items))
	for i, it := range p.Items {
		ids[i] = it.ID
	}
	return ids, p.Next
}

// sampleJSON is a kJSON value whose key order and spacing a round trip
// must keep.
const sampleJSON = `{"b":1,"a":[1,2]}`

// sample is a value a create may give a field, or false to leave it out.
func sample(t *testing.T, e syncer.EntityInfo, f syncer.FieldInfo, made map[string]string, me string) (any, bool) {
	t.Helper()
	if f.ServerSet || f.Name == "deleted_at" || f.Name == "sort_key" || f.Name == "parent_id" {
		return nil, false
	}
	switch f.Kind {
	case "string":
		switch f.Name {
		case "series_id", "source_id":
			return uuid.NewString(), true
		case "recurrence":
			return "FREQ=DAILY", true
		}
		return "Sample", true
	case "text":
		return "Some words", true
	case "enum":
		return f.Enum[len(f.Enum)-1], true
	case "time":
		return "2026-10-28T09:30:00Z", true
	case "ref":
		return made[f.Ref], true
	case "bool":
		return true, true
	case "int":
		return 30, true
	case "date":
		return "2026-11-02", true
	case "clock":
		return "14:30", true
	case "self":
		return nil, false // filled in, or a shared view
	case "member":
		return me, true
	case "json":
		// Each in the shape its field takes, with keys out of order.
		switch f.Name {
		case "options":
			return json.RawMessage(`[{"name":"Sample","id":"o1","color":"red"}]`), true
		case "config":
			return json.RawMessage(`{"b":1,"filter":{"text":"x","labels":[]}}`), true
		case "rule":
			return json.RawMessage(`{"then":[{"do":"priority","value":"high"}],"when":{"on":"created"}}`), true
		case "data": // a template's snapshot
			return json.RawMessage(`{"pages":[{"body":"Sample","id":"p1","parent":null,"title":"Sample"}],"tasks":[{"title":"Sample","id":"t1"}],"v":1}`), true
		case "body": // a workspace theme's document
			return json.RawMessage(`{"name":"Sample","mila":"theme","version":1,"id":"` + uuid.NewString() + `","base":"calm"}`), true
		case "value": // of a user field, the type made last
			return json.RawMessage(strconv.Quote(me)), true
		}
		return json.RawMessage(sampleJSON), true
	}
	t.Fatalf("no sample for %s.%s of kind %q", e.Name, f.Name, f.Kind)
	return nil, false
}

// changed is another value for a field, for a PATCH.
func changed(t *testing.T, f syncer.FieldInfo) any {
	t.Helper()
	switch f.Kind {
	case "string", "text":
		return "Changed"
	case "enum":
		return f.Enum[0]
	case "time":
		return "2026-10-29T10:00:00Z"
	case "date":
		return "2026-11-03"
	case "int":
		return 45
	case "bool":
		return false
	case "json":
		switch f.Name {
		case "options":
			return json.RawMessage(`[{"id":"o2","name":"Changed","color":"blue"}]`)
		case "config":
			return json.RawMessage(`{"filter":{"showDone":true}}`)
		case "rule":
			return json.RawMessage(`{"then":[{"do":"priority","value":"low"}],"when":{"on":"completed"}}`)
		case "value":
			return json.RawMessage(`"changed"`)
		}
		return json.RawMessage(`{"changed":true}`)
	}
	t.Fatalf("no change for %s of kind %q", f.Name, f.Kind)
	return nil
}

// patchField is the field a round trip changes: the row's name, or else
// the first that a PATCH may set.
func patchField(e syncer.EntityInfo) (syncer.FieldInfo, bool) {
	var first *syncer.FieldInfo
	for _, f := range e.Fields {
		if f.Immutable || f.ServerSet || f.Name == "deleted_at" || f.Kind == "ref" {
			continue
		}
		if slices.Contains([]string{"title", "name", "text", "body", "filename", "day", "remind_at", "value"}, f.Name) {
			return f, true
		}
		if first == nil {
			first = &f
		}
	}
	if first == nil {
		return syncer.FieldInfo{}, false
	}
	return *first, true
}

func (s *testServer) upload(t *testing.T, c *client, ws, task string) string {
	t.Helper()
	return s.uploadTo(t, c, "/workspaces/"+ws+"/tasks/"+task+"/attachments")
}

// uploadTo uploads a small text file to an upload endpoint and returns
// the new row's ID.
func (s *testServer) uploadTo(t *testing.T, c *client, path string) string {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("--x\r\nContent-Disposition: form-data; name=\"file\"; filename=\"notes.txt\"\r\nContent-Type: text/plain\r\n\r\nhello\r\n--x--\r\n")
	req, _ := http.NewRequest("POST", c.base+path, &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("Origin", c.origin)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 201 {
		t.Fatalf("upload = %d %v", resp.StatusCode, out)
	}
	return out["id"].(string)
}

func TestRowsRoundTrip(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		c := s.browser(t)
		s.setup(t, c)
		_, list := c.doList("GET", "/workspaces")
		ws := list[0]["id"].(string)
		me := c.userID()
		base := "/workspaces/" + ws + "/"
		made := map[string]string{}
		bodies := map[string]map[string]any{}

		for _, e := range syncer.Describe() {
			coll := base + e.Collection
			var id string
			body := map[string]any{}
			if e.ServerCreated {
				if resp, _ := c.request("POST", coll, map[string]any{"task_id": made["task"], "page_id": made["page"]}); resp.StatusCode == 201 || resp.StatusCode == 200 {
					t.Fatalf("POST %s = %d; it is made only by an upload", coll, resp.StatusCode)
				}
				if e.Name == "page_attachment" {
					id = s.uploadTo(t, c, base+"pages/"+made["page"]+"/attachments")
				} else {
					id = s.upload(t, c, ws, made["task"])
				}
			} else {
				for _, f := range e.Fields {
					if v, ok := sample(t, e, f, made, me); ok {
						body[f.Name] = v
					}
				}
				if e.Name == "page" {
					// The first page of a wiki is its main page.
					body["id"] = syncer.MainPageID(made["project"])
				}
				if e.Name == "task_dependency" {
					// A task waits for another, never itself.
					resp, raw := c.request("POST", base+"tasks", map[string]any{"title": "Blocker", "project_id": made["project"]})
					if resp.StatusCode != 201 {
						t.Fatalf("POST tasks = %d %s", resp.StatusCode, raw)
					}
					body["blocker_id"] = str(row(t, raw)["id"])
				}
				resp, raw := c.request("POST", coll, body)
				if resp.StatusCode != 201 || resp.Header.Get("ETag") == "" {
					t.Fatalf("POST %s %v = %d %s", coll, body, resp.StatusCode, raw)
				}
				got := row(t, raw)
				id = str(got["id"])
				if loc := resp.Header.Get("Location"); loc != "/api/v1"+coll+"/"+id {
					t.Fatalf("Location = %q", loc)
				}
				if len(e.Pair) == 2 {
					// Adding the pair again is the same row.
					if resp, raw := c.request("POST", coll, body); resp.StatusCode != 200 || str(row(t, raw)["id"]) != id {
						t.Fatalf("POST %s again = %d %s", coll, resp.StatusCode, raw)
					}
				}
			}
			made[e.Name] = id
			bodies[e.Name] = body

			// GET is the body with the defaults filled in, JSON kept exact.
			resp, raw := c.request("GET", coll+"/"+id, nil)
			if resp.StatusCode != 200 || resp.Header.Get("ETag") == "" {
				t.Fatalf("GET %s = %d %s", id, resp.StatusCode, raw)
			}
			got := row(t, raw)
			if len(got) != len(e.Fields)+3 || str(got["created_at"]) == "" || str(got["updated_at"]) == "" {
				t.Fatalf("%s row = %s", e.Name, raw)
			}
			for _, f := range e.Fields {
				want, given := body[f.Name]
				switch {
				case given:
					wantRaw, _ := json.Marshal(want)
					if string(got[f.Name]) != string(wantRaw) {
						t.Fatalf("%s.%s = %s, sent %s", e.Name, f.Name, got[f.Name], wantRaw)
					}
				case f.Name == "sort_key":
					if str(got[f.Name]) == "" {
						t.Fatalf("%s got no sort_key", e.Name)
					}
				case f.Kind == "self" && (f.Required || f.Name == "created_by"):
					if str(got[f.Name]) != me {
						t.Fatalf("%s.%s = %s, want the caller", e.Name, f.Name, got[f.Name])
					}
				case f.HasDefault && !e.ServerCreated:
					wantRaw, _ := json.Marshal(f.Default)
					if string(got[f.Name]) != string(wantRaw) {
						t.Fatalf("%s.%s = %s, want the default %s", e.Name, f.Name, got[f.Name], wantRaw)
					}
				}
			}
			etag := resp.Header.Get("ETag")
			for _, header := range []string{etag, "W/" + etag, `"other", ` + etag, "*"} {
				if resp, _ := c.request("GET", coll+"/"+id, nil, "If-None-Match", header); resp.StatusCode != 304 {
					t.Fatalf("GET with If-None-Match %s = %d", header, resp.StatusCode)
				}
			}
			if resp, _ := c.request("GET", coll+"/"+id, nil, "If-None-Match", `"other"`); resp.StatusCode != 200 {
				t.Fatalf("GET with another If-None-Match = %d", resp.StatusCode)
			}

			// Every filter finds it.
			for _, name := range e.Filters {
				value := string(got[name])
				if value == "null" {
					value = ""
				} else if strings.HasPrefix(value, `"`) {
					value = str(got[name])
				}
				ids, _ := c.page(coll + "?" + name + "=" + url.QueryEscape(value))
				if !slices.Contains(ids, id) {
					t.Fatalf("%s?%s=%s does not list %s", coll, name, value, id)
				}
			}

			// PATCH changes it, and its ETag.
			// A label or assignee on a task is only added and removed.
			onlyAdded := len(e.Pair) == 2 && e.Name != "my_day_entry" && e.Name != "custom_field_value"
			if f, ok := patchField(e); ok && !onlyAdded {
				resp, raw := c.request("PATCH", coll+"/"+id, map[string]any{f.Name: changed(t, f)}, "If-Match", etag)
				if resp.StatusCode != 200 || resp.Header.Get("ETag") == etag {
					t.Fatalf("PATCH %s.%s = %d %s", e.Name, f.Name, resp.StatusCode, raw)
				}
				wantRaw, _ := json.Marshal(changed(t, f))
				if got := row(t, raw)[f.Name]; string(got) != string(wantRaw) {
					t.Fatalf("PATCH %s.%s gave %s, want %s", e.Name, f.Name, got, wantRaw)
				}
				// A weak tag never matches If-Match; * matches any version.
				if resp, _ := c.request("PATCH", coll+"/"+id, map[string]any{f.Name: changed(t, f)}, "If-Match", "W/"+resp.Header.Get("ETag")); resp.StatusCode != 412 {
					t.Fatalf("PATCH with a weak If-Match = %d", resp.StatusCode)
				}
				if resp, _ := c.request("PATCH", coll+"/"+id, map[string]any{f.Name: changed(t, f)}, "If-Match", "*"); resp.StatusCode != 200 {
					t.Fatalf("PATCH with If-Match * = %d", resp.StatusCode)
				}
				// The old version no longer matches.
				if resp, raw := c.request("PATCH", coll+"/"+id, map[string]any{f.Name: changed(t, f)}, "If-Match", etag); resp.StatusCode != 412 || !strings.Contains(string(raw), "precondition_failed") {
					t.Fatalf("PATCH with a stale If-Match = %d %s", resp.StatusCode, raw)
				}
			} else if resp, _ := c.request("PATCH", coll+"/"+id, map[string]any{"deleted_at": nil}); resp.StatusCode == 200 {
				t.Fatalf("PATCH %s answered; a pair is added and removed", e.Name)
			}
		}

		// Field filters and empty values on tasks.
		tasks := base + "tasks"
		if ids, _ := c.page(tasks + "?due_date="); slices.Contains(ids, made["task"]) {
			t.Fatal("a task with a due date is listed as having none")
		}
		if ids, _ := c.page(tasks + "?project_id="); slices.Contains(ids, made["task"]) {
			t.Fatal("a task in a project is listed in the inbox")
		}
		for _, q := range []string{"?title=Sample", "?priority=dire", "?due_date=tomorrow", "?limit=0", "?limit=501", "?include_deleted=maybe", "?after=a%20b", "?status_id=a&status_id=b"} {
			if resp, raw := c.request("GET", tasks+q, nil); resp.StatusCode != 400 || !strings.Contains(string(raw), `"invalid"`) {
				t.Fatalf("GET tasks%s = %d %s", q, resp.StatusCode, raw)
			}
		}
		// Pages of one.
		for range 2 {
			if resp, raw := c.request("POST", tasks, map[string]any{"title": "More"}); resp.StatusCode != 201 {
				t.Fatalf("POST task = %d %s", resp.StatusCode, raw)
			}
		}
		all, next := c.page(tasks)
		if len(all) != 4 || next != "" { // the sample, the blocker a dependency waits for, and these two
			t.Fatalf("tasks = %v, next %q", all, next)
		}
		var paged []string
		for after := ""; ; {
			ids, next := c.page(tasks + "?limit=1&after=" + after)
			paged = append(paged, ids...)
			if next == "" {
				break
			}
			after = next
		}
		if !slices.Equal(paged, all) {
			t.Fatalf("paged %v, all %v", paged, all)
		}

		// Bodies the API refuses.
		for _, b := range []any{"[1]", "not json", map[string]any{"title": ""}, map[string]any{"title": "x", "created_at": "2026-10-28T00:00:00Z"},
			map[string]any{"title": "x", "nonsense": 1}, map[string]any{"title": "x", "sort_key": "a00"}, map[string]any{"title": "x", "id": "not an id"}} {
			if resp, raw := c.request("POST", tasks, b); resp.StatusCode != 400 {
				t.Fatalf("POST tasks %v = %d %s", b, resp.StatusCode, raw)
			}
		}
		resp, raw := c.request("POST", tasks, map[string]any{"title": ""})
		if !strings.Contains(string(raw), `"field":"title"`) {
			t.Fatalf("an invalid title names no field: %d %s", resp.StatusCode, raw)
		}
		if resp, raw := c.request("POST", tasks, `{"title":"`+strings.Repeat("x", maxRowBody)+`"}`); resp.StatusCode != 400 || !strings.Contains(string(raw), "1 MiB") {
			t.Fatalf("an oversized body = %d %.200s", resp.StatusCode, raw)
		}
		for _, b := range []any{map[string]any{}, map[string]any{"id": uuid.NewString()}, map[string]any{"updated_at": nil}} {
			if resp, raw := c.request("PATCH", tasks+"/"+made["task"], b); resp.StatusCode != 400 {
				t.Fatalf("PATCH task %v = %d %s", b, resp.StatusCode, raw)
			}
		}
		if resp, raw := c.request("POST", tasks, map[string]any{"id": made["task"], "title": "Taken"}); resp.StatusCode != 409 {
			t.Fatalf("POST a taken ID = %d %s", resp.StatusCode, raw)
		}
		if resp, _ := c.request("GET", tasks+"/"+uuid.NewString(), nil); resp.StatusCode != 404 {
			t.Fatalf("GET a missing task = %d", resp.StatusCode)
		}
		// A retry with the same key makes one task.
		first, raw1 := c.request("POST", tasks, map[string]any{"title": "Once"}, "Idempotency-Key", "k1")
		again, raw2 := c.request("POST", tasks, map[string]any{"title": "Once"}, "Idempotency-Key", "k1")
		if first.StatusCode != 201 || again.StatusCode != 200 || str(row(t, raw1)["id"]) != str(row(t, raw2)["id"]) {
			t.Fatalf("idempotent create = %d then %d: %s %s", first.StatusCode, again.StatusCode, raw1, raw2)
		}

		// Delete, in the trash, and back; children first, so they still list.
		entities := syncer.Describe()
		for i := len(entities) - 1; i >= 0; i-- {
			e := entities[i]
			coll, id := base+e.Collection, made[e.Name]
			if e.Name == "page" {
				// The page made is its wiki's main page, which goes only
				// with its project.
				if resp, _ := c.request("DELETE", coll+"/"+id, nil); resp.StatusCode != 400 {
					t.Fatalf("DELETE a main page = %d", resp.StatusCode)
				}
				continue
			}
			if resp, raw := c.request("DELETE", coll+"/"+id, nil); resp.StatusCode != 204 {
				t.Fatalf("DELETE %s = %d %s", e.Name, resp.StatusCode, raw)
			}
			if resp, _ := c.request("DELETE", coll+"/"+id, nil); resp.StatusCode != 204 {
				t.Fatalf("DELETE %s again = %d", e.Name, resp.StatusCode)
			}
			resp, raw := c.request("GET", coll+"/"+id, nil)
			if resp.StatusCode != 200 || string(row(t, raw)["deleted_at"]) == "null" {
				t.Fatalf("GET deleted %s = %d %s", e.Name, resp.StatusCode, raw)
			}
			if ids, _ := c.page(coll); slices.Contains(ids, id) {
				t.Fatalf("deleted %s still listed", e.Name)
			}
			if ids, _ := c.page(coll + "?include_deleted=true"); !slices.Contains(ids, id) {
				t.Fatalf("deleted %s not listed with include_deleted", e.Name)
			}
			restore, method := map[string]any{"deleted_at": nil}, "PATCH"
			if len(e.Pair) == 2 && e.Name != "my_day_entry" && e.Name != "custom_field_value" {
				restore, method = bodies[e.Name], "POST"
				if resp, raw := c.request(method, coll, restore); resp.StatusCode != 200 || string(row(t, raw)["deleted_at"]) != "null" {
					t.Fatalf("POST deleted %s again = %d %s", e.Name, resp.StatusCode, raw)
				}
				continue
			}
			if resp, raw := c.request(method, coll+"/"+id, restore); resp.StatusCode != 200 || string(row(t, raw)["deleted_at"]) != "null" {
				t.Fatalf("restore %s = %d %s", e.Name, resp.StatusCode, raw)
			}
		}

		// The file itself moved under /file, where a page's files are too.
		for _, name := range []string{"attachment", "page_attachment"} {
			resp, raw := c.request("GET", base+"attachments/"+made[name]+"/file", nil)
			if resp.StatusCode != 200 || string(raw) != "hello" {
				t.Fatalf("download %s = %d %s", name, resp.StatusCode, raw)
			}
		}
	})
}

func TestRowsPermissions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, team := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		ws := team["id"].(string)
		base := "/workspaces/" + ws + "/"
		people := map[string]*client{"owner": owner}
		for _, role := range []string{"admin", "member", "guest", "lonely guest"} {
			c := s.browser(t)
			email := strings.ReplaceAll(role, " ", "-") + "@example.com"
			invite := role
			if role == "lonely guest" {
				invite = "guest"
			}
			if resp, body := owner.do("POST", "/workspaces/"+ws+"/invites", map[string]string{"email": email, "role": invite}); resp.StatusCode != 201 {
				t.Fatalf("invite %s = %d %v", role, resp.StatusCode, body)
			}
			if resp, body := c.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": role}); resp.StatusCode != 200 {
				t.Fatalf("%s joins = %d %v", role, resp.StatusCode, body)
			}
			people[role] = c
		}
		// Someone with an account here, in another workspace.
		outsider := s.browser(t)
		_, other := owner.do("POST", "/workspaces", map[string]string{"name": "Other"})
		owner.do("POST", "/workspaces/"+other["id"].(string)+"/invites", map[string]string{"email": "outsider@example.com", "role": "member"})
		if resp, body := outsider.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Outsider"}); resp.StatusCode != 200 {
			t.Fatalf("outsider joins elsewhere = %d %v", resp.StatusCode, body)
		}
		people["outsider"] = outsider
		people["signed out"] = s.browser(t)

		post := func(c *client, path string, body map[string]any) map[string]json.RawMessage {
			t.Helper()
			resp, raw := c.request("POST", base+path, body)
			if resp.StatusCode != 201 {
				t.Fatalf("POST %s = %d %s", path, resp.StatusCode, raw)
			}
			return row(t, raw)
		}
		shared := str(post(owner, "projects", map[string]any{"name": "Shared"})["id"])
		todo := str(post(owner, "statuses", map[string]any{"project_id": shared, "name": "To do", "category": "todo"})["id"])
		private := str(post(owner, "projects", map[string]any{"name": "Private"})["id"])
		if resp, _ := owner.do("PUT", "/workspaces/"+ws+"/projects/"+shared+"/guests/"+people["guest"].userID(), nil); resp.StatusCode != 204 {
			t.Fatalf("share = %d", resp.StatusCode)
		}
		task := str(post(owner, "tasks", map[string]any{"title": "Shared task", "project_id": shared, "status_id": todo})["id"])
		hidden := str(post(owner, "tasks", map[string]any{"title": "Private task", "project_id": private})["id"])

		// What each person may do to a task in the shared project.
		cases := []struct {
			who                                string
			list, get, create, update, deleted int
		}{
			{"owner", 200, 200, 201, 200, 204},
			{"admin", 200, 200, 201, 200, 204},
			{"member", 200, 200, 201, 200, 204},
			{"guest", 200, 200, 201, 200, 204},
			{"lonely guest", 200, 404, 403, 404, 404},
			{"outsider", 404, 404, 404, 404, 404},
			{"signed out", 401, 401, 401, 401, 401},
		}
		for _, c := range cases {
			p := people[c.who]
			got := []int{}
			resp, _ := p.request("GET", base+"tasks", nil)
			got = append(got, resp.StatusCode)
			resp, _ = p.request("GET", base+"tasks/"+task, nil)
			got = append(got, resp.StatusCode)
			resp, _ = p.request("POST", base+"tasks", map[string]any{"title": "By " + c.who, "project_id": shared, "status_id": todo})
			got = append(got, resp.StatusCode)
			resp, _ = p.request("PATCH", base+"tasks/"+task, map[string]any{"notes": "By " + c.who})
			got = append(got, resp.StatusCode)
			resp, _ = p.request("DELETE", base+"tasks/"+task, nil)
			got = append(got, resp.StatusCode)
			if want := []int{c.list, c.get, c.create, c.update, c.deleted}; !slices.Equal(got, want) {
				t.Errorf("%s: list, get, create, update, delete = %v, want %v", c.who, got, want)
			}
			owner.request("PATCH", base+"tasks/"+task, map[string]any{"deleted_at": nil})
		}

		// A guest lists only their project's tasks, in full pages, and
		// cannot reach the rest or what only members change.
		guest := people["guest"]
		ids, _ := guest.page(base + "tasks")
		for _, id := range ids {
			_, raw := owner.request("GET", base+"tasks/"+id, nil)
			if str(row(t, raw)["project_id"]) != shared {
				t.Fatalf("the guest lists %s", raw)
			}
		}
		if len(ids) < 3 {
			t.Fatalf("the guest lists %d tasks", len(ids))
		}
		if one, next := guest.page(base + "tasks?limit=1"); len(one) != 1 || next == "" {
			t.Fatalf("a guest's page of one = %v, next %q", one, next)
		}
		if resp, _ := guest.request("PATCH", base+"tasks/"+hidden, map[string]any{"title": "Mine"}); resp.StatusCode != 404 {
			t.Fatalf("guest PATCH of a task they cannot see = %d", resp.StatusCode)
		}
		if resp, _ := guest.request("POST", base+"labels", map[string]any{"name": "Mine"}); resp.StatusCode != 403 {
			t.Fatalf("guest POST label = %d", resp.StatusCode)
		}
		if resp, _ := guest.request("PATCH", base+"tasks/"+task, map[string]any{"project_id": private, "status_id": nil}); resp.StatusCode != 403 {
			t.Fatalf("guest moving a task out of sight = %d", resp.StatusCode)
		}

		// Comments are their author's; admins may only delete them.
		comment := str(post(people["member"], "comments", map[string]any{"task_id": task, "body": "Mine"})["id"])
		if resp, _ := people["admin"].request("PATCH", base+"comments/"+comment, map[string]any{"body": "Edited"}); resp.StatusCode != 403 {
			t.Fatalf("admin editing a member's comment = %d", resp.StatusCode)
		}
		if resp, _ := people["member"].request("PATCH", base+"comments/"+comment, map[string]any{"body": "Edited"}); resp.StatusCode != 200 {
			t.Fatalf("author editing their comment = %d", resp.StatusCode)
		}
		if resp, raw := people["member"].request("POST", base+"comments", map[string]any{"task_id": task, "body": "As someone", "author_id": owner.userID()}); resp.StatusCode != 403 {
			t.Fatalf("a comment as someone else = %d %s", resp.StatusCode, raw)
		}
		if resp, _ := people["admin"].request("DELETE", base+"comments/"+comment, nil); resp.StatusCode != 204 {
			t.Fatalf("admin deleting a member's comment = %d", resp.StatusCode)
		}

		// Someone's reminder is theirs alone.
		reminder := str(post(people["member"], "reminders", map[string]any{"task_id": task, "remind_at": "2026-12-01T09:00:00Z"})["id"])
		if resp, _ := owner.request("GET", base+"reminders/"+reminder, nil); resp.StatusCode != 404 {
			t.Fatalf("someone else's reminder = %d", resp.StatusCode)
		}
		if ids, _ := owner.page(base + "reminders"); len(ids) != 0 {
			t.Fatalf("the owner lists someone else's reminders: %v", ids)
		}
		if ids, _ := people["member"].page(base + "reminders"); len(ids) != 1 {
			t.Fatalf("the member lists %v", ids)
		}

		// A script with the cookie must say where it comes from.
		bad := *owner
		bad.origin = "https://evil.example"
		if resp, raw := bad.request("POST", base+"tasks", map[string]any{"title": "x"}); resp.StatusCode != 403 || !strings.Contains(string(raw), "bad_origin") {
			t.Fatalf("POST from another origin = %d %s", resp.StatusCode, raw)
		}
		bad.origin = ""
		if resp, raw := bad.request("PATCH", base+"tasks/"+task, map[string]any{"title": "x"}); resp.StatusCode != 403 || !strings.Contains(string(raw), "bad_origin") {
			t.Fatalf("PATCH with no Origin = %d %s", resp.StatusCode, raw)
		}
		if resp, _ := bad.request("GET", base+"tasks/"+task, nil); resp.StatusCode != 200 {
			t.Fatalf("GET with no Origin = %d", resp.StatusCode)
		}
	})
}

func TestRowsReachDevices(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, team := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		ws := team["id"].(string)
		base := "/workspaces/" + ws + "/"

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		header := http.Header{"Origin": {owner.origin}}
		for _, c := range owner.http.Jar.Cookies(mustURL(s.URL)) {
			header.Add("Cookie", c.Name+"="+c.Value)
		}
		sock, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(s.URL, "http")+"/api/v1/sync/ws", &websocket.DialOptions{HTTPHeader: header})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sock.CloseNow() }()
		read := func() map[string]any {
			t.Helper()
			_, raw, err := sock.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return m
		}
		read() // hello

		resp, raw := owner.request("POST", base+"tasks", map[string]any{"title": "From a script"})
		if resp.StatusCode != 201 {
			t.Fatalf("POST = %d %s", resp.StatusCode, raw)
		}
		task := str(row(t, raw)["id"])
		if m := read(); m["type"] != "poke" || m["workspace"] != ws {
			t.Fatalf("after a REST write the socket got %v", m)
		}
		_, snap := owner.do("GET", base+"sync/bootstrap?"+syncQ, nil)
		_, changes := owner.do("GET", fmt.Sprintf("%ssync/changes?"+syncQ+"&after=0&epoch=%s", base, url.QueryEscape(snap["epoch"].(string))), nil)
		got := changes["changes"].([]any)
		c := got[len(got)-1].(map[string]any)
		fields, clocks := c["fields"].(map[string]any), c["clocks"].(map[string]any)
		// Every field but started_at, which only the server sets, once the
		// task first goes to a status of kind in-progress.
		if c["id"] != task || len(fields) != 18 {
			t.Fatalf("the change = %v", c)
		}
		for name, clock := range clocks {
			if !strings.HasSuffix(clock.(string), "-api") {
				t.Fatalf("%s stamped %v", name, clock)
			}
		}

		// A comment on an assigned task brings its notifications.
		member := s.browser(t)
		owner.do("POST", "/workspaces/"+ws+"/invites", map[string]string{"email": "dana@example.com", "role": "member"})
		member.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Dana"})
		if resp, raw := owner.request("POST", base+"task-assignees", map[string]any{"task_id": task, "user_id": member.userID()}); resp.StatusCode != 201 {
			t.Fatalf("assign = %d %s", resp.StatusCode, raw)
		}
		jobsBefore := countRows(t, s, "jobs")
		if resp, raw := owner.request("POST", base+"comments", map[string]any{"task_id": task, "body": "Over to you"}); resp.StatusCode != 201 {
			t.Fatalf("comment = %d %s", resp.StatusCode, raw)
		}
		if countRows(t, s, "jobs") <= jobsBefore {
			t.Fatal("a comment through REST queued no notification")
		}

		// Moving a task out of a guest's project resets their copy.
		guest := s.browser(t)
		owner.do("POST", "/workspaces/"+ws+"/invites", map[string]string{"email": "gus@example.com", "role": "guest"})
		guest.do("POST", "/invites/accept", map[string]string{"token": s.mail.token(t), "name": "Gus"})
		_, p := owner.request("POST", base+"projects", map[string]any{"name": "Shared"})
		shared := str(row(t, p)["id"])
		_, p = owner.request("POST", base+"projects", map[string]any{"name": "Elsewhere"})
		elsewhere := str(row(t, p)["id"])
		owner.do("PUT", "/workspaces/"+ws+"/projects/"+shared+"/guests/"+guest.userID(), nil)
		owner.request("PATCH", base+"tasks/"+task, map[string]any{"project_id": shared})
		_, gsnap := guest.do("GET", base+"sync/bootstrap?"+syncQ, nil)
		epoch, seq := url.QueryEscape(gsnap["epoch"].(string)), gsnap["seq"].(float64)
		if resp, raw := owner.request("PATCH", base+"tasks/"+task, map[string]any{"project_id": elsewhere}); resp.StatusCode != 200 {
			t.Fatalf("move = %d %s", resp.StatusCode, raw)
		}
		if resp, _ := guest.do("GET", fmt.Sprintf("%ssync/changes?"+syncQ+"&after=%d&epoch=%s", base, int64(seq), epoch), nil); resp.StatusCode != 410 {
			t.Fatalf("the guest's pull after a move out of sight = %d, want 410", resp.StatusCode)
		}
	})
}

func countRows(t *testing.T, s *testServer, table string) int {
	t.Helper()
	var n int
	if err := s.st.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRowsRateLimit(t *testing.T) {
	s := newServerWith(t, dbtest.Open(t), func(d *Deps) { d.APIWritesPerMinute = 1 })
	c := s.browser(t)
	s.setup(t, c)
	_, list := c.doList("GET", "/workspaces")
	tasks := "/workspaces/" + list[0]["id"].(string) + "/tasks"
	for i := range 2 {
		if resp, raw := c.request("POST", tasks, map[string]any{"title": "Fine"}); resp.StatusCode != 201 {
			t.Fatalf("write %d = %d %s", i, resp.StatusCode, raw)
		}
	}
	resp, raw := c.request("POST", tasks, map[string]any{"title": "Too many"})
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "60" || !strings.Contains(string(raw), "rate_limited") {
		t.Fatalf("third write = %d %v %s", resp.StatusCode, resp.Header, raw)
	}
	// Reads have their own allowance, and sync is not limited.
	if resp, _ := c.request("GET", tasks, nil); resp.StatusCode != 200 {
		t.Fatalf("read after writes = %d", resp.StatusCode)
	}
}

func TestAPIDocsRoute(t *testing.T) {
	h := newTest(t, Deps{})
	rec := get(h, "/api/docs")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="app"`) || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("/api/docs = %d %v", rec.Code, rec.Header())
	}
	rec = get(h, "/api/v1/openapi.json")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" || !bytes.Equal(rec.Body.Bytes(), openAPI) {
		t.Fatalf("/api/v1/openapi.json = %d %v", rec.Code, rec.Header())
	}
	// Compressed once, ahead, for browsers that take gzip.
	req := httptest.NewRequest("GET", "/api/v1/openapi.json", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("compressed spec headers = %v", rec.Header())
	}
	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(gz)
	if err != nil || !bytes.Equal(plain, openAPI) {
		t.Fatalf("the compressed spec is not the spec: %v", err)
	}
	var spec struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(openAPI, &spec); err != nil || spec.Paths["/workspaces/{workspaceId}/tasks/{taskId}"] == nil {
		t.Fatalf("the embedded spec has no task endpoints: %v", err)
	}
	if rec := get(h, "/api/docsx"); rec.Code != 404 {
		t.Fatalf("/api/docsx = %d", rec.Code)
	}
}
