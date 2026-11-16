package syncer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// A guest's refused write says nothing of rows they cannot see: a row in
// another project answers as one that does not exist, and a dependency on
// a hidden task is refused as theirs to make before any loop is looked
// for (whose message names the tasks in it).
func TestRefusalsTellGuestsNothingHidden(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		hidden, hiddenStatus := w.project("Hidden")
		w.share(shared, true)
		mine, secret := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", mine, hlcAt(now, "a"), newTask(shared, nil, "Paint")),
			op("task", secret, hlcAt(now, "a"), newTask(hidden, nil, "Layoffs")),
		)
		// The hidden task already waits for the guest's.
		wantStatus(t, w.push("owner", op("task_dependency", syncer.PairID(secret, mine), hlcAt(now, "a"),
			map[string]any{"task_id": secret, "blocker_id": mine}))[0], "applied")

		later := now.Add(time.Second)
		message := func(fields map[string]any) string {
			t.Helper()
			res := w.push("guest", op("task", uuid.NewString(), hlcAt(later, "g"), fields))[0]
			if res.Status != "rejected" {
				t.Fatalf("%v: %s", fields, res.Status)
			}
			return res.Message
		}
		for _, field := range []string{"status_id", "parent_id"} {
			ref := map[string]string{"status_id": hiddenStatus, "parent_id": secret}[field]
			a, b := newTask(shared, nil, "One"), newTask(shared, nil, "Two")
			a[field], b[field] = ref, uuid.NewString()
			if hiddenMsg, missingMsg := message(a), message(b); hiddenMsg != missingMsg {
				t.Errorf("%s: a hidden row answers %q, a missing one %q", field, hiddenMsg, missingMsg)
			}
		}

		res := w.push("guest", op("task_dependency", syncer.PairID(mine, secret), hlcAt(later, "g"),
			map[string]any{"task_id": mine, "blocker_id": secret}))[0]
		if res.Status != "rejected" || strings.Contains(res.Message, "Layoffs") || strings.Contains(res.Message, "loop") {
			t.Errorf("a dependency on a hidden task: %s (%s)", res.Status, res.Message)
		}
	})
}

// A guest's refusal names the rule where their own request decides it (the
// kind of row; a task or page they put outside every project), and the
// general rule otherwise, so it never says where a hidden row lives.
func TestGuestRefusalsNameTheRule(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		hidden, _ := w.project("Hidden")
		w.share(shared, true)
		mine, secret, inbox := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", mine, hlcAt(now, "a"), newTask(shared, nil, "Paint")),
			op("task", secret, hlcAt(now, "a"), newTask(hidden, nil, "Layoffs")),
			op("task", inbox, hlcAt(now, "a"), newTask(nil, nil, "Dentist")),
		)
		later := hlcAt(now.Add(time.Second), "g")
		comment := func(task string) map[string]any {
			return map[string]any{"task_id": task, "author_id": w.users["guest"], "body": "Hi", "edited_at": nil, "deleted_at": nil}
		}
		for _, c := range []struct {
			name, entity, id string
			fields           map[string]any
			want             string
		}{
			{"a project", "project", uuid.NewString(), newProject("Mine"), "Guests can't make or change projects"},
			{"a label", "label", uuid.NewString(), map[string]any{"name": "Mine", "color": "red", "sort_key": "a0", "deleted_at": nil}, "Guests can't make or change labels"},
			{"an inbox task", "task", uuid.NewString(), newTask(nil, nil, "Mine"), "Guests can't put tasks in the Inbox"},
			{"a shared task to the inbox", "task", mine, map[string]any{"project_id": nil}, "Guests can't put tasks in the Inbox"},
			{"a hidden task to the inbox", "task", secret, map[string]any{"project_id": nil}, "Guests can only change tasks in the projects shared with them"},
			{"a new task in a hidden project", "task", uuid.NewString(), newTask(hidden, nil, "Mine"), "Guests can only change tasks in the projects shared with them"},
			{"a comment on a hidden task", "comment", uuid.NewString(), comment(secret), "Guests can only change tasks in the projects shared with them"},
			{"a comment on someone's inbox task", "comment", uuid.NewString(), comment(inbox), "Guests can only change tasks in the projects shared with them"},
		} {
			res := w.push("guest", op(c.entity, c.id, later, c.fields))[0]
			if res.Status != "rejected" || !strings.HasPrefix(res.Message, c.want) {
				t.Errorf("%s: %s %q, want %q", c.name, res.Status, res.Message, c.want)
			}
		}
	})
}
