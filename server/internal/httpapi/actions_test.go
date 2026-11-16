package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// tokenCall sends a request as a script would, with extra headers, and reads
// its JSON answer.
func tokenCall(t *testing.T, s *testServer, secret, method, path, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, s.URL+"/api/v1"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// actionsWorld is a workspace with a token that can write and one that
// can only read.
type actionsWorld struct {
	s              *testServer
	owner          *client
	ws             string
	writer, reader string
}

func newActionsWorld(t *testing.T, d *db.DB) actionsWorld {
	t.Helper()
	return newActionsWorldWith(t, d, nil)
}

// newActionsWorldWith is newActionsWorld on a server whose Deps are
// changed first.
func newActionsWorldWith(t *testing.T, d *db.DB, change func(*Deps)) actionsWorld {
	t.Helper()
	s := newServerWith(t, d, change)
	owner := s.browser(t)
	s.setup(t, owner)
	_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
	w := actionsWorld{s: s, owner: owner, ws: ws["id"].(string)}
	for _, tok := range []struct {
		into  *string
		write bool
	}{{&w.writer, true}, {&w.reader, false}} {
		resp, made := owner.do("POST", "/me/tokens", map[string]any{"name": "Agent", "canWrite": tok.write, "workspaceId": w.ws})
		if resp.StatusCode != 201 {
			t.Fatalf("token = %d %v", resp.StatusCode, made)
		}
		*tok.into = made["secret"].(string)
	}
	return w
}

func TestABatchOfWritesIsUndoneAsOne(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		tasks := "/workspaces/" + w.ws + "/tasks"
		code, task := tokenCall(t, w.s, w.writer, "POST", tasks, `{"title":"Fence"}`, nil)
		if code != 201 {
			t.Fatalf("create = %d %v", code, task)
		}
		id := task["id"].(string)
		batch := map[string]string{"Mila-Batch": "run-7"}
		if code, out := tokenCall(t, w.s, w.writer, "PATCH", tasks+"/"+id, `{"title":"Fence, mended"}`, batch); code != 200 {
			t.Fatalf("patch = %d %v", code, out)
		}
		code, made := tokenCall(t, w.s, w.writer, "POST", tasks, `{"title":"Posts"}`, batch)
		if code != 201 {
			t.Fatalf("batched create = %d %v", code, made)
		}
		if code, out := tokenCall(t, w.s, w.writer, "PATCH", tasks+"/"+id, `{"title":"x"}`, map[string]string{"Mila-Batch": "no spaces"}); code != 400 {
			t.Fatalf("a bad batch = %d %v", code, out)
		}

		undo := "/workspaces/" + w.ws + "/actions/undo-batch"
		if code, out := tokenCall(t, w.s, w.reader, "POST", undo, `{"batch":"run-7"}`, nil); code != 403 {
			t.Fatalf("undo with a read-only token = %d %v", code, out)
		}
		code, out := tokenCall(t, w.s, w.writer, "POST", undo, `{"batch":"run-7"}`, nil)
		if code != 200 || out["undone"] != float64(2) {
			t.Fatalf("undo = %d %v", code, out)
		}
		if _, got := tokenCall(t, w.s, w.writer, "GET", tasks+"/"+id, "", nil); got["title"] != "Fence" {
			t.Errorf("title after undo = %v", got["title"])
		}
		if _, got := tokenCall(t, w.s, w.writer, "GET", tasks+"/"+made["id"].(string), "", nil); got["deleted_at"] == nil {
			t.Error("the task the batch made is still there")
		}
		if code, out := tokenCall(t, w.s, w.writer, "POST", undo, `{"batch":"run-8"}`, nil); code != 404 {
			t.Fatalf("undo of no batch = %d %v", code, out)
		}
	})
}

// act sends an action with the writer's token.
func (w actionsWorld) act(t *testing.T, name, body string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	return tokenCall(t, w.s, w.writer, "POST", "/workspaces/"+w.ws+"/actions/"+name, body, headers)
}

func (w actionsWorld) find(t *testing.T, secret, query string) (int, map[string]any) {
	t.Helper()
	return tokenCall(t, w.s, secret, "GET", "/workspaces/"+w.ws+"/actions/find-tasks?"+query, "", nil)
}

func named(v any, key string) string {
	m, _ := v.(map[string]any)
	s, _ := m[key].(string)
	return s
}

func TestActionsDoWhatTheAppDoes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		rows := "/workspaces/" + w.ws

		// A project comes with the app's five statuses.
		code, made := w.act(t, "create-project", `{"name":"Garden"}`, nil)
		if code != 200 || len(made["statuses"].([]any)) != 5 || named(made["project"], "name") != "Garden" {
			t.Fatalf("create-project = %d %v", code, made)
		}
		project := named(made["project"], "id")
		if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/labels", `{"name":"Wood"}`, nil); code != 201 {
			t.Fatalf("label = %d %v", code, out)
		}

		// Names, not IDs, and everything in one go; a retry makes nothing twice.
		add := `{"title":"Mend the fence","project":"garden","labels":["wood"],"assignees":["me"],"checklist":["Posts","Wire"],"due_date":"2026-11-13","priority":"high","notes":"## Plan\nold plan\n## Later\nkeep this"}`
		key := map[string]string{"Idempotency-Key": "add-1"}
		code, out := w.act(t, "add-task", add, key)
		task, _ := out["task"].(map[string]any)
		if code != 200 || named(task["status"], "name") != "To do" || named(task["project"], "name") != "Garden" ||
			len(task["labels"].([]any)) != 1 || len(task["assignees"].([]any)) != 1 || out["batch"] == "" {
			t.Fatalf("add-task = %d %v", code, out)
		}
		id := task["id"].(string)
		if _, again := w.act(t, "add-task", add, key); named(again["task"], "id") != id {
			t.Fatalf("a retry made another task: %v", again["task"])
		}
		if _, items := tokenCall(t, w.s, w.writer, "GET", rows+"/checklist-items?task_id="+id, "", nil); len(items["items"].([]any)) != 2 {
			t.Fatalf("checklist after a retry = %v", items["items"])
		}

		// What it can't find, or can't tell apart, it says, and makes nothing.
		if code, out := w.act(t, "add-task", `{"title":"x","project":"Orchard"}`, nil); code != 400 || !strings.Contains(out["message"].(string), `"Garden"`) {
			t.Fatalf("unknown project = %d %v", code, out)
		}
		w.act(t, "create-project", `{"name":"garden"}`, nil)
		if code, out := w.act(t, "add-task", `{"title":"x","project":"Garden"}`, nil); code != 409 || !strings.Contains(out["message"].(string), project) {
			t.Fatalf("two of a name = %d %v", code, out)
		}
		if code, out := w.act(t, "add-task", `{"title":"x","due":"friday"}`, nil); code != 400 || !strings.Contains(out["message"].(string), `"due"`) {
			t.Fatalf("an unknown field = %d %v", code, out)
		}

		// Statuses kept in step with completion.
		if code, out := w.act(t, "set-status", `{"task":"`+id+`","status":"in progress"}`, nil); code != 200 || named(named2(out, "task", "status"), "name") != "In progress" {
			t.Fatalf("set-status = %d %v", code, out)
		}
		code, out = w.act(t, "complete-task", `{"task":"`+id+`"}`, nil)
		if code != 200 || named(named2(out, "task", "status"), "category") != "done" || named(out["task"], "completed_at") == "" {
			t.Fatalf("complete = %d %v", code, out)
		}
		if code, out := w.act(t, "reopen-task", `{"task":"`+id+`"}`, nil); code != 200 || named(named2(out, "task", "status"), "name") != "To do" {
			t.Fatalf("reopen = %d %v", code, out)
		}

		// Moving takes the subtasks along, to the Inbox and back.
		code, sub := w.act(t, "add-task", `{"title":"Buy wire","parent":"`+id+`"}`, nil)
		if code != 200 || named(sub["task"], "project_id") != project {
			t.Fatalf("subtask = %d %v", code, sub)
		}
		subID := named(sub["task"], "id")
		if code, out := w.act(t, "move-task", `{"task":"`+id+`","project":null}`, nil); code != 200 || out["task"].(map[string]any)["project"] != nil {
			t.Fatalf("to the Inbox = %d %v", code, out)
		}
		if _, got := tokenCall(t, w.s, w.writer, "GET", rows+"/tasks/"+subID, "", nil); got["project_id"] != nil {
			t.Fatalf("the subtask stayed: %v", got["project_id"])
		}
		if code, out := w.act(t, "move-task", `{"task":"`+subID+`","project":"`+project+`"}`, nil); code != 400 {
			t.Fatalf("a subtask on its own = %d %v", code, out)
		}
		if code, out := w.act(t, "move-task", `{"task":"`+id+`","project":"`+project+`"}`, nil); code != 200 || named(named2(out, "task", "status"), "name") != "To do" {
			t.Fatalf("back = %d %v", code, out)
		}

		// Notes: one section replaced, the rest left; then added to.
		code, out = w.act(t, "replace-section", `{"task":"`+id+`","heading":"## plan","text":"new plan"}`, nil)
		notes := named(out["row"], "notes")
		if code != 200 || !strings.Contains(notes, "new plan") || strings.Contains(notes, "old plan") || !strings.Contains(notes, "keep this") {
			t.Fatalf("replace-section = %d %q", code, notes)
		}
		if code, out := w.act(t, "append-notes", `{"task":"`+id+`","text":"Done by Friday."}`, nil); code != 200 || !strings.HasSuffix(strings.TrimSpace(named(out["row"], "notes")), "Done by Friday.") {
			t.Fatalf("append-notes = %d %v", code, out)
		}

		// A status deleted: its tasks go to another, the same kind first.
		code, out = w.act(t, "delete-status", `{"status":"To do","project":"`+project+`"}`, nil)
		if code != 200 || out["moved"] != float64(2) || named(out["to"], "name") != "Backlog" {
			t.Fatalf("delete-status = %d %v", code, out)
		}

		// Finding, by a read-only token too.
		if code, out := w.find(t, w.reader, "list=upcoming&due_from=2026-11-01&due_to=2026-11-30"); code != 200 || len(out["items"].([]any)) != 1 || out["today"] == "" {
			t.Fatalf("find upcoming = %d %v", code, out)
		}
		if code, out := w.find(t, w.reader, "words=WIRE"); code != 200 || len(out["items"].([]any)) != 1 {
			t.Fatalf("find words = %d %v", code, out)
		}
		if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/saved-views", `{"name":"Urgent","type":"list","config":{"filter":{"priorities":["high"]}}}`, nil); code != 201 {
			t.Fatalf("view = %d %v", code, out)
		}
		if code, out := w.find(t, w.reader, "view=urgent"); code != 200 || len(out["items"].([]any)) != 1 {
			t.Fatalf("find view = %d %v", code, out)
		}
		if code, out := w.find(t, w.reader, "list=nowhere"); code != 400 {
			t.Fatalf("a bad list = %d %v", code, out)
		}
		if code, out := tokenCall(t, w.s, w.reader, "POST", rows+"/actions/add-task", `{"title":"No"}`, nil); code != 403 {
			t.Fatalf("add-task with a read-only token = %d %v", code, out)
		}
	})
}

// named2 is a field's field.
func named2(v map[string]any, a, b string) any {
	m, _ := v[a].(map[string]any)
	return m[b]
}

func TestFinishingARepeatingTaskMakesTheNextAndUndoesAsOne(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		w.act(t, "create-project", `{"name":"Home"}`, nil)
		code, out := w.act(t, "add-task", `{"title":"Water the plants","project":"Home","due_date":"2026-11-02"}`, nil)
		if code != 200 {
			t.Fatalf("add = %d %v", code, out)
		}
		id := named(out["task"], "id")
		if code, out := tokenCall(t, w.s, w.writer, "PATCH", "/workspaces/"+w.ws+"/tasks/"+id, `{"recurrence":"FREQ=WEEKLY","recurrence_mode":"schedule"}`, nil); code != 200 {
			t.Fatalf("repeat = %d %v", code, out)
		}
		code, out = w.act(t, "complete-task", `{"task":"`+id+`"}`, nil)
		next, _ := out["next"].(map[string]any)
		if code != 200 || next == nil || named(next["status"], "name") != "To do" || next["due_date"] == "2026-11-02" {
			t.Fatalf("complete = %d %v", code, out)
		}
		code, undone := tokenCall(t, w.s, w.writer, "POST", "/workspaces/"+w.ws+"/actions/undo-batch", `{"batch":"`+out["batch"].(string)+`"}`, nil)
		if code != 200 {
			t.Fatalf("undo = %d %v", code, undone)
		}
		if _, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/tasks/"+id, "", nil); got["completed_at"] != nil {
			t.Errorf("still completed after undo: %v", got["completed_at"])
		}
		if _, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/tasks/"+next["id"].(string), "", nil); got["deleted_at"] == nil {
			t.Error("the next task stayed after undo")
		}
	})
}

// Someone assigned must be able to reach the task, and that is known
// before anything is written: a guest only in the projects shared with them.
func TestAddingATaskAssignsOnlyWhoCanReachIt(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		gus := w.s.join(t, w.owner, w.ws, "gus@example.com", "Gus", "guest")
		_, shared := w.act(t, "create-project", `{"name":"Shared"}`, nil)
		w.act(t, "create-project", `{"name":"Private"}`, nil)
		path := "/workspaces/" + w.ws + "/projects/" + named(shared["project"], "id") + "/guests/" + gus.userID()
		if resp, _ := w.owner.do("PUT", path, nil); resp.StatusCode != 204 {
			t.Fatalf("share = %d", resp.StatusCode)
		}
		for _, project := range []string{`"project":"Private",`, ""} {
			code, out := w.act(t, "add-task", `{"title":"Secret",`+project+`"assignees":["gus"]}`, nil)
			if code != 400 || !strings.Contains(out["message"].(string), "can't reach") {
				t.Fatalf("%s= %d %v", project, code, out)
			}
		}
		if _, found := w.find(t, w.reader, "words=secret&state=all"); len(found["items"].([]any)) != 0 {
			t.Fatalf("a refused task was made: %v", found["items"])
		}
		code, out := w.act(t, "add-task", `{"title":"Paint","project":"Shared","assignees":["Gus"]}`, nil)
		if code != 200 || len(out["task"].(map[string]any)["assignees"].([]any)) != 1 {
			t.Fatalf("shared = %d %v", code, out)
		}
		paint := named(out["task"], "id")

		// The same rules for assigning an existing task, and taking it off.
		code, private := w.act(t, "add-task", `{"title":"Accounts","project":"Private"}`, nil)
		if code != 200 {
			t.Fatalf("private = %d %v", code, private)
		}
		if code, out := w.act(t, "assign-task", `{"task":"`+named(private["task"], "id")+`","people":["gus"]}`, nil); code != 400 || !strings.Contains(out["message"].(string), "can't reach") {
			t.Fatalf("assign out of reach = %d %v", code, out)
		}
		if code, out := w.act(t, "assign-task", `{"task":"`+paint+`","people":["Gus","me"]}`, nil); code != 200 || len(out["task"].(map[string]any)["assignees"].([]any)) != 2 {
			t.Fatalf("assign = %d %v", code, out)
		}
		if code, out := w.act(t, "assign-task", `{"task":"`+paint+`","people":["gus"],"remove":true}`, nil); code != 200 || len(out["task"].(map[string]any)["assignees"].([]any)) != 1 {
			t.Fatalf("unassign = %d %v", code, out)
		}
		if code, out := tokenCall(t, w.s, w.writer, "POST", "/workspaces/"+w.ws+"/labels", `{"name":"Outdoor"}`, nil); code != 201 {
			t.Fatalf("label = %d %v", code, out)
		}
		if code, out := w.act(t, "label-task", `{"task":"`+paint+`","labels":["outdoor"]}`, nil); code != 200 || len(out["task"].(map[string]any)["labels"].([]any)) != 1 {
			t.Fatalf("label-task = %d %v", code, out)
		}
		if code, out := w.act(t, "label-task", `{"task":"`+paint+`","labels":["Outdoor"],"remove":true}`, nil); code != 200 || len(out["task"].(map[string]any)["labels"].([]any)) != 0 {
			t.Fatalf("unlabel = %d %v", code, out)
		}
	})
}

func TestFindingAQueueOfWork(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		w.s.join(t, w.owner, w.ws, "gus@example.com", "Gus", "member")
		w.act(t, "create-project", `{"name":"Garden"}`, nil)
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		add(`{"title":"Mine","project":"Garden","assignees":["me"]}`)
		add(`{"title":"Gus's","project":"Garden","assignees":["Gus"]}`)
		started := add(`{"title":"Started","project":"Garden","assignees":["me"]}`)
		inbox := add(`{"title":"Inbox","assignees":["me"]}`)

		// A status by its kind: the project's first of that kind.
		code, out := w.act(t, "set-status", `{"task":"`+started+`","kind":"in-progress"}`, nil)
		if code != 200 || named(named2(out, "task", "status"), "name") != "In progress" {
			t.Fatalf("set-status by kind = %d %v", code, out)
		}
		for body, want := range map[string]string{
			`{"task":"` + started + `"}`:                               "Give a status",
			`{"task":"` + started + `","status":"Done","kind":"done"}`: "Give a status",
			`{"task":"` + started + `","kind":"review"}`:               "A kind is",
			`{"task":"` + inbox + `","kind":"done"}`:                   "Inbox has no status",
		} {
			if code, out := w.act(t, "set-status", body, nil); code != 400 || !strings.Contains(out["message"].(string), want) {
				t.Errorf("set-status %s = %d %v", body, code, out)
			}
		}

		titles := func(query string) []string {
			t.Helper()
			code, out := w.find(t, w.reader, query)
			if code != 200 {
				t.Fatalf("find %s = %d %v", query, code, out)
			}
			var got []string
			for _, item := range out["items"].([]any) {
				got = append(got, named(item, "title"))
			}
			slices.Sort(got)
			return got
		}
		for query, want := range map[string][]string{
			"assignee=me":                          {"Inbox", "Mine", "Started"},
			"assignee=gus":                         {"Gus's"},
			"assignee=me&kind=backlog,todo":        {"Inbox", "Mine"},
			"kind=in-progress":                     {"Started"},
			"assignee=me&kind=todo&project=Garden": {"Mine"},
		} {
			if got := titles(query); !slices.Equal(got, want) {
				t.Errorf("find %s = %v, want %v", query, got, want)
			}
		}
		if code, out := w.find(t, w.reader, "kind=review"); code != 400 || out["field"] != "kind" {
			t.Errorf("an unknown kind = %d %v", code, out)
		}
		if code, out := w.find(t, w.reader, "assignee=nobody"); code != 400 || out["field"] != "assignee" {
			t.Errorf("an unknown person = %d %v", code, out)
		}
	})
}
