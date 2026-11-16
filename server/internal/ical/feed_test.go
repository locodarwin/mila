package ical

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

const base = "https://mila.example.com"

// world is a workspace with an owner, a member and a guest, the project
// Home shared with the guest and Work not, and a feed service whose clock
// says 2026-10-29 at noon in Chicago.
type world struct {
	t     *testing.T
	ctx   context.Context
	st    *store.Store
	sync  *syncer.Service
	svc   *Service
	ws    string
	home  string
	work  string
	users map[string]string
	seq   int
}

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &world{t: t, ctx: ctx, st: st, ws: uuid.NewString(), users: map[string]string{}}
	w.sync = syncer.New(st, log, nil)
	w.svc = New(st, w.sync, log, base)
	w.svc.SetClock(func() time.Time { return time.Date(2026, 10, 29, 17, 0, 0, 0, time.UTC) })
	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member", "guest"} {
		id := uuid.NewString()
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "-" + id[:8] + "@example.com", Name: role, Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: id, Role: role, Now: now}))
		w.users[role] = id
		_, err := accounts.UpdateSettings(ctx, st, id, func(s accounts.Settings) error {
			s["timezone"] = json.RawMessage(`"America/Chicago"`)
			return nil
		})
		must(t, err)
	}
	w.home, w.work = uuid.NewString(), uuid.NewString()
	w.push("owner", "project", w.home, map[string]any{"name": "Home", "sort_key": "a0"})
	w.push("owner", "project", w.work, map[string]any{"name": "Work", "sort_key": "a1"})
	q := "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"
	if d.Dialect == db.Postgres {
		q = "INSERT INTO project_access (project_id, user_id, created_at) VALUES ($1, $2, $3)"
	}
	_, err := d.ExecContext(ctx, q, w.home, w.users["guest"], now)
	must(t, err)
	return w
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (w *world) push(role, entity, id string, fields map[string]any) {
	w.t.Helper()
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	w.seq++
	hlc := fmt.Sprintf("%013d-%04x-%s", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()+int64(w.seq)*1000, w.seq, role)
	res, _, err := w.sync.Push(w.ctx, w.users[role], w.ws, []syncer.Op{{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw}})
	must(w.t, err)
	if res[0].Status != "applied" {
		w.t.Fatalf("%s %s: %s %s", role, entity, res[0].Status, res[0].Message)
	}
}

func pairID(a, b string) string { return ids.Pair(a, b) }

// task makes a task; extra may hold "assign" (a role or list of roles).
func (w *world) task(project, title string, extra map[string]any) string {
	id := uuid.NewString()
	fields := map[string]any{"title": title, "sort_key": "a0", "created_by": w.users["owner"]}
	if project != "" {
		fields["project_id"] = project
	}
	var assign []string
	for k, v := range extra {
		switch k {
		case "assign":
			assign = strings.Split(v.(string), ",")
		default:
			fields[k] = v
		}
	}
	w.push("owner", "task", id, fields)
	for _, role := range assign {
		w.push("owner", "task_assignee", pairID(id, w.users[role]), map[string]any{"task_id": id, "user_id": w.users[role]})
	}
	return id
}

func (w *world) create(role, scope, target string) (Feed, string) {
	w.t.Helper()
	f, link, err := w.svc.Create(w.ctx, w.users[role], w.ws, scope, target)
	must(w.t, err)
	token, ok := strings.CutPrefix(link, base+"/ical/")
	if !ok || !strings.HasSuffix(token, ".ics") {
		w.t.Fatalf("link = %q", link)
	}
	return f, strings.TrimSuffix(token, ".ics")
}

// events fetches a feed and returns its events by summary, each as its
// property lines.
func (w *world) events(token string) map[string]map[string]string {
	w.t.Helper()
	body, err := w.svc.Calendar(w.ctx, token)
	must(w.t, err)
	out := map[string]map[string]string{}
	var cur map[string]string
	for _, p := range parse(w.t, body) {
		switch {
		case p.name == "BEGIN" && p.value == "VEVENT":
			cur = map[string]string{}
		case p.name == "END" && p.value == "VEVENT":
			out[unescape(w.t, cur["SUMMARY"])] = cur
			cur = nil
		case cur != nil:
			cur[p.name] = p.value
		}
	}
	return out
}

func (w *world) gone(token string) {
	w.t.Helper()
	if _, err := w.svc.Calendar(w.ctx, token); apperr.KindOf(err) != apperr.NotFound {
		w.t.Fatalf("got %v, want not found", err)
	}
}

func keys(m map[string]map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestFeeds(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		errand, heavy := uuid.NewString(), uuid.NewString()
		w.push("owner", "label", errand, map[string]any{"name": "Errand", "sort_key": "a0"})
		w.push("owner", "label", heavy, map[string]any{"name": "Big, heavy; awkward", "sort_key": "a1"})

		groceries := w.task(w.home, "Groceries", map[string]any{"due_date": "2026-11-02", "assign": "owner",
			"notes": "Milk, eggs\nand bread"})
		w.push("owner", "task_label", pairID(groceries, heavy), map[string]any{"task_id": groceries, "label_id": heavy})
		w.push("owner", "task_label", pairID(groceries, errand), map[string]any{"task_id": groceries, "label_id": errand})
		// 14:30 in Chicago on the 3rd, after the clocks go back: 20:30 UTC.
		w.task(w.work, "Standup notes", map[string]any{"due_date": "2026-11-03", "due_time": "14:30", "assign": "owner,member"})
		w.task(w.home, "Dana's chore", map[string]any{"due_date": "2026-11-04", "assign": "member"})
		w.task(w.home, "Done already", map[string]any{"due_date": "2026-11-01", "assign": "owner", "completed_at": "2026-10-28T10:00:00Z"})
		w.task(w.home, "No date", map[string]any{"assign": "owner"})
		w.task(w.home, "Bins", map[string]any{"due_date": "2026-11-02", "assign": "owner",
			"recurrence": "FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231"})
		w.task(w.work, "Pills", map[string]any{"due_date": "2026-11-01", "due_time": "09:00", "assign": "owner",
			"recurrence": "FREQ=DAILY;UNTIL=20261231"})
		w.task(w.home, "Water plants", map[string]any{"due_date": "2026-11-05", "assign": "owner",
			"recurrence": "FREQ=DAILY;INTERVAL=3", "recurrence_mode": "completion"})
		w.task("", "Call the bank", map[string]any{"due_date": "2026-11-06", "assign": "owner"})
		w.task(w.home, "Buy milk", map[string]any{"due_date": "2026-11-02", "assign": "owner", "parent_id": groceries})
		// My Day: undated tasks go on their day, or today while carried over.
		today := w.task(w.home, "Tidy desk", nil)
		later := w.task(w.home, "Plan trip", nil)
		mine := w.task(w.home, "Theirs only", nil)
		w.push("owner", "my_day_entry", syncer.PairID(today, w.users["owner"]), map[string]any{"task_id": today, "user_id": w.users["owner"], "day": "2026-10-20", "sort_key": "a0"})
		w.push("owner", "my_day_entry", syncer.PairID(later, w.users["owner"]), map[string]any{"task_id": later, "user_id": w.users["owner"], "day": "2026-10-31", "sort_key": "a1"})
		w.push("member", "my_day_entry", syncer.PairID(mine, w.users["member"]), map[string]any{"task_id": mine, "user_id": w.users["member"], "day": "2026-10-29", "sort_key": "a0"})

		_, me := w.create("owner", ScopeMe, "")
		ev := w.events(me)
		want := []string{"Bins", "Buy milk", "Call the bank", "Groceries", "Pills", "Plan trip", "Standup notes", "Tidy desk", "Water plants"}
		if got := keys(ev); !slices.Equal(got, want) {
			t.Fatalf("my tasks = %v, want %v", got, want)
		}
		g := ev["Groceries"]
		if g["DTSTART;VALUE=DATE"] != "20261102" || g["DTEND;VALUE=DATE"] != "20261103" ||
			g["UID"] != groceries+"@mila.example.com" ||
			g["URL"] != base+"/?task="+groceries+"&ws="+w.ws ||
			g["CATEGORIES"] != `Errand,Big\, heavy\; awkward` {
			t.Errorf("Groceries = %v", g)
		}
		if desc := unescape(t, g["DESCRIPTION"]); desc != "Milk, eggs\nand bread\n\nProject: Home\nOpen in Mila: "+base+"/?task="+groceries+"&ws="+w.ws {
			t.Errorf("description = %q", desc)
		}
		if g["DTSTAMP"] == "" || g["DTSTAMP"] != g["LAST-MODIFIED"] || !strings.HasPrefix(g["DTSTAMP"], "202610") {
			t.Errorf("stamps = %v", g)
		}
		if s := ev["Standup notes"]; s["DTSTART"] != "20261103T203000Z" || s["DTEND"] != "20261103T210000Z" || s["RRULE"] != "" {
			t.Errorf("Standup = %v", s)
		}
		if b := ev["Bins"]; b["RRULE"] != "FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231" || b["DTSTART;VALUE=DATE"] != "20261102" {
			t.Errorf("Bins = %v", b)
		}
		// A repeating task with a time is in the owner's zone, and UNTIL is
		// the end of that day there, in UTC.
		if p := ev["Pills"]; p["DTSTART;TZID=America/Chicago"] != "20261101T090000" ||
			p["DTEND;TZID=America/Chicago"] != "20261101T093000" ||
			p["RRULE"] != "FREQ=DAILY;UNTIL=20270101T055959Z" {
			t.Errorf("Pills = %v", p)
		}
		if p := ev["Water plants"]; p["RRULE"] != "" {
			t.Errorf("a task repeating after completion has a rule: %v", p)
		}
		if c := unescape(t, ev["Call the bank"]["DESCRIPTION"]); !strings.HasPrefix(c, "Project: Inbox\n") {
			t.Errorf("inbox description = %q", c)
		}
		if ev["Tidy desk"]["DTSTART;VALUE=DATE"] != "20261029" || ev["Plan trip"]["DTSTART;VALUE=DATE"] != "20261031" {
			t.Errorf("My Day = %v / %v", ev["Tidy desk"], ev["Plan trip"])
		}
		body, err := w.svc.Calendar(w.ctx, me)
		must(t, err)
		if !strings.Contains(string(body), "BEGIN:VTIMEZONE\r\nTZID:America/Chicago\r\n") ||
			!strings.Contains(string(body), "X-WR-CALNAME:My tasks\r\n") {
			t.Errorf("body = %s", body)
		}
		// Nothing changed, so the body (and its ETag) is the same.
		again, err := w.svc.Calendar(w.ctx, me)
		must(t, err)
		if string(again) != string(body) {
			t.Error("the same feed came out differently")
		}
		feeds, err := w.svc.List(w.ctx, w.users["owner"], w.ws)
		must(t, err)
		if len(feeds) != 1 || feeds[0].LastUsedAt == nil || feeds[0].Name != "My tasks" {
			t.Errorf("feeds = %+v", feeds)
		}

		// A project: its open tasks with a date, subtasks too, anyone's.
		pf, project := w.create("owner", ScopeProject, w.home)
		if pf.Name != "Home" || pf.TargetID != w.home {
			t.Errorf("feed = %+v", pf)
		}
		if got := keys(w.events(project)); !slices.Equal(got, []string{"Bins", "Buy milk", "Dana's chore", "Groceries", "Water plants"}) {
			t.Errorf("Home = %v", got)
		}

		// A shared view: top-level tasks through its filter.
		view := uuid.NewString()
		w.push("owner", "saved_view", view, map[string]any{"name": "Errands", "type": "calendar", "sort_key": "a0",
			"config": map[string]any{"filter": map[string]any{"labels": []string{errand}}}})
		_, vt := w.create("owner", ScopeView, view)
		if got := keys(w.events(vt)); !slices.Equal(got, []string{"Groceries"}) {
			t.Errorf("Errands = %v", got)
		}
		// A label in the trash is on no task, as the view page has it,
		// though the rows that put it there are still live.
		w.push("owner", "label", errand, map[string]any{"deleted_at": "2026-10-29T10:00:00Z"})
		if got := keys(w.events(vt)); len(got) != 0 {
			t.Errorf("Errands with its label in the trash = %v", got)
		}
		// A project view with done tasks shown, through the member's eyes.
		workView := uuid.NewString()
		w.push("owner", "saved_view", workView, map[string]any{"name": "Work done too", "type": "list", "sort_key": "a1",
			"project_id": w.work, "config": map[string]any{"filter": map[string]any{"showDone": true, "assignees": []string{"me"}}}})
		w.task(w.work, "Shipped", map[string]any{"due_date": "2026-10-27", "assign": "member", "completed_at": "2026-10-27T10:00:00Z"})
		_, wv := w.create("member", ScopeView, workView)
		if got := keys(w.events(wv)); !slices.Equal(got, []string{"Shipped", "Standup notes"}) {
			t.Errorf("Work done too = %v", got)
		}

		// Someone else's personal view cannot be subscribed to.
		personal := uuid.NewString()
		w.push("member", "saved_view", personal, map[string]any{"name": "Mine", "type": "list", "sort_key": "a2",
			"owner_id": w.users["member"], "config": map[string]any{"filter": map[string]any{}}})
		if _, _, err := w.svc.Create(w.ctx, w.users["owner"], w.ws, ScopeView, personal); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("someone else's view: %v", err)
		}
		for _, bad := range [][2]string{{ScopeMe, w.home}, {"everything", ""}, {ScopeProject, ""}, {ScopeProject, uuid.NewString()}} {
			if _, _, err := w.svc.Create(w.ctx, w.users["owner"], w.ws, bad[0], bad[1]); apperr.KindOf(err) == 0 {
				t.Errorf("%v: %v", bad, err)
			}
		}

		// A guest sees only the shared project's tasks, and cannot make a
		// feed of the other.
		// Assigned while they could reach it, then moved out of their sight.
		moved := w.task(w.home, "Guest in Work", map[string]any{"due_date": "2026-11-02", "assign": "guest"})
		w.push("owner", "task", moved, map[string]any{"project_id": w.work})
		w.task(w.home, "Guest at Home", map[string]any{"due_date": "2026-11-02", "assign": "guest"})
		_, gm := w.create("guest", ScopeMe, "")
		if got := keys(w.events(gm)); !slices.Equal(got, []string{"Guest at Home"}) {
			t.Errorf("guest's tasks = %v", got)
		}
		if _, _, err := w.svc.Create(w.ctx, w.users["guest"], w.ws, ScopeProject, w.work); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("guest, Work: %v", err)
		}
		_, gp := w.create("guest", ScopeProject, w.home)
		w.events(gp)
		// Unsharing the project ends the guest's project feed at once; the
		// tasks leave their own feed.
		q := "DELETE FROM project_access WHERE project_id = ? AND user_id = ?"
		if d.Dialect == db.Postgres {
			q = "DELETE FROM project_access WHERE project_id = $1 AND user_id = $2"
		}
		_, err = d.ExecContext(w.ctx, q, w.home, w.users["guest"])
		must(t, err)
		w.gone(gp)
		if got := keys(w.events(gm)); len(got) != 0 {
			t.Errorf("guest's tasks after unsharing = %v", got)
		}

		// Leaving the workspace ends every feed there.
		must(t, w.st.DeleteMembership(w.ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: w.ws, UserID: w.users["member"]}))
		w.gone(wv)

		// A deleted view or project ends its feed.
		w.push("owner", "saved_view", view, map[string]any{"deleted_at": "2026-10-29T10:00:00Z"})
		w.gone(vt)
		w.push("owner", "project", w.home, map[string]any{"deleted_at": "2026-10-29T10:00:00Z"})
		w.gone(project)
		if got := keys(w.events(me)); slices.Contains(got, "Groceries") || !slices.Contains(got, "Standup notes") {
			t.Errorf("my tasks after Home went to the trash = %v", got)
		}

		// Revoking ends a feed; only its owner can.
		meID := feeds[0].ID
		if err := w.svc.Revoke(w.ctx, w.users["guest"], w.ws, meID); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("someone else revoked it: %v", err)
		}
		w.events(me)
		must(t, w.svc.Revoke(w.ctx, w.users["owner"], w.ws, meID))
		w.gone(me)
		if err := w.svc.Revoke(w.ctx, w.users["owner"], w.ws, meID); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("revoked twice: %v", err)
		}
		feeds, err = w.svc.List(w.ctx, w.users["owner"], w.ws)
		must(t, err)
		if len(feeds) != 2 || slices.ContainsFunc(feeds, func(f Feed) bool { return f.ID == meID }) {
			t.Errorf("feeds after revoking = %+v", feeds)
		}

		// Tokens that were never made.
		for _, token := range []string{"", "short", strings.Repeat("A", 43), strings.Repeat("A", 42) + "!"} {
			w.gone(token)
		}
	})
}

// TestUTCOwner: an owner in UTC gets no VTIMEZONE; repeating tasks with a
// time stay in UTC.
func TestUTCOwner(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, err := accounts.UpdateSettings(w.ctx, w.st, w.users["owner"], func(s accounts.Settings) error {
			delete(s, "timezone")
			return nil
		})
		must(t, err)
		w.task(w.work, "Pills", map[string]any{"due_date": "2026-11-01", "due_time": "09:00", "assign": "owner",
			"recurrence": "FREQ=MONTHLY;BYDAY=-1FR"})
		_, me := w.create("owner", ScopeMe, "")
		body, err := w.svc.Calendar(w.ctx, me)
		must(t, err)
		if strings.Contains(string(body), "VTIMEZONE") {
			t.Errorf("body = %s", body)
		}
		if p := w.events(me)["Pills"]; p["DTSTART"] != "20261101T090000Z" || p["RRULE"] != "FREQ=MONTHLY;BYDAY=-1FR" {
			t.Errorf("Pills = %v", p)
		}
	})
}

// TestViewFeedFields: a view's feed honors its conditions on custom
// fields, with the owner's today for relative dates and the owner's
// visibility for fields, and ignores the view's display.
func TestViewFeedFields(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		stage, review, secret := uuid.NewString(), uuid.NewString(), uuid.NewString()
		w.push("owner", "custom_field", stage, map[string]any{"name": "Stage", "type": "select", "sort_key": "a0",
			"options": []map[string]string{{"id": "o-now", "name": "Now", "color": "red"}, {"id": "o-later", "name": "Later", "color": "blue"}}})
		w.push("owner", "custom_field", review, map[string]any{"name": "Review", "type": "date", "sort_key": "a1"})
		// A field on Work, which the guest cannot see.
		w.push("owner", "custom_field", secret, map[string]any{"name": "Secret", "type": "number", "sort_key": "a2", "project_id": w.work})
		set := func(task, field string, value any) {
			w.push("owner", "custom_field_value", pairID(task, field), map[string]any{"task_id": task, "field_id": field, "value": value})
		}
		apples := w.task(w.home, "Pick apples", map[string]any{"due_date": "2026-11-02"})
		set(apples, stage, "o-now")
		set(apples, review, "2026-11-01")
		rake := w.task(w.home, "Rake leaves", map[string]any{"due_date": "2026-11-03"})
		set(rake, stage, "o-later")
		set(rake, review, "2026-11-01")
		prune := w.task(w.home, "Prune", map[string]any{"due_date": "2026-11-04"})
		set(prune, stage, "o-now")
		set(prune, review, "2026-12-15")
		audit := w.task(w.work, "Audit", map[string]any{"due_date": "2026-11-02"})
		set(audit, secret, 5)
		w.task(w.home, "Undated", map[string]any{})

		view := func(name string, fields ...map[string]any) string {
			id := uuid.NewString()
			w.push("owner", "saved_view", id, map[string]any{"name": name, "type": "calendar", "sort_key": "a0",
				"config": map[string]any{"filter": map[string]any{"fields": fields}}})
			return id
		}
		// Today is the 29th where the owner is, so a week on is November 5.
		soon := view("Now, reviewed soon",
			map[string]any{"field": stage, "op": "any", "value": []string{"o-now"}},
			map[string]any{"field": review, "op": "before", "value": 7})
		_, st := w.create("owner", ScopeView, soon)
		if got := keys(w.events(st)); !slices.Equal(got, []string{"Pick apples"}) {
			t.Errorf("Now, reviewed soon = %v", got)
		}
		before, err := w.svc.Calendar(w.ctx, st)
		must(t, err)
		// Display is not the feed's business: the body stays the same.
		w.push("owner", "saved_view", soon, map[string]any{"display": map[string]any{
			"sort": []map[string]any{{"by": "field", "field": review, "dir": "desc"}}, "group": map[string]any{"by": "priority"}}})
		after, err := w.svc.Calendar(w.ctx, st)
		must(t, err)
		if string(after) != string(before) {
			t.Errorf("changing the display changed the feed:\n%s\n%s", before, after)
		}

		// A field the guest cannot see reads as no task having a value.
		secretSet := view("Secret set", map[string]any{"field": secret, "op": "not_empty"})
		secretUnset := view("Secret unset", map[string]any{"field": secret, "op": "empty"})
		_, owner := w.create("owner", ScopeView, secretSet)
		if got := keys(w.events(owner)); !slices.Equal(got, []string{"Audit"}) {
			t.Errorf("the owner's Secret set = %v", got)
		}
		_, gs := w.create("guest", ScopeView, secretSet)
		if got := keys(w.events(gs)); len(got) != 0 {
			t.Errorf("the guest's Secret set = %v", got)
		}
		_, gu := w.create("guest", ScopeView, secretUnset)
		if got := keys(w.events(gu)); !slices.Equal(got, []string{"Pick apples", "Prune", "Rake leaves"}) {
			t.Errorf("the guest's Secret unset = %v", got)
		}
	})
}

// readsOf counts the custom field rows a feed asks sync for.
type readsOf struct {
	sync   Rows
	fields int
}

func (r *readsOf) Rows(ctx context.Context, userID, workspaceID string, names ...string) ([]syncer.Row, error) {
	if slices.Contains(names, "custom_field_value") {
		r.fields++
	}
	return r.sync.Rows(ctx, userID, workspaceID, names...)
}

func TestViewFeedArchivedAndFieldReads(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		reads := &readsOf{sync: w.sync}
		w.svc.rows = reads
		w.task(w.home, "Sweep", map[string]any{"due_date": "2026-11-02"})
		w.task(w.work, "File report", map[string]any{"due_date": "2026-11-03"})
		all, work := uuid.NewString(), uuid.NewString()
		w.push("owner", "saved_view", all, map[string]any{"name": "Everything", "type": "list", "sort_key": "a0",
			"config": map[string]any{"filter": map[string]any{}}})
		w.push("owner", "saved_view", work, map[string]any{"name": "Work", "type": "list", "sort_key": "a1",
			"project_id": w.work, "config": map[string]any{"filter": map[string]any{}}})
		_, at := w.create("owner", ScopeView, all)
		_, wt := w.create("owner", ScopeView, work)
		if got := keys(w.events(at)); !slices.Equal(got, []string{"File report", "Sweep"}) {
			t.Errorf("Everything = %v", got)
		}
		// Without field conditions, no field values are read.
		if reads.fields != 0 {
			t.Errorf("field values read %d times for a view without conditions", reads.fields)
		}
		// An archived project's tasks leave views across projects, as on the
		// view page, but a view of that project still shows them.
		w.push("owner", "project", w.work, map[string]any{"archived_at": "2026-10-29T10:00:00Z"})
		if got := keys(w.events(at)); !slices.Equal(got, []string{"Sweep"}) {
			t.Errorf("Everything with Work archived = %v", got)
		}
		if got := keys(w.events(wt)); !slices.Equal(got, []string{"File report"}) {
			t.Errorf("Work, archived = %v", got)
		}
		// A condition on a field makes the feed read them.
		w.push("owner", "saved_view", all, map[string]any{"config": map[string]any{"filter": map[string]any{
			"fields": []map[string]any{{"field": uuid.NewString(), "op": "empty"}}}}})
		if got := keys(w.events(at)); !slices.Equal(got, []string{"Sweep"}) {
			t.Errorf("Everything with a condition = %v", got)
		}
		if reads.fields != 1 {
			t.Errorf("field values read %d times, want 1", reads.fields)
		}
	})
}
