package syncer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// world is a workspace with members, on a fresh database.
type world struct {
	t     *testing.T
	st    *store.Store
	svc   *syncer.Service
	ws    string
	users map[string]string // role -> user ID
}

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	w := &world{t: t, st: st, svc: syncer.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil), ws: uuid.NewString(), users: map[string]string{}}
	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member", "guest"} {
		id := uuid.NewString()
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "-" + id[:8] + "@example.com", Name: role, Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: id, Role: role, Now: now}))
		w.users[role] = id
	}
	return w
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// clock is a hybrid logical clock, the same rules as the browser's.
type clock struct {
	node    string
	ms      int64
	counter int
}

func (c *clock) tick(now int64) string {
	if now > c.ms {
		c.ms, c.counter = now, 0
	} else {
		c.counter++
	}
	return c.String()
}

func (c *clock) observe(h string) {
	ms, _ := strconv.ParseInt(h[:13], 10, 64)
	n, _ := strconv.ParseInt(h[14:18], 16, 64)
	if ms > c.ms || (ms == c.ms && int(n) > c.counter) {
		c.ms, c.counter = ms, int(n)
	}
}

func (c *clock) String() string { return fmt.Sprintf("%013d-%04x-%s", c.ms, c.counter, c.node) }

func hlcAt(t time.Time, node string) string {
	return fmt.Sprintf("%013d-%04x-%s", t.UnixMilli(), 0, node)
}

// op builds an op from plain values.
func op(entity, id, hlc string, fields map[string]any) syncer.Op {
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	return syncer.Op{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw}
}

func (w *world) push(role string, ops ...syncer.Op) []syncer.Result {
	w.t.Helper()
	res, _, err := w.svc.Push(context.Background(), w.users[role], w.ws, ops)
	must(w.t, err)
	return res
}

func wantStatus(t *testing.T, res syncer.Result, status string) {
	t.Helper()
	if res.Status != status {
		t.Fatalf("op %s: status %s (%s), want %s", res.OpID, res.Status, res.Message, status)
	}
}

func newProject(name string) map[string]any {
	return map[string]any{"name": name, "icon": "", "color": "blue", "description": "", "sort_key": "a0", "archived_at": nil, "deleted_at": nil}
}

func newStatus(project, name, category string) map[string]any {
	return map[string]any{"project_id": project, "name": name, "category": category, "sort_key": "a0", "deleted_at": nil}
}

func newTask(project, status any, title string) map[string]any {
	return map[string]any{"project_id": project, "status_id": status, "title": title, "notes": "", "sort_key": "a0", "completed_at": nil, "deleted_at": nil,
		"parent_id": nil, "priority": "none", "start_date": nil, "due_date": nil, "due_time": nil,
		"created_by": nil, "recurrence": nil, "recurrence_mode": "schedule", "series_id": nil, "estimate_minutes": nil}
}

// pairID is the ID a task's label or assignee row gets on every device:
// a hash of the two IDs, so adding the same one twice makes one row.
func pairID(a, b string) string { return ids.Pair(a, b) }

func TestPushPullBootstrap(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		p, s, task := uuid.NewString(), uuid.NewString(), uuid.NewString()
		res := w.push("owner",
			op("project", p, hlcAt(now, "a"), newProject("Garden")),
			op("status", s, hlcAt(now, "a"), newStatus(p, "To do", "todo")),
			op("task", task, hlcAt(now, "a"), newTask(p, s, "Plant tomatoes")),
		)
		for _, r := range res {
			wantStatus(t, r, "applied")
		}

		snap, err := w.svc.Bootstrap(ctx, w.users["member"], w.ws)
		must(t, err)
		if snap.Seq != 3 || len(snap.Rows) != 3 {
			t.Fatalf("snapshot seq %d with %d rows, want 3 and 3", snap.Seq, len(snap.Rows))
		}
		changes, err := w.svc.Pull(ctx, w.users["member"], w.ws, 1)
		must(t, err)
		if changes.Seq != 3 || len(changes.Changes) != 2 || changes.Changes[1].Fields["title"] != "Plant tomatoes" {
			t.Fatalf("pull after 1 = %+v", changes)
		}
		_, err = w.svc.Pull(ctx, w.users["member"], w.ws, 99)
		if apperr.KindOf(err) != apperr.Gone {
			t.Fatalf("pull ahead of the server = %v, want Gone", err)
		}
		// No such workspace, or not a member of it, look the same.
		for _, c := range []struct{ user, ws string }{{w.users["member"], uuid.NewString()}, {uuid.NewString(), w.ws}} {
			if _, err := w.svc.Bootstrap(ctx, c.user, c.ws); apperr.KindOf(err) != apperr.NotFound {
				t.Fatalf("bootstrap of %s by %s = %v, want NotFound", c.ws, c.user, err)
			}
			if _, err := w.svc.Pull(ctx, c.user, c.ws, 0); apperr.KindOf(err) != apperr.NotFound {
				t.Fatalf("pull of %s by %s = %v, want NotFound", c.ws, c.user, err)
			}
		}
	})
}

func TestMergeIsPerFieldLastWriterWins(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		task := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Draft")))[0], "applied")

		// Two devices edit different fields: both win.
		later := now.Add(time.Second)
		w.push("owner", op("task", task, hlcAt(later, "a"), map[string]any{"title": "From A"}))
		w.push("member", op("task", task, hlcAt(later, "b"), map[string]any{"notes": "From B"}))
		// An older edit to the title arrives late and loses, quietly.
		res := w.push("member", op("task", task, hlcAt(now.Add(500*time.Millisecond), "b"), map[string]any{"title": "Stale"}))
		wantStatus(t, res[0], "applied")

		snap, err := w.svc.Bootstrap(ctx, w.users["owner"], w.ws)
		must(t, err)
		got := snap.Rows[0].Fields
		if got["title"] != "From A" || got["notes"] != "From B" {
			t.Fatalf("merged row = %v", got)
		}
		// The losing write logged nothing. (The write to the notes also
		// logged a row of the task's document: four changes.)
		if snap.Seq != 4 {
			t.Fatalf("seq = %d, want 4", snap.Seq)
		}
	})
}

func TestDuplicatePushChangesNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		o := op("task", uuid.NewString(), hlcAt(time.Now(), "a"), newTask(nil, nil, "Once"))
		wantStatus(t, w.push("owner", o)[0], "applied")
		wantStatus(t, w.push("owner", o)[0], "duplicate")
	})
}

func TestRejections(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		h := hlcAt(now, "a")
		p1, p2, s2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("project", p1, h, newProject("One")),
			op("project", p2, h, newProject("Two")),
			op("status", s2, h, newStatus(p2, "Doing", "in-progress")),
		)
		task := uuid.NewString()
		wantStatus(t, w.push("owner", op("task", task, h, newTask(p1, nil, "Ok")))[0], "applied")
		later := hlcAt(now.Add(time.Second), "a")

		cases := map[string]syncer.Op{
			"unknown entity":    op("spaceship", uuid.NewString(), h, map[string]any{}),
			"unknown field":     op("task", task, later, map[string]any{"color": "red"}),
			"empty title":       op("task", task, later, map[string]any{"title": ""}),
			"padded title":      op("task", task, later, map[string]any{"title": " x "}),
			"bad enum":          op("project", p1, later, map[string]any{"color": "mauve"}),
			"missing required":  op("task", uuid.NewString(), later, map[string]any{"title": "No sort key"}),
			"dangling ref":      op("task", task, later, map[string]any{"project_id": uuid.NewString()}),
			"foreign status":    op("task", task, later, map[string]any{"status_id": s2}),
			"immutable change":  op("status", s2, later, map[string]any{"project_id": p1}),
			"clock from future": op("task", task, hlcAt(now.Add(time.Hour), "a"), map[string]any{"title": "Later"}),
			"bad clock":         op("task", task, "yesterday", map[string]any{"title": "Later"}),
			"bad time":          op("task", task, later, map[string]any{"completed_at": "tuesday"}),
		}
		for name, o := range cases {
			t.Run(name, func(t *testing.T) {
				res := w.push("owner", o)[0]
				wantStatus(t, res, "rejected")
				if res.Message == "" {
					t.Error("a rejection must say why")
				}
			})
		}
		// A rejected edit comes back with the server's row to revert to.
		res := w.push("owner", op("task", task, later, map[string]any{"title": ""}))[0]
		if res.Row == nil || res.Row.Fields["title"] != "Ok" {
			t.Fatalf("rejection row = %+v", res.Row)
		}
		// Moving a task needs its status moved with it.
		wantStatus(t, w.push("owner", op("task", task, later, map[string]any{"project_id": p2, "status_id": s2}))[0], "applied")
	})
}

func TestGuestsSeeAndChangeOnlySharedProjects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		h := hlcAt(time.Now(), "a")
		shared, private := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("project", shared, h, newProject("Shared")),
			op("project", private, h, newProject("Private")),
			op("task", uuid.NewString(), h, newTask(shared, nil, "Visible")),
			op("task", uuid.NewString(), h, newTask(private, nil, "Hidden")),
			op("task", uuid.NewString(), h, newTask(nil, nil, "Inbox")),
		)
		_, err := d.ExecContext(ctx, rebind(d, "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"), shared, w.users["guest"], store.Now())
		must(t, err)

		snap, err := w.svc.Bootstrap(ctx, w.users["guest"], w.ws)
		must(t, err)
		var titles []string
		for _, r := range snap.Rows {
			if r.Entity == "task" {
				titles = append(titles, r.Fields["title"].(string))
			}
		}
		if !reflect.DeepEqual(titles, []string{"Visible"}) {
			t.Fatalf("guest sees tasks %v", titles)
		}
		changes, err := w.svc.Pull(ctx, w.users["guest"], w.ws, 0)
		must(t, err)
		if len(changes.Changes) != 2 || changes.Seq != 5 {
			t.Fatalf("guest pull = %d changes up to %d, want 2 up to 5", len(changes.Changes), changes.Seq)
		}

		later := hlcAt(time.Now().Add(time.Second), "g")
		wantStatus(t, w.push("guest", op("task", uuid.NewString(), later, newTask(shared, nil, "Guest task")))[0], "applied")
		wantStatus(t, w.push("guest", op("task", uuid.NewString(), later, newTask(private, nil, "Sneaky")))[0], "rejected")
		wantStatus(t, w.push("guest", op("project", shared, later, map[string]any{"name": "Mine"}))[0], "rejected")
	})
}

func rebind(d *db.DB, q string) string {
	if d.Dialect != db.Postgres {
		return q
	}
	out, n := []byte{}, 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			out = append(out, '$')
			out = append(out, strconv.Itoa(n)...)
			continue
		}
		out = append(out, q[i])
	}
	return string(out)
}

// replica is a simulated device: a local copy, an outbox and a clock,
// following the same rules as web/src/lib/sync.
type replica struct {
	stats   *stats
	role    string
	clock   clock
	rows    map[string]*localRow // entity/id
	outbox  []syncer.Op
	lastSeq int64
}

type localRow struct {
	entity string
	values map[string]any
	clocks map[string]string
}

func key(entity, id string) string { return entity + "/" + id }

func (r *replica) edit(now int64, entity, id string, fields map[string]any) {
	h := r.clock.tick(now)
	row := r.rows[key(entity, id)]
	if row == nil {
		row = &localRow{entity: entity, values: map[string]any{}, clocks: map[string]string{}}
		r.rows[key(entity, id)] = row
	}
	for k, v := range fields {
		if _, set := row.clocks[k]; set && firstWins[entity+"/"+k] {
			continue // the server keeps the older
		}
		row.values[k] = v
		row.clocks[k] = h
	}
	r.outbox = append(r.outbox, op(entity, id, h, fields))
}

// firstWins are the fields where the oldest write wins (FIRST_WINS in
// web/src/lib/sync/entities.ts), keyed entity/field.
var firstWins = map[string]bool{"task/created_by": true}

// applyRemote takes each field only if it is newer than the local one, or
// older for a firstWins field.
func (r *replica) applyRemote(entity, id string, fields map[string]any, clocks map[string]string) {
	row := r.rows[key(entity, id)]
	if row == nil {
		row = &localRow{entity: entity, values: map[string]any{}, clocks: map[string]string{}}
		r.rows[key(entity, id)] = row
	}
	for k, v := range fields {
		// A field nothing has written yet (one only the server sets) has no
		// clock, and the app takes nothing from it, as here.
		if clocks[k] == "" {
			continue
		}
		r.clock.observe(clocks[k])
		mine, ok := row.clocks[k]
		if firstWins[entity+"/"+k] && (!ok || clocks[k] < mine) || !firstWins[entity+"/"+k] && clocks[k] > mine {
			row.values[k] = v
			row.clocks[k] = clocks[k]
		}
	}
}

// stats counts what a run exercised, so a run that tests nothing fails.
type stats struct {
	rejected, duplicate, applied int
	// newest is the highest HLC of any accepted write to each field,
	// keyed entity/id/field: the value last-writer-wins must keep. For a
	// firstWins field it is the lowest.
	newest map[string]string
	// creators of each next task made under a derived ID, by device.
	creators map[string]map[string]bool
	// shares counts projects given to or taken from the guest; resets,
	// the times a device found its copy out of date and started again.
	shares, resets int
	// rest and restRefused count writes through REST; lastTask is the
	// newest task op the server took from a device.
	rest, restRefused int
	lastTask          *syncer.Op
}

// push sends the whole outbox. A rejected op's fields fall back to the
// server's copy wherever the rejected op was the last local write.
func (r *replica) push(w *world, dup bool) {
	if len(r.outbox) == 0 {
		return
	}
	ops := r.outbox
	if dup {
		ops = append(append([]syncer.Op{}, ops...), ops[0])
	}
	res := w.push(r.role, ops...)
	for _, x := range res {
		switch x.Status {
		case "rejected":
			r.stats.rejected++
		case "duplicate":
			r.stats.duplicate++
		default:
			r.stats.applied++
		}
	}
	for i, o := range ops {
		if res[i].Status == "rejected" {
			continue
		}
		if o.Entity == "task" {
			r.stats.lastTask = &ops[i]
		}
		for name := range o.Fields {
			k := key(o.Entity, o.ID) + "/" + name
			cur, seen := r.stats.newest[k]
			if firstWins[o.Entity+"/"+name] && (!seen || o.HLC < cur) || !firstWins[o.Entity+"/"+name] && o.HLC > cur {
				r.stats.newest[k] = o.HLC
			}
		}
	}
	for i, o := range r.outbox {
		if res[i].Status != "rejected" {
			continue
		}
		row := r.rows[key(o.Entity, o.ID)]
		for name := range o.Fields {
			if row.clocks[name] != o.HLC {
				continue
			}
			if res[i].Row == nil {
				delete(row.values, name)
				delete(row.clocks, name)
				continue
			}
			row.values[name] = res[i].Row.Fields[name]
			row.clocks[name] = res[i].Row.Clocks[name]
		}
		if len(row.values) == 0 {
			delete(r.rows, key(o.Entity, o.ID))
		}
	}
	r.outbox = nil
}

func (r *replica) pull(w *world) {
	for {
		changes, err := w.svc.Pull(context.Background(), w.users[r.role], w.ws, r.lastSeq)
		if apperr.KindOf(err) == apperr.Gone {
			r.stats.resets++
			r.bootstrap(w)
			return
		}
		must(w.t, err)
		for _, c := range changes.Changes {
			r.applyRemote(c.Entity, c.ID, c.Fields, c.Clocks)
		}
		r.lastSeq = changes.Seq
		if !changes.More {
			return
		}
	}
}

// bootstrap replaces the copy with the server's, then replays the unsent
// outbox on top, as the browser does when a pull answers 410.
func (r *replica) bootstrap(w *world) {
	snap, err := w.svc.Bootstrap(context.Background(), w.users[r.role], w.ws)
	must(w.t, err)
	r.rows = map[string]*localRow{}
	for _, row := range snap.Rows {
		r.applyRemote(row.Entity, row.ID, row.Fields, row.Clocks)
	}
	for _, o := range r.outbox {
		fields, clocks := map[string]any{}, map[string]string{}
		for k, raw := range o.Fields {
			var v any
			must(w.t, json.Unmarshal(raw, &v))
			fields[k], clocks[k] = v, o.HLC
		}
		r.applyRemote(o.Entity, o.ID, fields, clocks)
	}
	r.lastSeq = snap.Seq
}

// share gives the guest a project or takes it away, as accounts does:
// the access row and a reset of the guest's copy in one transaction.
func (w *world) share(project string, on bool) {
	ctx := context.Background()
	must(w.t, w.st.InTx(ctx, func(q store.Queries) error {
		var err error
		if on {
			_, err = q.GrantProjectAccess(ctx, sqlitedb.GrantProjectAccessParams{ProjectID: project, UserID: w.users["guest"], Now: store.Now()})
		} else {
			_, err = q.RevokeProjectAccess(ctx, sqlitedb.RevokeProjectAccessParams{ProjectID: project, UserID: w.users["guest"]})
		}
		if err != nil {
			return err
		}
		return store.ResetMember(ctx, q, w.ws, w.users["guest"])
	}))
}

// live is the replica's rows that are not deleted, as the server would
// show them: rows hanging off a deleted task are hidden with it.
func (r *replica) live() map[string]localRow {
	out := map[string]localRow{}
	for k, row := range r.rows {
		if row.values["deleted_at"] != nil {
			continue
		}
		if task, ok := row.values["task_id"].(string); ok {
			if t := r.rows[key("task", task)]; t == nil || t.values["deleted_at"] != nil {
				continue
			}
		}
		out[k] = *row
	}
	return out
}

func TestConvergence(t *testing.T) {
	rounds := 300
	if s := os.Getenv("MILA_FUZZ_ROUNDS"); s != "" {
		rounds, _ = strconv.Atoi(s)
	}
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		for _, seed := range []uint64{1, 2, 3} {
			t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
				converge(t, d, seed, rounds)
			})
		}
	})
}

func converge(t *testing.T, d *db.DB, seed uint64, rounds int) {
	ctx := context.Background()
	w := newWorld(t, d)
	rng := rand.New(rand.NewPCG(seed, 99))
	script := rand.New(rand.NewPCG(seed, 7))
	base := time.Now().Add(-30 * time.Second).UnixMilli()
	st := &stats{newest: map[string]string{}, creators: map[string]map[string]bool{}}
	devices := []*replica{
		{stats: st, role: "owner", clock: clock{node: "owner"}, rows: map[string]*localRow{}},
		{stats: st, role: "owner", clock: clock{node: "laptop"}, rows: map[string]*localRow{}},
		{stats: st, role: "member", clock: clock{node: "member"}, rows: map[string]*localRow{}},
		// The guest is given projects and loses them partway through.
		{stats: st, role: "guest", clock: clock{node: "guest"}, rows: map[string]*localRow{}},
	}
	shared := map[string]bool{}
	offline := make([]bool, len(devices))

	// Two projects with statuses, made on the first device and synced.
	type proj struct{ id, todo, done string }
	projects := []proj{}
	first := devices[0]
	for i := range 2 {
		p := proj{uuid.NewString(), uuid.NewString(), uuid.NewString()}
		first.edit(base, "project", p.id, newProject(fmt.Sprintf("Project %d", i)))
		first.edit(base, "status", p.todo, newStatus(p.id, "To do", "todo"))
		first.edit(base, "status", p.done, newStatus(p.id, "Done", "done"))
		projects = append(projects, p)
	}
	labels := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range labels {
		first.edit(base, "label", id, map[string]any{"name": fmt.Sprintf("Label %d", i), "color": "red", "sort_key": "a0", "deleted_at": nil})
	}
	first.push(w, false)
	for _, dev := range devices {
		dev.pull(w)
	}
	var children []struct{ entity, id string }

	var tasks []string
	ms := base
	for range rounds {
		ms += int64(rng.IntN(5)) // devices often act in the same millisecond
		i := rng.IntN(len(devices))
		dev := devices[i]
		// A script writes through REST now and then, with a generator of
		// its own so the devices' runs play out as they did before it.
		if len(tasks) > 0 && script.IntN(4) == 0 {
			p := projects[script.IntN(len(projects))]
			id := tasks[script.IntN(len(tasks))]
			// Often the task a device just wrote, so the two contend.
			var after map[string]json.RawMessage
			if st.lastTask != nil && script.IntN(2) == 0 {
				id, after = st.lastTask.ID, st.lastTask.Fields
			}
			switch script.IntN(10) {
			case 0:
				made := uuid.NewString()
				if restWrite(t, w, st, script, ms, syncer.Write{Entity: "task", ID: made, Mode: syncer.Create,
					Fields: restFields(map[string]any{"title": "From a script", "project_id": p.id, "status_id": p.todo})}) {
					tasks = append(tasks, made)
				}
			case 1:
				restWrite(t, w, st, script, ms, syncer.Write{Entity: "task", ID: id, Mode: syncer.Delete})
			case 2, 3:
				// A label on the task, added (or brought back) or taken off.
				label := labels[script.IntN(len(labels))]
				if script.IntN(2) == 0 {
					restWrite(t, w, st, script, ms, syncer.Write{Entity: "task_label", Mode: syncer.Upsert,
						Fields: restFields(map[string]any{"task_id": id, "label_id": label})})
				} else {
					restWrite(t, w, st, script, ms, syncer.Write{Entity: "task_label", ID: pairID(id, label), Mode: syncer.Delete})
				}
			default:
				restWrite(t, w, st, script, ms, scriptUpdate(script, ms, id, p.id, p.todo, after))
			}
		}
		switch roll := rng.IntN(100); {
		case roll < 8:
			offline[i] = !offline[i]
		case roll < 20 && !offline[i]:
			dev.push(w, rng.IntN(4) == 0)
		case roll < 32 && !offline[i]:
			dev.pull(w)
		case roll >= 91 && roll < 94:
			p := projects[rng.IntN(len(projects))].id
			shared[p] = !shared[p]
			w.share(p, shared[p])
			st.shares++
		// The first few tasks repeat: completing one makes the next under an
		// ID every device derives alike, so devices completing it apart race
		// to create the same row.
		case roll >= 94 && len(tasks) > 0:
			id := tasks[rng.IntN(min(3, len(tasks)))]
			t := dev.rows[key("task", id)]
			if t == nil {
				continue
			}
			dev.edit(ms, "task", id, map[string]any{"completed_at": time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)})
			next := recur.NextTaskID(id)
			series := t.values["series_id"]
			if series == nil {
				series = id
			}
			dev.edit(ms, "task", next, merge(newTask(t.values["project_id"], t.values["status_id"], "Next"), map[string]any{"created_by": w.users[dev.role], "series_id": series}))
			if st.creators[next] == nil {
				st.creators[next] = map[string]bool{}
			}
			st.creators[next][dev.clock.node] = true
		// A small pool of tasks, so devices keep colliding on the same fields.
		case len(tasks) == 0 || (roll < 47 && len(tasks) < 15):
			id := uuid.NewString()
			p := projects[rng.IntN(len(projects))]
			var project, status any = p.id, p.todo
			if rng.IntN(3) == 0 {
				project, status = nil, nil // inbox
			}
			dev.edit(ms, "task", id, newTask(project, status, fmt.Sprintf("Task %d", len(tasks))))
			tasks = append(tasks, id)
		default:
			id := tasks[rng.IntN(len(tasks))]
			if dev.rows[key("task", id)] == nil {
				continue // this device has not seen the task yet
			}
			if rng.IntN(3) == 0 {
				// Something hanging off the task.
				switch rng.IntN(6) {
				case 0:
					c := uuid.NewString()
					dev.edit(ms, "checklist_item", c, map[string]any{"task_id": id, "text": "Step", "done": false, "sort_key": "a0", "deleted_at": nil})
					children = append(children, struct{ entity, id string }{"checklist_item", c})
				case 1:
					label := labels[rng.IntN(len(labels))]
					var del any
					if rng.IntN(2) == 0 {
						del = time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
					}
					dev.edit(ms, "task_label", pairID(id, label), map[string]any{"task_id": id, "label_id": label, "deleted_at": del})
				case 2:
					c := uuid.NewString()
					dev.edit(ms, "comment", c, map[string]any{"task_id": id, "author_id": w.users[dev.role], "body": "Looks good", "edited_at": nil, "deleted_at": nil})
					children = append(children, struct{ entity, id string }{"comment", c})
				case 3:
					t := dev.rows[key("task", id)]
					sub := uuid.NewString()
					dev.edit(ms, "task", sub, merge(newTask(t.values["project_id"], t.values["status_id"], "Subtask"), map[string]any{"parent_id": id}))
					tasks = append(tasks, sub)
				default:
					if len(children) == 0 {
						continue
					}
					c := children[rng.IntN(len(children))]
					if dev.rows[key(c.entity, c.id)] == nil {
						continue
					}
					if c.entity == "checklist_item" {
						dev.edit(ms, c.entity, c.id, map[string]any{"done": rng.IntN(2) == 0})
					} else {
						// Another member editing someone's comment is refused.
						dev.edit(ms, c.entity, c.id, map[string]any{"body": fmt.Sprintf("Edited %d", rng.IntN(100)), "edited_at": time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)})
					}
				}
				continue
			}
			switch rng.IntN(10) {
			case 0:
				dev.edit(ms, "task", id, map[string]any{"title": fmt.Sprintf("Renamed %d", rng.IntN(1000))})
			case 1:
				dev.edit(ms, "task", id, map[string]any{"notes": fmt.Sprintf("Note %d", rng.IntN(1000))})
			case 2:
				var at any
				if rng.IntN(2) == 0 {
					at = time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
				}
				dev.edit(ms, "task", id, map[string]any{"completed_at": at})
			case 3:
				p := projects[rng.IntN(len(projects))]
				dev.edit(ms, "task", id, map[string]any{"project_id": p.id, "status_id": p.done})
			case 4:
				if rng.IntN(3) == 0 {
					dev.edit(ms, "task", id, map[string]any{"deleted_at": time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)})
				}
			case 5:
				dev.edit(ms, "task", id, map[string]any{"title": ""}) // invalid: rejected and reverted
			case 6:
				dev.edit(ms, "task", id, map[string]any{"priority": []string{"none", "low", "high", "urgent"}[rng.IntN(4)]})
			case 7:
				// Picked from the clock, not the generator, so the seeds play out
				// as they did before repeat rules existed.
				if ms%2 == 0 {
					dev.edit(ms, "task", id, map[string]any{"recurrence": []any{nil, "FREQ=DAILY", "FREQ=WEEKLY;BYDAY=MO,TH"}[ms%3], "recurrence_mode": []string{"schedule", "completion"}[ms/2%2]})
					continue
				}
				dev.edit(ms, "task", id, map[string]any{"due_date": fmt.Sprintf("2026-11-%02d", 1+rng.IntN(28)), "due_time": "09:30"})
			default:
				dev.edit(ms, "task", id, map[string]any{"title": fmt.Sprintf("Edit %d", rng.IntN(1000)), "notes": fmt.Sprintf("Both %d", rng.IntN(1000))})
			}
		}
	}

	// Everyone comes online and syncs; a second round catches changes
	// pushed after an earlier device pulled.
	for range 2 {
		for _, dev := range devices {
			dev.push(w, false)
			dev.pull(w)
		}
	}

	// What the server shows each person: the live rows of what a new
	// device of theirs would get, which holds the trash too.
	view := func(role string) map[string]localRow {
		snap, err := w.svc.Bootstrap(ctx, w.users[role], w.ws)
		must(t, err)
		fresh := &replica{rows: map[string]*localRow{}}
		for _, r := range snap.Rows {
			fresh.rows[key(r.Entity, r.ID)] = &localRow{entity: r.Entity, values: r.Fields, clocks: r.Clocks}
		}
		return fresh.live()
	}
	server := view("owner")
	// Last writer wins: every field holds the newest accepted write (the
	// oldest for a firstWins field).
	contested := 0
	for k, want := range st.newest {
		ent, rest, _ := cut(k)
		id, name, _ := cut(rest)
		row, ok := server[key(ent, id)]
		if !ok {
			continue // deleted
		}
		if got := row.clocks[name]; got != want {
			t.Fatalf("%s keeps the write from %s, but the newest accepted write was %s", k, got, want)
		}
		contested++
	}
	raced := 0
	for _, by := range st.creators {
		if len(by) > 1 {
			raced++
		}
	}
	guestSees := view("guest")
	if st.rejected == 0 || st.duplicate == 0 || st.applied < rounds/4 || len(server) < 10 || contested < 40 || raced == 0 ||
		st.shares < 2 || st.resets < 2 || st.rest < rounds/30 {
		t.Fatalf("the run exercised too little: %d applied, %d rejected, %d duplicates, %d contested fields, %d live rows, %d tasks made on two devices, %d shares, %d resets, %d REST writes",
			st.applied, st.rejected, st.duplicate, contested, len(server), raced, st.shares, st.resets, st.rest)
	}
	for n, dev := range devices {
		server := server
		if dev.role == "guest" {
			server = guestSees
		}
		got := dev.live()
		if len(got) != len(server) {
			var extra, missing []string
			for k := range got {
				if _, ok := server[k]; !ok {
					extra = append(extra, k)
				}
			}
			for k := range server {
				if _, ok := got[k]; !ok {
					missing = append(missing, k)
				}
			}
			sort.Strings(extra)
			sort.Strings(missing)
			t.Fatalf("device %d has %d live rows, server has %d; only on the device: %v; only on the server: %v",
				n, len(got), len(server), extra, missing)
		}
		keys := make([]string, 0, len(server))
		for k := range server {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			want, have := server[k], got[k]
			// A field nothing has written (one only the server sets, not yet
			// set) is empty on the server and absent on a device.
			for name, v := range want.values {
				if _, clocked := want.clocks[name]; !clocked && v == nil {
					if _, held := have.values[name]; !held {
						want.values = maps.Clone(want.values)
						delete(want.values, name)
					}
				}
			}
			if !reflect.DeepEqual(have.values, want.values) || !reflect.DeepEqual(have.clocks, want.clocks) {
				t.Fatalf("device %d differs from the server on %s:\n device %v %v\n server %v %v", n, k, have.values, have.clocks, want.values, want.clocks)
			}
		}
	}
}

// restWrite is a script writing through REST beside the devices, stamped
// by the server's clock: here the simulated one, a little behind the
// devices', as a device's clock may be ahead of the server's. It makes
// tasks, changes them, deletes them, and adds and removes their labels.
// Its writes count toward last-writer-wins like any device's, and it gets
// back every value it sent. It reports whether the server took the write.
func restWrite(t *testing.T, w *world, st *stats, rng *rand.Rand, ms int64, wr syncer.Write) bool {
	t.Helper()
	role := []string{"owner", "member"}[rng.IntN(2)]
	behind := int64(rng.IntN(20))
	w.svc.SetNow(func() time.Time { return time.UnixMilli(ms - behind).UTC() })
	rec, _, err := w.svc.Write(context.Background(), w.users[role], w.ws, wr)
	w.svc.SetNow(store.Now)
	if apperr.KindOf(err) != 0 {
		st.restRefused++ // a task not pushed yet, or a subtask moved alone
		return false
	}
	must(t, err)
	st.rest++
	for name, sent := range wr.Fields {
		got, _ := json.Marshal(rec.Fields[name])
		if string(got) != string(sent) {
			t.Fatalf("a REST %s of %s %s sent %s %s and got back %s", wr.Mode, wr.Entity, rec.ID, name, sent, got)
		}
	}
	written := slices.Collect(maps.Keys(wr.Fields))
	switch wr.Mode {
	case syncer.Create:
		written = slices.Collect(maps.Keys(rec.Fields))
	case syncer.Delete, syncer.Upsert:
		written = append(written, "deleted_at")
	}
	if wr.Mode == syncer.Delete && rec.Fields["deleted_at"] == nil || wr.Mode == syncer.Upsert && rec.Fields["deleted_at"] != nil {
		t.Fatalf("a REST %s of %s %s left deleted_at %v", wr.Mode, wr.Entity, rec.ID, rec.Fields["deleted_at"])
	}
	for _, name := range written {
		k := key(wr.Entity, rec.ID) + "/" + name
		c := rec.Clocks[name]
		if c == "" {
			continue // a field only the server sets, which it hasn't yet
		}
		// Every field it wrote is stamped now or later, even one it left as
		// it was: an older edit arriving afterward must not undo it. The
		// pair's own fields are fixed and left alone.
		if stamped, _ := strconv.ParseInt(c[:13], 10, 64); stamped < ms-behind && (wr.Mode != syncer.Upsert || name == "deleted_at") {
			t.Fatalf("a REST %s of %s %s left %s at %s, older than the server's clock", wr.Mode, wr.Entity, rec.ID, name, c)
		}
		if cur, seen := st.newest[k]; firstWins[wr.Entity+"/"+name] && (!seen || c < cur) || !firstWins[wr.Entity+"/"+name] && c > cur {
			st.newest[k] = c
		}
	}
	return true
}

// scriptUpdate is a change to a task: often to a field the device's op
// it follows wrote, so the two contend.
func scriptUpdate(rng *rand.Rand, ms int64, id, project, status string, after map[string]json.RawMessage) syncer.Write {
	var values map[string]any
	pick := rng.IntN(5)
	for i, name := range []string{"title", "notes", "project_id", "due_date", "completed_at"} {
		if _, ok := after[name]; ok {
			pick = i
			break
		}
	}
	switch pick {
	case 0:
		values = map[string]any{"title": fmt.Sprintf("Scripted %d", rng.IntN(1000))}
	case 1:
		values = map[string]any{"notes": fmt.Sprintf("Script %d", rng.IntN(1000)), "priority": []string{"none", "low", "medium"}[rng.IntN(3)]}
	case 2:
		values = map[string]any{"project_id": project, "status_id": status}
	case 3:
		values = map[string]any{"due_date": fmt.Sprintf("2026-12-%02d", 1+rng.IntN(28))}
	default:
		var at any
		if rng.IntN(2) == 0 {
			at = time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
		}
		values = map[string]any{"completed_at": at}
	}
	return syncer.Write{Entity: "task", ID: id, Mode: syncer.Update, Fields: restFields(values)}
}

func restFields(values map[string]any) map[string]json.RawMessage {
	raw := map[string]json.RawMessage{}
	for k, v := range values {
		raw[k], _ = json.Marshal(v)
	}
	return raw
}

func cut(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// The browser's clock rules are checked against the same cases.
func TestSharedHLCVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../shared/sync-vectors.json")
	must(t, err)
	var v struct {
		Valid, Invalid, Ordered []string
	}
	must(t, json.Unmarshal(raw, &v))
	now := time.UnixMilli(1793237400000)
	for _, h := range v.Valid {
		if err := syncer.CheckHLC(h, now); err != nil {
			t.Errorf("%q should be valid: %v", h, err)
		}
	}
	for _, h := range v.Invalid {
		if syncer.CheckHLC(h, now) == nil {
			t.Errorf("%q should be invalid", h)
		}
	}
	if !sort.StringsAreSorted(v.Ordered) {
		t.Error("ordered vectors are not in string order")
	}
}

func merge(a, b map[string]any) map[string]any {
	for k, v := range b {
		a[k] = v
	}
	return a
}

// A guest's edit that lost to a newer one, to a task they no longer see,
// is refused rather than taken as a change that did nothing: their device
// drops its copy of the edit instead of keeping a row it cannot see.
func TestGuestStaleEditAfterLosingAccess(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		p, s, task := uuid.NewString(), uuid.NewString(), uuid.NewString()
		for _, r := range w.push("owner",
			op("project", p, hlcAt(now, "a"), newProject("Shared")),
			op("status", s, hlcAt(now, "a"), newStatus(p, "To do", "todo")),
			op("task", task, hlcAt(now, "a"), newTask(p, s, "Plant tomatoes")),
		) {
			wantStatus(t, r, "applied")
		}
		w.share(p, true)
		// The guest edits offline; the owner edits the same field later.
		stale := op("task", task, hlcAt(now.Add(time.Second), "guest"), map[string]any{"title": "Mine"})
		wantStatus(t, w.push("owner", op("task", task, hlcAt(now.Add(2*time.Second), "a"), map[string]any{"title": "Theirs"}))[0], "applied")
		w.share(p, false)
		res := w.push("guest", stale)[0]
		if res.Status != "rejected" || res.Row != nil {
			t.Fatalf("stale edit by a guest without access = %s, row %v; want rejected with no row", res.Status, res.Row)
		}
		// With access, the same kind of edit is simply outdated.
		w.share(p, true)
		late := op("task", task, hlcAt(now.Add(time.Second), "guest"), map[string]any{"title": "Mine again"})
		wantStatus(t, w.push("guest", late)[0], "applied")
	})
}

// A new device gets the trash with everything else, so a task restored
// later comes back whole, its checklist with it, rather than as a row
// holding only deleted_at.
func TestBootstrapHoldsTheTrash(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		w := newWorld(t, d)
		now := time.Now()
		task, item := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", task, hlcAt(now, "a"), newTask(nil, nil, "Plant tomatoes")),
			op("checklist_item", item, hlcAt(now, "a"), map[string]any{"task_id": task, "text": "Dig", "done": false, "sort_key": "a0", "deleted_at": nil}),
			op("task", task, hlcAt(now.Add(time.Second), "a"), map[string]any{"deleted_at": now.UTC().Format(time.RFC3339Nano)}),
		)
		snap, err := w.svc.Bootstrap(ctx, w.users["member"], w.ws)
		must(t, err)
		got := map[string]map[string]any{}
		for _, r := range snap.Rows {
			got[r.Entity] = r.Fields
		}
		if got["task"]["title"] != "Plant tomatoes" || got["task"]["deleted_at"] == nil || got["checklist_item"]["text"] != "Dig" {
			t.Fatalf("bootstrap = %v, want the deleted task and its checklist", got)
		}
	})
}
