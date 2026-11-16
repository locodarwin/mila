package syncer_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

func entryRow(task, user, day string, minutes int) map[string]any {
	return map[string]any{"task_id": task, "user_id": user, "day": day, "minutes": minutes}
}

func TestTimeIsChangedByItsPersonAndAdmins(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		admin := uuid.NewString()
		now := store.Now()
		must(t, w.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: admin, Email: "admin-" + admin[:8] + "@example.com", Name: "admin", Now: now}))
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: admin, Role: "admin", Now: now}))
		w.users["admin"] = admin
		project, status := w.project("Kitchen")
		w.share(project, true)
		task := uuid.NewString()
		at := time.Now()
		wantStatus(t, w.push("member", op("task", task, hlcAt(at, "m"), newTask(project, status, "Wire it")))[0], "applied")

		// Time is logged as oneself, never as someone else.
		wantStatus(t, w.push("member", op("time_entry", uuid.NewString(), hlcAt(at, "m"), entryRow(task, w.users["owner"], "2026-10-28", 30)))[0], "rejected")
		entry := uuid.NewString()
		wantStatus(t, w.push("member", op("time_entry", entry, hlcAt(at, "m"), entryRow(task, w.users["member"], "2026-10-28", 30)))[0], "applied")
		// A day is the most one entry holds.
		wantStatus(t, w.push("member", op("time_entry", uuid.NewString(), hlcAt(at, "m"), entryRow(task, w.users["member"], "2026-10-28", 24*60+1)))[0], "rejected")

		// Another member cannot change it, nor a guest; an admin can,
		// and its person can delete it.
		later := at.Add(time.Second)
		wantStatus(t, w.push("owner", op("time_entry", entry, hlcAt(later, "o"), map[string]any{"minutes": 45}))[0], "applied")
		wantStatus(t, w.push("guest", op("time_entry", entry, hlcAt(later.Add(time.Second), "g"), map[string]any{"minutes": 5}))[0], "rejected")
		wantStatus(t, w.push("admin", op("time_entry", entry, hlcAt(later.Add(2*time.Second), "a"), map[string]any{"note": "Fixed"}))[0], "applied")
		if f := w.rowFields("time_entry", entry); f["minutes"] != int64(45) || f["note"] != "Fixed" {
			t.Fatalf("the entry = %v", f)
		}
		member := uuid.NewString()
		must(t, w.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: member, Email: "m2-" + member[:8] + "@example.com", Name: "m2", Now: now}))
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: member, Role: "member", Now: now}))
		w.users["member2"] = member
		wantStatus(t, w.push("member2", op("time_entry", entry, hlcAt(later.Add(3*time.Second), "n"), map[string]any{"minutes": 1}))[0], "rejected")
		wantStatus(t, w.push("member2", op("time_entry", entry, hlcAt(later.Add(3*time.Second), "n"), map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339)}))[0], "rejected")

		// A guest logs time on a task they see, and sees everyone's.
		wantStatus(t, w.push("guest", op("time_entry", uuid.NewString(), hlcAt(later, "g"), entryRow(task, w.users["guest"], "2026-10-28", 15)))[0], "applied")
		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		n := 0
		for _, r := range snap.Rows {
			if r.Entity == "time_entry" {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("a guest's bootstrap holds %d entries", n)
		}
		// Not on a task in a project they cannot see.
		private, privateStatus := w.project("Private")
		hidden := uuid.NewString()
		wantStatus(t, w.push("member", op("task", hidden, hlcAt(at, "m"), newTask(private, privateStatus, "Hidden")))[0], "applied")
		wantStatus(t, w.push("guest", op("time_entry", uuid.NewString(), hlcAt(later, "g"), entryRow(hidden, w.users["guest"], "2026-10-28", 15)))[0], "rejected")
	})
}

func TestTheDigestReadsTime(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		project, status := w.project("Kitchen")
		at := time.Now()
		over, under, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
		withEstimate := func(title string, minutes int) map[string]any {
			f := newTask(project, status, title)
			f["created_by"] = w.users["member"]
			f["estimate_minutes"] = minutes
			return f
		}
		wantStatus(t, w.push("member", op("task", over, hlcAt(at, "m"), withEstimate("Order the countertop", 120)))[0], "applied")
		wantStatus(t, w.push("member", op("task", under, hlcAt(at, "m"), withEstimate("Fit the boxes", 600)))[0], "applied")
		wantStatus(t, w.push("owner", op("task", other, hlcAt(at, "o"), newTask(project, status, "Someone else's")))[0], "applied")
		log := func(role, task, day string, minutes int) {
			wantStatus(t, w.push(role, op("time_entry", uuid.NewString(), hlcAt(at, role[:1]), entryRow(task, w.users[role], day, minutes)))[0], "applied")
		}
		log("member", over, "2026-10-27", 100)
		log("owner", over, "2026-10-27", 50)
		log("member", under, "2026-10-27", 30)
		log("member", under, "2026-10-26", 30)
		log("owner", other, "2026-10-27", 300)

		got, err := w.svc.OverEstimate(ctx, w.users["member"])
		must(t, err)
		if len(got) != 1 || got[0].ID != over || got[0].Estimate != 120 || got[0].Logged != 150 {
			t.Fatalf("over the estimate: %+v", got)
		}
		day, err := w.svc.LoggedOn(ctx, w.users["member"], "2026-10-27")
		must(t, err)
		if len(day) != 2 || day[0].ID != over || day[0].Logged != 100 || day[1].Logged != 30 {
			t.Fatalf("logged on the day: %+v", day)
		}
		// Done tasks are past worrying about.
		wantStatus(t, w.push("member", op("task", over, hlcAt(at.Add(time.Second), "m"), map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339)}))[0], "applied")
		got, err = w.svc.OverEstimate(ctx, w.users["member"])
		must(t, err)
		if len(got) != 0 {
			t.Fatalf("a done task is over: %+v", got)
		}
	})
}

func TestATimerBeingAboutIsNoNews(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		project, status := w.project("Kitchen")
		at := time.Now()
		task := uuid.NewString()
		wantStatus(t, w.push("member", op("task", task, hlcAt(at, "m"), newTask(project, status, "Wire it")))[0], "applied")
		entry := uuid.NewString()
		started := at.UTC().Format(time.RFC3339)
		fields := entryRow(task, w.users["member"], "2026-10-28", 0)
		fields["running"], fields["started_at"], fields["active_at"] = true, started, started
		wantStatus(t, w.push("member", op("time_entry", entry, hlcAt(at, "m"), fields))[0], "applied")
		before := w.count("activity", "task_id = ?", task)
		wantStatus(t, w.push("member", op("time_entry", entry, hlcAt(at.Add(time.Second), "m"), map[string]any{"active_at": time.Now().UTC().Format(time.RFC3339)}))[0], "applied")
		if after := w.count("activity", "task_id = ?", task); after != before {
			t.Fatalf("being about made %d history entries", after-before)
		}
		wantStatus(t, w.push("member", op("time_entry", entry, hlcAt(at.Add(2*time.Second), "m"), map[string]any{"running": false, "minutes": 42}))[0], "applied")
		if after := w.count("activity", "task_id = ?", task); after != before+1 {
			t.Fatalf("stopping made %d history entries", after-before)
		}
	})
}
