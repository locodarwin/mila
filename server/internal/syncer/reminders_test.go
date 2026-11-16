package syncer_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func newReminder(task, user string, at time.Time, offset any) map[string]any {
	return map[string]any{"task_id": task, "user_id": user, "remind_at": at.UTC().Format(time.RFC3339Nano),
		"offset_minutes": offset, "deleted_at": nil}
}

func TestRemindersArePrivate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		task := uuid.NewString()
		w.push("owner", op("task", task, later(0), newTask(nil, nil, "Shared task")))
		at := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
		mine := uuid.NewString()
		wantStatus(t, w.push("member", op("reminder", mine, later(1), newReminder(task, w.users["member"], at, 60)))[0], "applied")

		for name, fields := range map[string]map[string]any{
			"in someone else's name": newReminder(task, w.users["owner"], at, nil),
			"a negative offset":      newReminder(task, w.users["member"], at, -5),
			"an offset over a year":  newReminder(task, w.users["member"], at, syncer.MaxReminderOffset+1),
			"no time":                {"task_id": task, "user_id": w.users["member"]},
		} {
			t.Run(name, func(t *testing.T) {
				wantStatus(t, w.push("member", op("reminder", uuid.NewString(), later(2), fields))[0], "rejected")
			})
		}
		// Nobody else can move it, and a refusal does not show it to them.
		res := w.push("owner", op("reminder", mine, later(3), map[string]any{"remind_at": at.Add(time.Hour).Format(time.RFC3339)}))[0]
		wantStatus(t, res, "rejected")
		if res.Row != nil {
			t.Fatal("a refused edit revealed someone else's reminder")
		}
		wantStatus(t, w.push("member", op("reminder", mine, later(4), map[string]any{"offset_minutes": nil}))[0], "applied")

		count := func(role string) (int, int) {
			snap, err := w.svc.Bootstrap(ctx, w.users[role], w.ws)
			must(t, err)
			changes, err := w.svc.Pull(ctx, w.users[role], w.ws, 0)
			must(t, err)
			n, m := 0, 0
			for _, r := range snap.Rows {
				if r.Entity == "reminder" {
					n++
				}
			}
			for _, c := range changes.Changes {
				if c.Entity == "reminder" {
					m++
				}
			}
			return n, m
		}
		if n, m := count("member"); n != 1 || m != 2 {
			t.Fatalf("member sees %d reminders, pulls %d changes", n, m)
		}
		if n, m := count("owner"); n != 0 || m != 0 {
			t.Fatalf("owner sees %d reminders, pulls %d changes", n, m)
		}
		// A reminder is nobody else's news: the task's history leaves it out.
		events, err := w.svc.Activity(ctx, w.users["owner"], w.ws, task)
		must(t, err)
		for _, e := range events {
			if e.Entity == "reminder" {
				t.Fatalf("history shows a reminder: %+v", e)
			}
		}
	})
}

func TestRemindersDueAndCopied(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		project := uuid.NewString()
		w.push("owner", op("project", project, later(0), newProject("Home")))
		_, err := d.ExecContext(ctx, rebind(d, "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"), project, w.users["guest"], store.Now())
		must(t, err)

		first := uuid.NewString()
		w.push("owner", op("task", first, later(1), merge(newTask(project, nil, "Water the plants"), map[string]any{
			"recurrence": "FREQ=WEEKLY", "due_date": "2026-11-02", "due_time": "18:00", "series_id": first})))
		at := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)
		fixed, ownerRel, memberRel, guestRel, gone := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner", op("reminder", fixed, later(2), newReminder(first, w.users["owner"], at, nil)))
		w.push("owner", op("reminder", ownerRel, later(3), newReminder(first, w.users["owner"], at, 60)))
		w.push("member", op("reminder", memberRel, later(4), newReminder(first, w.users["member"], at.Add(-23*time.Hour), 1440)))
		w.push("guest", op("reminder", guestRel, later(5), newReminder(first, w.users["guest"], at, 60)))
		w.push("member", op("reminder", gone, later(6), merge(newReminder(first, w.users["member"], at, 15), map[string]any{
			"deleted_at": time.Now().UTC().Format(time.RFC3339Nano)})))

		// What the scan sees: fixed reminders in the window, relative ones by
		// their task's due date.
		list, err := w.svc.Reminders(ctx, at.Add(-time.Minute), at, "2026-11-01")
		must(t, err)
		if len(list) != 4 {
			t.Fatalf("listed %d reminders, want 4: %+v", len(list), list)
		}
		list, err = w.svc.Reminders(ctx, at.Add(time.Minute), at.Add(time.Hour), "2026-11-03")
		must(t, err)
		if len(list) != 0 {
			t.Fatalf("listed %+v", list)
		}

		// The owner completes it; their device makes the next task and its
		// own copy, as the app does. The server copies the others'.
		next := recur.NextTaskID(first)
		w.push("owner", op("task", first, later(7), map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339Nano)}))
		w.push("owner", op("task", next, later(8), merge(newTask(project, nil, "Water the plants"), map[string]any{
			"recurrence": "FREQ=WEEKLY", "due_date": "2026-11-09", "due_time": "18:00", "series_id": first})))
		ownCopy := recur.HashID(next + "/" + ownerRel)
		w.push("owner", op("reminder", ownCopy, later(9), newReminder(next, w.users["owner"], at.AddDate(0, 0, 7), 60)))

		// Someone who left is not copied.
		_, err = d.ExecContext(ctx, rebind(d, "DELETE FROM project_access WHERE user_id = ?"), w.users["guest"])
		must(t, err)
		made, err := w.svc.CopyReminders(ctx, next)
		must(t, err)
		if made != 1 {
			t.Fatalf("copied %d reminders, want the member's one", made)
		}
		if made, _ := w.svc.CopyReminders(ctx, next); made != 0 {
			t.Fatalf("copied %d more on a second run", made)
		}
		snap, err := w.svc.Bootstrap(ctx, w.users["member"], w.ws)
		must(t, err)
		var copied *syncer.Row
		for i, r := range snap.Rows {
			if r.Entity == "reminder" && r.Fields["task_id"] == next {
				copied = &snap.Rows[i]
			}
		}
		if copied == nil || copied.ID != recur.HashID(next+"/"+memberRel) ||
			copied.Fields["offset_minutes"] != float64(1440) && copied.Fields["offset_minutes"] != int64(1440) ||
			copied.Fields["remind_at"] != "2026-11-08T18:00:00Z" {
			t.Fatalf("member's copy = %+v", copied)
		}
		// A task that follows nothing copies nothing.
		if made, err := w.svc.CopyReminders(ctx, first); err != nil || made != 0 {
			t.Fatalf("copied %d to the first task (%v)", made, err)
		}
	})
}
