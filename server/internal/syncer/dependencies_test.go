package syncer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func depRow(task, blocker string) map[string]any {
	return map[string]any{"task_id": task, "blocker_id": blocker, "deleted_at": nil}
}

func dep(task, blocker, hlc string) syncer.Op {
	return op("task_dependency", syncer.PairID(task, blocker), hlc, depRow(task, blocker))
}

func TestDependenciesRefuseLoops(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", a, hlcAt(now, "a"), newTask(nil, nil, "Demo")),
			op("task", b, hlcAt(now, "a"), newTask(nil, nil, "Drywall")),
			op("task", c, hlcAt(now, "a"), newTask(nil, nil, "Paint")))
		// b waits for a, c for b.
		wantStatus(t, w.push("owner", dep(b, a, hlcAt(now, "a")))[0], "applied")
		wantStatus(t, w.push("owner", dep(c, b, hlcAt(now, "a")))[0], "applied")
		// a waiting for c would close the loop, as would a task waiting for itself.
		res := w.push("owner", dep(a, c, hlcAt(now, "a")))[0]
		wantStatus(t, res, "rejected")
		if !strings.Contains(res.Message, `"Paint" already waits for "Demo"`) {
			t.Errorf("the refusal says %q", res.Message)
		}
		wantStatus(t, w.push("owner", dep(a, a, hlcAt(now, "a")))[0], "rejected")
		// A row not named by its pair is refused.
		wantStatus(t, w.push("owner", op("task_dependency", uuid.NewString(), hlcAt(now, "a"), depRow(a, b)))[0], "rejected")

		// Once c no longer waits for b, a may wait for c; bringing the old
		// row back would then close a loop, and is refused.
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("task_dependency", syncer.PairID(c, b), hlcAt(later, "a"), map[string]any{"deleted_at": later.UTC().Format(time.RFC3339Nano)}))[0], "applied")
		wantStatus(t, w.push("owner", dep(a, c, hlcAt(later, "a")))[0], "applied")
		again := later.Add(time.Second)
		wantStatus(t, w.push("owner", op("task_dependency", syncer.PairID(c, b), hlcAt(again, "a"), map[string]any{"deleted_at": nil}))[0], "rejected")
	})
}

func TestAGuestSeesADependencyOnlyWhenTheySeeBothTasks(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		shared, status := w.project("Shared")
		hidden, hiddenStatus := w.project("Hidden")
		w.share(shared, true)
		a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", a, hlcAt(now, "a"), newTask(shared, status, "Seen")),
			op("task", b, hlcAt(now, "a"), newTask(shared, status, "Also seen")),
			op("task", c, hlcAt(now, "a"), newTask(hidden, hiddenStatus, "Not seen")))
		inside, across := syncer.PairID(b, a), syncer.PairID(a, c)
		before, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		w.push("owner", dep(b, a, hlcAt(now, "a")), dep(a, c, hlcAt(now, "a")))

		got := map[string]bool{}
		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		for _, r := range snap.Rows {
			if r.Entity == "task_dependency" {
				got[r.ID] = true
			}
		}
		if !got[inside] || got[across] {
			t.Fatalf("the guest's bootstrap holds %v, want only the dependency inside their project", got)
		}
		changes, err := w.svc.Pull(ctx, w.users["guest"], w.ws, before.Seq)
		must(t, err)
		pulled := map[string]bool{}
		for _, c := range changes.Changes {
			if c.Entity == "task_dependency" {
				pulled[c.ID] = true
			}
		}
		if !pulled[inside] || pulled[across] {
			t.Fatalf("the guest pulled %v, want only the dependency inside their project", pulled)
		}
		// A member sees both.
		snap, err = w.svc.Bootstrap(ctx, w.users["member"], w.ws)
		must(t, err)
		n := 0
		for _, r := range snap.Rows {
			if r.Entity == "task_dependency" {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("the member's bootstrap holds %d dependencies, want 2", n)
		}
		// A guest may link tasks they see, and not one they cannot.
		wantStatus(t, w.push("guest", dep(a, b, hlcAt(now, "g")))[0], "rejected") // a loop: b waits for a
		x := uuid.NewString()
		w.push("owner", op("task", x, hlcAt(now, "a"), newTask(shared, status, "Third")))
		wantStatus(t, w.push("guest", dep(x, a, hlcAt(now, "g")))[0], "applied")
		wantStatus(t, w.push("guest", dep(x, c, hlcAt(now, "g")))[0], "rejected")

		// REST the same: a guest neither lists nor gets a dependency with
		// an end they cannot see, either way across.
		y := uuid.NewString()
		w.push("owner", op("task", y, hlcAt(now, "a"), newTask(shared, status, "Fourth")))
		back := syncer.PairID(c, y)
		w.push("owner", dep(c, y, hlcAt(now, "a")))
		listed := map[string]bool{}
		page, err := w.svc.List(ctx, w.users["guest"], w.ws, "task_dependency", syncer.Query{})
		must(t, err)
		for _, r := range page.Rows {
			listed[r.ID] = true
		}
		if !listed[inside] || !listed[syncer.PairID(x, a)] || listed[across] || listed[back] || len(listed) != 2 {
			t.Fatalf("the guest's REST list holds %v, want only the dependencies inside their project", listed)
		}
		for _, id := range []string{across, back} {
			_, err := w.svc.Get(ctx, w.users["guest"], w.ws, "task_dependency", id)
			if err == nil {
				t.Fatalf("the guest got the dependency %s across", id)
			}
		}
		page, err = w.svc.List(ctx, w.users["member"], w.ws, "task_dependency", syncer.Query{})
		must(t, err)
		if len(page.Rows) != 4 {
			t.Fatalf("the member's REST list holds %d dependencies, want 4", len(page.Rows))
		}
	})
}

func TestADependencyGoesWithItsTask(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		a, b := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", a, hlcAt(now, "a"), newTask(nil, nil, "First")),
			op("task", b, hlcAt(now, "a"), newTask(nil, nil, "Second")))
		w.push("owner", dep(b, a, hlcAt(now, "a")))
		dd := w.st.DB()
		_, err := dd.ExecContext(context.Background(), rebind(dd, "DELETE FROM tasks WHERE id = ?"), a)
		must(t, err)
		if n := w.count("task_dependencies", "1 = 1"); n != 0 {
			t.Fatalf("%d dependencies left after the blocker was purged", n)
		}
	})
}
