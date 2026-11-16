package syncer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestRecurrenceFields(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		task := uuid.NewString()
		wantStatus(t, w.push("member", op("task", task, later(0), merge(newTask(nil, nil, "Water the plants"), map[string]any{
			"created_by": w.users["member"], "recurrence": "FREQ=WEEKLY;BYDAY=MO,TH", "series_id": task, "due_date": "2026-10-29",
		})))[0], "applied")

		for name, fields := range map[string]map[string]any{
			"a rule outside the subset": {"recurrence": "FREQ=HOURLY"},
			"an empty rule":             {"recurrence": ""},
			"a count":                   {"recurrence": "FREQ=DAILY;COUNT=3"},
			"an unknown mode":           {"recurrence_mode": "whenever"},
			"a series that is no ID":    {"series_id": "not an id!"},
			"someone else as creator":   {"created_by": w.users["owner"]},
		} {
			t.Run(name, func(t *testing.T) {
				wantStatus(t, w.push("member", op("task", task, later(1), fields))[0], "rejected")
			})
		}
		// A new task can only be created by the one making it.
		wantStatus(t, w.push("member", op("task", uuid.NewString(), later(1), merge(newTask(nil, nil, "Forged"), map[string]any{
			"created_by": w.users["owner"]})))[0], "rejected")

		wantStatus(t, w.push("member", op("task", task, later(2), map[string]any{"recurrence": nil, "recurrence_mode": "completion"}))[0], "applied")

		// Two people complete the same task offline and both make its next
		// task, with the same derived ID. The one who made it first by the
		// clock is its creator, whichever push arrives first, and the pull
		// tells the other device so.
		copyOf := func(by string) map[string]any {
			return merge(newTask(nil, nil, "Water the plants"), map[string]any{
				"created_by": by, "recurrence": "FREQ=WEEKLY;BYDAY=MO,TH", "series_id": task, "due_date": "2026-11-02"})
		}
		creator := func(id string) any {
			snap, err := w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
			must(t, err)
			for _, r := range snap.Rows {
				if r.ID == id {
					return r.Fields["created_by"]
				}
			}
			t.Fatalf("task %s is missing", id)
			return nil
		}
		var last, lastAt string
		for _, order := range []string{"first made arrives first", "first made arrives last"} {
			t.Run(order, func(t *testing.T) {
				// The task they both completed, in the same series.
				done := uuid.NewString()
				wantStatus(t, w.push("member", op("task", done, later(2), merge(newTask(nil, nil, "Water the plants"), map[string]any{
					"created_by": w.users["member"], "series_id": task})))[0], "applied")
				next := recur.NextTaskID(done)
				// The member makes it before the owner, by the clock.
				byMember, byOwner := op("task", next, later(3), copyOf(w.users["member"])), op("task", next, later(4), copyOf(w.users["owner"]))
				if order == "first made arrives first" {
					wantStatus(t, w.push("member", byMember)[0], "applied")
				} else {
					wantStatus(t, w.push("owner", byOwner)[0], "applied")
				}
				var since int64
				for more := true; more; {
					pulled, err := w.svc.Pull(ctx, w.users["owner"], w.ws, since)
					must(t, err)
					since, more = pulled.Seq, pulled.More
				}
				if order == "first made arrives first" {
					wantStatus(t, w.push("owner", byOwner)[0], "applied")
				} else {
					wantStatus(t, w.push("member", byMember)[0], "applied")
				}
				if got := creator(next); got != w.users["member"] {
					t.Fatalf("the next task's creator is %v, want the member who made it first", got)
				}
				// What the owner's device pulls next says who made it.
				pulled, err := w.svc.Pull(ctx, w.users["owner"], w.ws, since)
				must(t, err)
				var told any
				for _, c := range pulled.Changes {
					if v, ok := c.Fields["created_by"]; ok && c.ID == next {
						told = v
					}
				}
				if order == "first made arrives last" && told != w.users["member"] {
					t.Fatalf("the owner's device was told the creator is %v, want the member", told)
				}
				if order == "first made arrives first" && told != nil {
					t.Fatalf("a later creator was logged: %v", told)
				}
				last, lastAt = next, byMember.HLC
			})
		}

		// An old clock does not make an editor the creator of a task they
		// did not make, or clear it.
		mine := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", mine, later(5), merge(newTask(nil, nil, "Owner's"), map[string]any{
			"created_by": w.users["owner"]})))[0], "applied")
		yearAgo := hlcAt(time.Now().AddDate(-1, 0, 0), "m")
		// Just before the member made the copy, and after the task it follows.
		justBefore := strings.TrimSuffix(lastAt, "a") + "0"
		for name, o := range map[string]syncer.Op{
			"taking it with an old clock":             op("task", mine, yearAgo, map[string]any{"created_by": w.users["member"]}),
			"clearing it with an old clock":           op("task", mine, hlcAt(time.Now().AddDate(-2, 0, 0), "m"), map[string]any{"created_by": nil}),
			"a whole task with an old clock":          op("task", mine, yearAgo, merge(newTask(nil, nil, "Mine"), map[string]any{"created_by": w.users["member"]})),
			"a series copy older than it could be":    op("task", last, yearAgo, copyOf(w.users["owner"])),
			"a series copy naming no one":             op("task", last, justBefore, merge(copyOf(w.users["owner"]), map[string]any{"created_by": nil})),
			"part of a series copy with an old clock": op("task", last, justBefore, map[string]any{"created_by": w.users["owner"]}),
		} {
			t.Run(name, func(t *testing.T) {
				wantStatus(t, w.push("member", o)[0], "rejected")
			})
		}
		if got := creator(mine); got != w.users["owner"] {
			t.Fatalf("the owner's task was made by %v after the old writes, want the owner", got)
		}
		if got := creator(last); got != w.users["member"] {
			t.Fatalf("the series copy was made by %v after the old writes, want the member", got)
		}
	})
}
