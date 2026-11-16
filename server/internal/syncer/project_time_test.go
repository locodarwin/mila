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

func projectEntry(project, user, day string, minutes int) map[string]any {
	return map[string]any{"project_id": project, "user_id": user, "day": day, "minutes": minutes}
}

// Time logged to a project itself: its person's, changed by them and by
// admins, and seen by whoever sees the project, guests included.
func TestProjectTimeFollowsItsProject(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		admin := uuid.NewString()
		now := store.Now()
		must(t, w.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: admin, Email: "admin-" + admin[:8] + "@example.com", Name: "admin", Now: now}))
		must(t, w.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: admin, Role: "admin", Now: now}))
		w.users["admin"] = admin
		shared, _ := w.project("Kitchen")
		w.share(shared, true)
		private, _ := w.project("Private")
		at := time.Now()

		// Logged as oneself, never as someone else, and a day at most.
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(at, "m"), projectEntry(shared, w.users["owner"], "2026-11-12", 30)))[0], "rejected")
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(at, "m"), projectEntry(shared, w.users["member"], "2026-11-12", 24*60+1)))[0], "rejected")
		entry := uuid.NewString()
		wantStatus(t, w.push("member", op("project_time_entry", entry, hlcAt(at, "m"), projectEntry(shared, w.users["member"], "2026-11-12", 90)))[0], "applied")

		// Its project is fixed; a guest cannot change it, an admin can.
		later := at.Add(time.Second)
		wantStatus(t, w.push("member", op("project_time_entry", entry, hlcAt(later, "m"), map[string]any{"project_id": private}))[0], "rejected")
		wantStatus(t, w.push("guest", op("project_time_entry", entry, hlcAt(later.Add(time.Second), "g"), map[string]any{"minutes": 5}))[0], "rejected")
		wantStatus(t, w.push("admin", op("project_time_entry", entry, hlcAt(later.Add(2*time.Second), "a"), map[string]any{"note": "Supplier call"}))[0], "applied")
		if f := w.rowFields("project_time_entry", entry); f["minutes"] != int64(90) || f["note"] != "Supplier call" {
			t.Fatalf("the entry = %v", f)
		}

		// A guest logs time to a project they see, and sees everyone's there,
		// but none in a project they don't, nor logs any there.
		wantStatus(t, w.push("guest", op("project_time_entry", uuid.NewString(), hlcAt(later, "g"), projectEntry(shared, w.users["guest"], "2026-11-12", 15)))[0], "applied")
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(later, "m"), projectEntry(private, w.users["member"], "2026-11-12", 20)))[0], "applied")
		wantStatus(t, w.push("guest", op("project_time_entry", uuid.NewString(), hlcAt(later, "g"), projectEntry(private, w.users["guest"], "2026-11-12", 15)))[0], "rejected")
		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		n := 0
		for _, r := range snap.Rows {
			if r.Entity == "project_time_entry" {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("a guest's bootstrap holds %d project entries, want 2", n)
		}
	})
}

// The digest counts time logged to a project itself beside time on tasks,
// most first, and only in projects the person still sees.
func TestTheDigestReadsProjectTime(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		project, status := w.project("Kitchen")
		at := time.Now()
		task := uuid.NewString()
		wantStatus(t, w.push("member", op("task", task, hlcAt(at, "m"), newTask(project, status, "Fit the boxes")))[0], "applied")
		wantStatus(t, w.push("member", op("time_entry", uuid.NewString(), hlcAt(at, "m"), entryRow(task, w.users["member"], "2026-11-12", 30)))[0], "applied")
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(at, "m"), projectEntry(project, w.users["member"], "2026-11-12", 50)))[0], "applied")
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(at, "m"), projectEntry(project, w.users["member"], "2026-11-12", 25)))[0], "applied")
		// Another day, and another person, are not this one's.
		wantStatus(t, w.push("member", op("project_time_entry", uuid.NewString(), hlcAt(at, "m"), projectEntry(project, w.users["member"], "2026-11-11", 600)))[0], "applied")
		wantStatus(t, w.push("owner", op("project_time_entry", uuid.NewString(), hlcAt(at, "o"), projectEntry(project, w.users["owner"], "2026-11-12", 600)))[0], "applied")

		day, err := w.svc.LoggedOn(ctx, w.users["member"], "2026-11-12")
		must(t, err)
		if len(day) != 2 || !day[0].Itself || day[0].ProjectID != project || day[0].Title != "Kitchen" || day[0].Logged != 75 ||
			day[1].Itself || day[1].ID != task || day[1].Logged != 30 {
			t.Fatalf("logged on the day: %+v", day)
		}
	})
}
