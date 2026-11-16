package syncer_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// IDs are one space for every workspace, and some are worked out from
// others. Someone who knows a workspace's IDs (a guest, someone who left)
// cannot take those of its rows to come in a workspace of their own, which
// would keep the real rows from ever being made: a label or person on a
// task, a repeating task's next one and what it copies, a wiki's main page.
func TestWorkedOutIDsCannotBeTakenFromElsewhere(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		victim, attacker := newWorld(t, d), newWorld(t, d)
		project, task, label, item := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		repeating := newTask(project, nil, "Water the plants")
		repeating["recurrence"] = "FREQ=WEEKLY"
		victim.push("owner",
			op("project", project, later(0), newProject("Garden")),
			op("task", task, later(1), repeating),
			op("label", label, later(1), map[string]any{"name": "Urgent", "color": "red", "sort_key": "a0", "deleted_at": nil}),
			op("checklist_item", item, later(2), map[string]any{"task_id": task, "text": "Ferns", "done": false, "sort_key": "a0", "deleted_at": nil}),
		)
		next := recur.NextTaskID(task)
		copied := recur.HashID(next + "/" + item)

		// The attacker's own project and repeating task, to hang rows on.
		mine, own := uuid.NewString(), uuid.NewString()
		ownRepeating := newTask(mine, nil, "Mine")
		ownRepeating["recurrence"] = "FREQ=WEEKLY"
		attacker.push("owner", op("project", mine, later(3), newProject("Mine")), op("task", own, later(3), ownRepeating))

		taking := map[string]syncer.Op{
			"the next task":              op("task", next, later(4), withSeries(newTask(mine, nil, "Squat"), own)),
			"the next task's checklist":  op("checklist_item", copied, later(4), map[string]any{"task_id": own, "text": "x", "done": false, "sort_key": "a0", "deleted_at": nil}),
			"a pair's ID as a task":      op("task", syncer.PairID(task, label), later(4), newTask(mine, nil, "Squat")),
			"the project's main page":    op("page", syncer.MainPageID(project), later(4), newPage(mine, nil, "Squat")),
			"the workspace's main page":  op("page", syncer.MainPageID(victim.ws), later(4), newPage(nil, nil, "Squat")),
			"a main page's ID as a task": op("task", syncer.MainPageID(project), later(4), newTask(mine, nil, "Squat")),
		}
		for what, o := range taking {
			if res := attacker.push("owner", o)[0]; res.Status != "rejected" {
				t.Errorf("taking %s from another workspace: %s", what, res.Status)
			}
		}
		// Two IDs of the attacker's own whose XOR is the victim's pair, as
		// pair IDs once were: their pair's row is another row now.
		ownTask := uuid.NewString()
		ownLabel := ids.XOR(ids.XOR(ownTask, task), label)
		attacker.push("owner",
			op("task", ownTask, later(5), newTask(mine, nil, "Squat")),
			op("label", ownLabel, later(5), map[string]any{"name": "Squat", "color": "red", "sort_key": "a0", "deleted_at": nil}),
			op("task_label", syncer.PairID(ownTask, ownLabel), later(5), map[string]any{"task_id": ownTask, "label_id": ownLabel, "deleted_at": nil}),
		)

		// The victim makes every one of them.
		done := map[string]any{"completed_at": "2026-11-05T10:00:00Z"}
		wantStatus(t, victim.push("owner", op("task", task, later(6), done))[0], "applied")
		// In order: the next task before its checklist.
		for _, m := range []struct {
			what string
			op   syncer.Op
		}{
			{"a label on the task", op("task_label", syncer.PairID(task, label), later(7), map[string]any{"task_id": task, "label_id": label, "deleted_at": nil})},
			{"the next task", op("task", next, later(7), withSeries(newTask(project, nil, "Water the plants"), task))},
			{"its checklist", op("checklist_item", copied, later(8), map[string]any{"task_id": next, "text": "Ferns", "done": false, "sort_key": "a0", "deleted_at": nil})},
			{"the project's wiki", op("page", syncer.MainPageID(project), later(7), newPage(project, nil, "Garden"))},
			{"the workspace's wiki", op("page", syncer.MainPageID(victim.ws), later(7), newPage(nil, nil, "Team"))},
		} {
			if res := victim.push("owner", m.op)[0]; res.Status != "applied" {
				t.Errorf("making %s: %s (%s)", m.what, res.Status, res.Message)
			}
		}
	})
}

func withSeries(task map[string]any, series string) map[string]any {
	task["series_id"] = series
	return task
}

// An import that keeps its IDs cannot take them either: an export can be
// anyone's making. A row under an ID Mila works out, which nothing in the
// export works out, gets a fresh one; what the export does work out (a
// repeating task's next one and its checklist) keeps its own, and a pair
// from an export made when pair IDs were XORs is named by its hash.
func TestAnImportKeepingIDsCannotTakeWorkedOutOnes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		victim := newWorld(t, d)
		project, task := uuid.NewString(), uuid.NewString()
		victim.push("owner", op("project", project, later(0), newProject("Garden")), op("task", task, later(1), newTask(project, nil, "Paint")))

		f := fill(t, d)
		owner := f.users["owner"]
		dump, err := f.svc.Dump(ctx, owner, f.ws)
		must(t, err)
		must(t, f.st.PurgeWorkspace(ctx, f.ws))
		taken := []string{recur.NextTaskID(task), syncer.MainPageID(project), recur.HashID("nothing here")}
		for _, id := range taken {
			raw := map[string]any{}
			for k, v := range newTask(nil, nil, "Squat") {
				raw[k] = v
			}
			dump.Rows = append(dump.Rows, syncer.Row{Entity: "task", ID: id, Fields: raw, Clocks: map[string]string{}})
		}
		legacy := 0
		for i, r := range dump.Rows {
			if r.Entity == "task_assignee" || r.Entity == "task_label" {
				a, _ := r.Fields["task_id"].(string)
				b, _ := r.Fields["user_id"].(string)
				if r.Entity == "task_label" {
					b, _ = r.Fields["label_id"].(string)
				}
				dump.Rows[i].ID = syncer.LegacyPairID(a, b)
				legacy++
			}
		}
		if legacy == 0 {
			t.Fatal("the export holds no pairs to name the old way")
		}
		into := newHidden(t, f.st)
		_, err = f.svc.Restore(ctx, into, dump.Rows, dump.Activity, syncer.RestoreOptions{FreshIDs: false, Owner: owner, Exporter: owner})
		must(t, err)
		rows, err := f.svc.Restored(ctx, into)
		must(t, err)
		got := map[string]bool{}
		for _, r := range rows {
			got[r.ID] = true
			if r.Entity == "task_assignee" || r.Entity == "task_label" {
				a, _ := r.Fields["task_id"].(string)
				b, _ := r.Fields["user_id"].(string)
				if r.Entity == "task_label" {
					b, _ = r.Fields["label_id"].(string)
				}
				if r.ID != syncer.PairID(a, b) {
					t.Errorf("a %s from an export of XORed pairs is %s, not its pair's hash", r.Entity, r.ID)
				}
			}
		}
		for _, id := range taken {
			if got[id] {
				t.Errorf("the import kept %s, an ID Mila works out for another workspace's row", id)
			}
		}
		for what, id := range map[string]string{"the next task": f.next, "its checklist": f.nextItem} {
			if !got[id] {
				t.Errorf("%s did not keep its ID %s", what, id)
			}
		}
	})
}
