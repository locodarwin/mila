package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// find-tasks asks the database what a task's own row answers, and checks
// the rest a page at a time in its order: the answers must be what
// checking every task would give.
func TestFindingTasksInTheDatabase(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		rows := "/workspaces/" + w.ws
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		titles := func(query string) ([]string, bool) {
			t.Helper()
			code, out := w.find(t, w.reader, query)
			if code != 200 {
				t.Fatalf("find %s = %d %v", query, code, out)
			}
			var got []string
			for _, item := range out["items"].([]any) {
				got = append(got, named(item, "title"))
			}
			more, _ := out["more"].(bool)
			return got, more
		}
		expect := func(query string, want ...string) {
			t.Helper()
			if got, _ := titles(query); !slices.Equal(got, want) {
				t.Errorf("find %s = %q, want %q", query, got, want)
			}
		}

		// Words: what LIKE takes as a pattern is matched as written, and
		// letters only Go lowercases to ASCII still match.
		for _, title := range []string{"50% off", "500 nails", "snake_case name", "Here comes", `back\slash`,
			"Temperature in K", "TAXİ rank", "Élan vital"} {
			add(fmt.Sprintf(`{"title":%q,"due_date":"2030-01-01"}`, title))
		}
		expect("words=50%25", "50% off")
		expect("words=e_c", "snake_case name")
		expect(`words=k%5Cs`, `back\slash`)
		expect("words=in+k", "Temperature in K")
		expect("words=taxi", "TAXİ rank")
		expect("words=%C3%A9lan", "Élan vital")
		expect("words=%C3%89LAN", "Élan vital")

		// The order: soonest due first, a time before none, then the title
		// without regard to case, and undated last.
		w.act(t, "create-project", `{"name":"Orchard"}`, nil)
		for _, task := range []string{
			`{"title":"Undated","project":"Orchard"}`,
			`{"title":"Cherry","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"Banana","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"apple","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"Zest at nine","project":"Orchard","due_date":"2031-05-02","due_time":"09:00"}`,
			`{"title":"First","project":"Orchard","due_date":"2031-05-01"}`,
		} {
			add(task)
		}
		expect("project=Orchard", "First", "Zest at nine", "apple", "Banana", "Cherry", "Undated")
		// Cut inside the day: the database's order must be the same.
		if got, more := titles("project=Orchard&limit=3"); !slices.Equal(got, []string{"First", "Zest at nine", "apple"}) || !more {
			t.Errorf("limit 3 = %q, more %v", got, more)
		}
		if _, more := titles("project=Orchard&limit=6"); more {
			t.Error("limit 6 of 6 says there are more")
		}

		// An archived project's tasks show only when it is asked for; a
		// trashed one's not at all.
		_, made := w.act(t, "create-project", `{"name":"Old"}`, nil)
		old := named(made["project"], "id")
		add(`{"title":"Old work","project":"Old"}`)
		_, made = w.act(t, "create-project", `{"name":"Binned"}`, nil)
		binned := named(made["project"], "id")
		add(`{"title":"Binned work","project":"Binned"}`)
		if code, out := tokenCall(t, w.s, w.writer, "PATCH", rows+"/projects/"+old, `{"archived_at":"`+time.Now().UTC().Format(time.RFC3339)+`"}`, nil); code != 200 {
			t.Fatalf("archive = %d %v", code, out)
		}
		if code, out := tokenCall(t, w.s, w.writer, "DELETE", rows+"/projects/"+binned, "", nil); code >= 300 {
			t.Fatalf("trash = %d %v", code, out)
		}
		expect("words=work")
		expect("words=work&project=Old", "Old work")

		// A view that few tasks match is read a page at a time until it has
		// one more than asked for.
		w.act(t, "create-project", `{"name":"Field"}`, nil)
		for i := range 30 {
			priority := ""
			if i == 3 || i == 28 {
				priority = `,"priority":"high"`
			}
			add(fmt.Sprintf(`{"title":"Row %02d","project":"Field","due_date":"2032-01-%02d"%s}`, i, i+1, priority))
		}
		if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/saved-views", `{"name":"Hot","type":"list","config":{"filter":{"priorities":["high"]}}}`, nil); code != 201 {
			t.Fatalf("view = %d %v", code, out)
		}
		if got, more := titles("view=Hot&project=Field&limit=1"); !slices.Equal(got, []string{"Row 03"}) || !more {
			t.Errorf("view, limit 1 = %q, more %v", got, more)
		}
		if got, more := titles("view=Hot&project=Field&limit=5"); !slices.Equal(got, []string{"Row 03", "Row 28"}) || more {
			t.Errorf("view, limit 5 = %q, more %v", got, more)
		}

		// Blocked by a task in another project, which is read for it.
		w.act(t, "create-project", `{"name":"House"}`, nil)
		waits := add(`{"title":"Plant","project":"Field"}`)
		blocker := add(`{"title":"Dig","project":"House"}`)
		if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/task-dependencies", `{"task_id":"`+waits+`","blocker_id":"`+blocker+`"}`, nil); code != 201 {
			t.Fatalf("dependency = %d %v", code, out)
		}
		if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/saved-views", `{"name":"Stuck","type":"list","config":{"filter":{"dependencies":["blocked"]}}}`, nil); code != 201 {
			t.Fatalf("view = %d %v", code, out)
		}
		expect("view=Stuck&project=Field", "Plant")
		if code, out := w.act(t, "complete-task", `{"task":"`+blocker+`"}`, nil); code != 200 {
			t.Fatalf("complete = %d %v", code, out)
		}
		expect("view=Stuck&project=Field")

		// My Day: picked today or before and not done, or done today.
		_, found := w.find(t, w.reader, "limit=1")
		today, _ := time.Parse(time.DateOnly, found["today"].(string))
		me := w.owner.userID()
		for title, day := range map[string]time.Time{"Today's": today, "Yesterday's": today.AddDate(0, 0, -1), "Tomorrow's": today.AddDate(0, 0, 1), "Done before": today.AddDate(0, 0, -2)} {
			id := add(`{"title":"` + title + `"}`)
			entry := fmt.Sprintf(`{"id":%q,"task_id":%q,"user_id":%q,"day":%q,"sort_key":"a0"}`, syncer.PairID(id, me), id, me, day.Format(time.DateOnly))
			if code, out := tokenCall(t, w.s, w.writer, "POST", rows+"/my-day-entries", entry, nil); code != 201 {
				t.Fatalf("my day = %d %v", code, out)
			}
			if title == "Done before" {
				w.act(t, "complete-task", `{"task":"`+id+`"}`, nil)
			}
		}
		got, _ := titles("list=my-day&state=all")
		slices.Sort(got)
		if want := []string{"Today's", "Yesterday's"}; !slices.Equal(got, want) {
			t.Errorf("my day = %q, want %q", got, want)
		}
	})
}

// find-tasks reads in its order from where it is told: next goes on from
// where an answer stopped, at the limit or at the bound on what one search
// may read, and a kind is asked of the database.
func TestFindTasksGoesOnWhereItStopped(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorldWith(t, d, func(deps *Deps) { deps.Actions.BoundSearches(4, time.Hour) })
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		w.act(t, "create-project", `{"name":"Orchard"}`, nil)
		ids := map[string]string{}
		for _, task := range []string{
			`{"title":"Undated b","project":"Orchard"}`,
			`{"title":"undated A","project":"Orchard"}`,
			`{"title":"Éclair","project":"Orchard"}`,
			`{"title":"Cherry","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"Banana","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"apple","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"Apple","project":"Orchard","due_date":"2031-05-02"}`,
			`{"title":"Zest at nine","project":"Orchard","due_date":"2031-05-02","due_time":"09:00"}`,
			`{"title":"Café first","project":"Orchard","due_date":"2031-05-01"}`,
			`{"title":"Late café","project":"Orchard","due_date":"2031-06-01"}`,
		} {
			var in struct{ Title string }
			_ = json.Unmarshal([]byte(task), &in)
			ids[in.Title] = add(task)
		}
		// follow asks a query and then each next, as a caller going on.
		follow := func(query string) (got []string, answers []map[string]any) {
			t.Helper()
			after := ""
			for range 50 {
				q := query
				if after != "" {
					q += "&after=" + url.QueryEscape(after)
				}
				code, out := w.find(t, w.reader, q)
				if code != 200 {
					t.Fatalf("find %s = %d %v", q, code, out)
				}
				answers = append(answers, out)
				for _, item := range out["items"].([]any) {
					got = append(got, named(item, "title"))
				}
				next, _ := out["next"].(string)
				if more, _ := out["more"].(bool); !more {
					if next != "" {
						t.Fatalf("find %s: a next with no more", q)
					}
					return got, answers
				}
				if next == "" {
					t.Fatalf("find %s: more with no next", q)
				}
				after = next
			}
			t.Fatalf("find %s never ended", query)
			return nil, nil
		}
		whole, answers := follow("project=Orchard&limit=500")
		if len(answers) != 1 {
			t.Fatalf("all at once took %d answers", len(answers))
		}
		apples := []string{"Apple", "apple"}
		if ids["apple"] < ids["Apple"] {
			apples = []string{"apple", "Apple"}
		}
		want := slices.Concat([]string{"Café first", "Zest at nine"}, apples, []string{"Banana", "Cherry", "Late café", "undated A", "Undated b", "Éclair"})
		if !slices.Equal(whole, want) {
			t.Fatalf("the order = %q, want %q", whole, want)
		}
		// Two at a time, through the same day, title and undated tasks.
		if got, answers := follow("project=Orchard&limit=2"); !slices.Equal(got, want) || len(answers) != 5 {
			t.Fatalf("two at a time = %q in %d answers, want %q", got, len(answers), want)
		}
		// Words the database can't narrow: the search stops after reading
		// four tasks, with fewer than its limit, and goes on from there.
		got, answers := follow("project=Orchard&words=caf%C3%A9&limit=5")
		if !slices.Equal(got, []string{"Café first", "Late café"}) || len(answers) != 2 || len(answers[0]["items"].([]any)) != 1 {
			t.Fatalf("a search cut short = %q in %v", got, answers)
		}
		for _, bad := range []string{"nonsense", "W10", base64.RawURLEncoding.EncodeToString([]byte(`["2031-05-02","","x"]`)),
			base64.RawURLEncoding.EncodeToString([]byte(`["","","a\u0000b","x"]`))} {
			if code, out := w.find(t, w.reader, "after="+url.QueryEscape(bad)); code != 400 || out["field"] != "after" {
				t.Errorf("after %q = %d %v", bad, code, out)
			}
		}

		// Kinds, asked of the database: a status of the kind, or for a task
		// without one, todo while open and done once completed.
		if code, out := w.act(t, "set-status", `{"task":"`+ids["Cherry"]+`","kind":"canceled"}`, nil); code != 200 {
			t.Fatalf("cancel = %d %v", code, out)
		}
		if code, out := w.act(t, "complete-task", `{"task":"`+ids["Banana"]+`"}`, nil); code != 200 {
			t.Fatalf("complete = %d %v", code, out)
		}
		loose := add(`{"title":"Loose end"}`)
		if got, _ := follow("kind=canceled&state=all"); !slices.Equal(got, []string{"Cherry"}) {
			t.Errorf("canceled = %q", got)
		}
		if got, _ := follow("kind=done&state=all&project=Orchard"); !slices.Equal(got, []string{"Banana"}) {
			t.Errorf("done in Orchard = %q", got)
		}
		if got, _ := follow("kind=todo&words=loose"); !slices.Equal(got, []string{"Loose end"}) {
			t.Errorf("an open Inbox task as todo = %q", got)
		}
		if code, out := w.act(t, "complete-task", `{"task":"`+loose+`"}`, nil); code != 200 {
			t.Fatalf("complete = %d %v", code, out)
		}
		if got, _ := follow("kind=todo&words=loose&state=all"); len(got) != 0 {
			t.Errorf("a done Inbox task as todo = %q", got)
		}
		if got, _ := follow("kind=done&words=loose&state=all"); !slices.Equal(got, []string{"Loose end"}) {
			t.Errorf("a done Inbox task as done = %q", got)
		}
	})
}
