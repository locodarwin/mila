package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
	// down makes every send fail, like an unreachable mail server.
	down bool
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.down {
		return errors.New("mail server unreachable")
	}
	o.sent = append(o.sent, m)
	return nil
}

func (o *outbox) take() []mail.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.sent
	o.sent = nil
	return out
}

type poker struct {
	mu     sync.Mutex
	poked  []string
	called int
}

func (p *poker) Notify(ids ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.poked = append(p.poked, ids...)
	p.called++
}

// world is a workspace with an owner, a member and a guest, a project
// shared with the guest, and notifications wired to sync.
type world struct {
	t       *testing.T
	ctx     context.Context
	st      *store.Store
	sync    *syncer.Service
	runner  *jobs.Runner
	svc     *notify.Service
	mail    *outbox
	poker   *poker
	clock   time.Time
	ws      string
	project string
	users   map[string]string
	names   map[string]string
}

const base = "https://mila.example.com"

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &world{t: t, ctx: ctx, st: st, mail: &outbox{}, poker: &poker{}, ws: uuid.NewString(),
		users: map[string]string{}, names: map[string]string{}, clock: time.Now().UTC()}
	w.runner = jobs.New(st, log, func() time.Time { return w.clock })
	w.sync = syncer.New(st, log, nil)
	w.svc = notify.New(st, log, w.runner, w.mail, w.sync, w.poker, notify.Config{BaseURL: base, Email: true})
	w.sync.SetWatcher(w.svc)

	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member", "guest"} {
		id := uuid.NewString()
		name := map[string]string{"owner": "Shawn", "member": "Dana", "guest": "Lee"}[role]
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "-" + id[:8] + "@example.com", Name: name, Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: id, Role: role, Now: now}))
		w.users[role], w.names[role] = id, name
	}
	w.project = uuid.NewString()
	w.push("owner", "project", w.project, map[string]any{"name": "Home", "sort_key": "a0"})
	w.share(w.project, "guest")
	return w
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var seq int

func (w *world) push(role, entity, id string, fields map[string]any) {
	w.t.Helper()
	w.pushAt(role, time.Now(), entity, id, fields)
}

// pushAt pushes an edit made at a given time, by its clock.
func (w *world) pushAt(role string, at time.Time, entity, id string, fields map[string]any) {
	w.t.Helper()
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	seq++
	hlc := fmt.Sprintf("%013d-%04x-%s", at.UnixMilli(), seq, role)
	res, _, err := w.sync.Push(w.ctx, w.users[role], w.ws, []syncer.Op{{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw}})
	must(w.t, err)
	if res[0].Status != "applied" {
		w.t.Fatalf("%s %s: %s %s", role, entity, res[0].Status, res[0].Message)
	}
}

func (w *world) share(project, role string) {
	d := w.st.DB()
	q := "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"
	if d.Dialect == db.Postgres {
		q = "INSERT INTO project_access (project_id, user_id, created_at) VALUES ($1, $2, $3)"
	}
	_, err := d.ExecContext(w.ctx, q, project, w.users[role], store.Now())
	must(w.t, err)
}

func (w *world) task(role, project, title string, extra map[string]any) string {
	id := uuid.NewString()
	fields := map[string]any{"title": title, "sort_key": "a0", "created_by": w.users[role]}
	if project != "" {
		fields["project_id"] = project
	}
	for k, v := range extra {
		fields[k] = v
	}
	w.push(role, "task", id, fields)
	return id
}

func pairID(a, b string) string { return ids.Pair(a, b) }

func (w *world) assign(actor, task, who string, deleted bool) {
	var del any
	if deleted {
		del = time.Now().UTC().Format(time.RFC3339Nano)
	}
	w.push(actor, "task_assignee", pairID(task, w.users[who]), map[string]any{"task_id": task, "user_id": w.users[who], "deleted_at": del})
}

func (w *world) comment(role, task, body string) string {
	id := uuid.NewString()
	w.push(role, "comment", id, map[string]any{"task_id": task, "author_id": w.users[role], "body": body})
	return id
}

func (w *world) mention(role string) string {
	return "@[" + w.names[role] + "](user:" + w.users[role] + ")"
}

// run delivers everything queued.
func (w *world) run() {
	w.t.Helper()
	if _, err := w.runner.RunDue(w.ctx); err != nil {
		w.t.Fatal(err)
	}
}

// inbox is what a user's in-app list says, newest first, as "kind: title".
func (w *world) inbox(role string) []string {
	w.t.Helper()
	page, err := w.svc.List(w.ctx, w.users[role], "", 100)
	must(w.t, err)
	out := []string{}
	for _, n := range page.Items {
		out = append(out, n.Kind+": "+n.Title)
	}
	return out
}

func want(t *testing.T, got []string, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestAssigned(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("owner", w.project, "Repaint the porch", nil)
		w.assign("owner", task, "owner", false) // yourself: nothing
		w.assign("owner", task, "member", false)
		w.run()
		want(t, w.inbox("member"), "assigned: Shawn assigned you to Repaint the porch")
		want(t, w.inbox("owner"))
		if !slices.Contains(w.poker.poked, w.users["member"]) {
			t.Fatal("the member's open apps were not told")
		}
		// Assigned is in the app and by push by default, not email.
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("emailed %v", sent)
		}

		// Unassigning says nothing; assigning again does.
		w.assign("owner", task, "member", true)
		w.run()
		w.assign("owner", task, "member", false)
		w.run()
		if got := w.inbox("member"); len(got) != 2 {
			t.Fatalf("after reassigning: %q", got)
		}

		// A guest hears of a task in their shared project.
		w.assign("member", task, "guest", false)
		w.run()
		want(t, w.inbox("guest"), "assigned: Dana assigned you to Repaint the porch")

		// The next task of a series arrives with its assignees copied, which
		// is not news to them.
		next := w.task("owner", w.project, "Water the plants", map[string]any{"series_id": task, "recurrence": "FREQ=DAILY", "due_date": "2026-10-29"})
		w.assign("owner", next, "member", false)
		w.run()
		if got := w.inbox("member"); len(got) != 2 {
			t.Fatalf("a repeat's copy notified its assignee: %q", got)
		}

		// An undo takes the copy and its assignees away; completing the task
		// again an hour later brings them back, which is not news either.
		w.push("owner", "task", next, map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.assign("owner", next, "member", true)
		w.backdate(next, time.Hour)
		w.push("owner", "task", next, map[string]any{"deleted_at": nil})
		w.assign("owner", next, "member", false)
		w.run()
		if got := w.inbox("member"); len(got) != 2 {
			t.Fatalf("a repeat's copy brought back notified its assignee: %q", got)
		}
		// Someone assigned to a copy well after it came back does hear.
		ferns := uuid.NewString()
		w.pushAt("owner", time.Now().Add(-5*time.Minute), "task", ferns, map[string]any{"title": "Water the ferns", "sort_key": "a0",
			"project_id": w.project, "created_by": w.users["owner"], "series_id": task, "deleted_at": nil})
		w.backdate(ferns, time.Hour)
		w.assign("member", ferns, "guest", false)
		w.run()
		want(t, w.inbox("guest"), "assigned: Dana assigned you to Water the ferns", "assigned: Dana assigned you to Repaint the porch")
	})
}

// stored counts the notifications kept for someone, whether or not the
// app lists them.
func (w *world) stored(role string) int {
	w.t.Helper()
	d := w.st.DB()
	q := "SELECT COUNT(*) FROM notifications WHERE user_id = ?"
	if d.Dialect == db.Postgres {
		q = "SELECT COUNT(*) FROM notifications WHERE user_id = $1"
	}
	var n int
	must(w.t, d.QueryRowContext(w.ctx, q, w.users[role]).Scan(&n))
	return n
}

// email is someone's address.
func (w *world) email(role string) string {
	w.t.Helper()
	u, err := w.st.GetUser(w.ctx, w.users[role])
	must(w.t, err)
	return u.Email
}

// backdate moves when a task was first made into the past.
func (w *world) backdate(task string, by time.Duration) {
	w.t.Helper()
	d := w.st.DB()
	q := "UPDATE tasks SET created_at = ? WHERE id = ?"
	if d.Dialect == db.Postgres {
		q = "UPDATE tasks SET created_at = $1 WHERE id = $2"
	}
	_, err := d.ExecContext(w.ctx, q, time.Now().UTC().Add(-by).Truncate(time.Microsecond), task)
	must(w.t, err)
}

func TestCommentsAndMentions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("member", w.project, "Repaint the porch", nil)
		w.assign("member", task, "guest", false)
		w.run()
		w.poker.poked = nil

		// The owner comments: the creator and the assignee hear of it.
		w.comment("owner", task, "Which color?")
		w.run()
		want(t, w.inbox("member"), "comment: Shawn commented on Repaint the porch")
		want(t, w.inbox("guest"), "comment: Shawn commented on Repaint the porch", "assigned: Dana assigned you to Repaint the porch")
		want(t, w.inbox("owner"))
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("comments are in the app only by default, but emailed %v", sent)
		}

		// A mention replaces the comment notice for the person mentioned, and
		// reaches people otherwise not involved. Strangers' IDs count for
		// nothing, and nobody hears about their own comment.
		stranger := "@[Eve](user:" + uuid.NewString() + ")"
		c := w.comment("guest", task, "Ask "+w.mention("owner")+" and "+w.mention("member")+", not "+stranger+". "+w.mention("guest"))
		w.run()
		want(t, w.inbox("owner"), "mention: Lee mentioned you in Repaint the porch")
		want(t, w.inbox("member"), "mention: Lee mentioned you in Repaint the porch", "comment: Shawn commented on Repaint the porch")
		if got := w.inbox("guest"); len(got) != 2 {
			t.Fatalf("the guest heard about their own comment: %q", got)
		}
		page, _ := w.svc.List(w.ctx, w.users["owner"], "", 1)
		if body := page.Items[0].Body; body != "Ask @Shawn and @Dana, not @Eve. @Lee" {
			t.Fatalf("excerpt = %q", body)
		}
		if page.Items[0].URL != "/?task="+task+"&ws="+w.ws {
			t.Fatalf("url = %q", page.Items[0].URL)
		}

		// Mentions are emailed by default.
		sent := w.mail.take()
		if len(sent) != 2 {
			t.Fatalf("emailed %d messages, want 2", len(sent))
		}
		m := sent[0]
		if m.Subject != "Lee mentioned you in Repaint the porch" ||
			!strings.Contains(m.Text, "> Ask @Shawn and @Dana") ||
			!strings.Contains(m.Text, "Project: Home (Team)") ||
			!strings.Contains(m.Text, base+"/?task="+task+"&ws="+w.ws+"&n=") ||
			!strings.Contains(m.Text, base+"/settings/notifications") {
			t.Fatalf("email:\n%s\n%s", m.Subject, m.Text)
		}
		// Each is emailed once.
		w.clock = w.clock.Add(time.Hour)
		w.run()
		if again := w.mail.take(); len(again) != 0 {
			t.Fatalf("emailed again: %v", again)
		}

		// Editing a comment mentions only who is new in it.
		w.push("guest", "comment", c, map[string]any{"body": "Ask " + w.mention("owner") + " and " + w.mention("member") + ", fixed"})
		w.run()
		if got := w.inbox("owner"); len(got) != 1 {
			t.Fatalf("an edit re-mentioned: %q", got)
		}
		w.push("guest", "comment", c, map[string]any{"body": w.mention("owner") + " only"})
		w.push("guest", "comment", c, map[string]any{"body": w.mention("owner") + " " + w.mention("member")})
		w.run()
		if got := w.inbox("member"); len(got) != 3 || !strings.HasPrefix(got[0], "mention:") {
			t.Fatalf("a mention added back by an edit: %q", got)
		}

		// Someone who cannot see the task is never told, mentioned or not.
		inbox := w.task("owner", "", "Owner only", nil)
		w.comment("member", inbox, "Hi "+w.mention("guest")+" and "+w.mention("owner"))
		w.run()
		if got := w.inbox("guest"); len(got) != 2 {
			t.Fatalf("the guest heard about a task they cannot see: %q", got)
		}
		if got := w.inbox("owner"); len(got) != 2 {
			t.Fatalf("owner = %q", got)
		}
	})
}

func TestImportedCommentsTellNobody(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("member", w.project, "Repaint the porch", nil)
		w.assign("member", task, "guest", false)
		w.run()
		before := len(w.inbox("guest"))

		// Brought in with many others, as an import writes them.
		raw := map[string]json.RawMessage{}
		for k, v := range map[string]any{"task_id": task, "author_id": w.users["owner"], "body": "Originally by dana: ask " + w.mention("guest")} {
			b, _ := json.Marshal(v)
			raw[k] = b
		}
		seq++
		op := syncer.Op{OpID: uuid.NewString(), Entity: "comment", ID: uuid.NewString(), HLC: fmt.Sprintf("%013d-%04x-owner", time.Now().UnixMilli(), seq), Fields: raw, Bulk: true}
		res, _, err := w.sync.Push(w.ctx, w.users["owner"], w.ws, []syncer.Op{op})
		must(t, err)
		if res[0].Status != "applied" {
			t.Fatalf("push: %s %s", res[0].Status, res[0].Message)
		}
		w.run()
		if got := w.inbox("guest"); len(got) != before {
			t.Fatalf("an imported comment told the assignee: %q", got)
		}
		if got := w.inbox("member"); len(got) != 0 {
			t.Fatalf("an imported comment told the task's maker: %q", got)
		}

		// One typed afterward still does.
		w.comment("owner", task, "Which color?")
		w.run()
		want(t, w.inbox("member"), "comment: Shawn commented on Repaint the porch")
	})
}

func TestPreferences(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		p, _, err := w.svc.Prefs(w.ctx, w.users["member"])
		must(t, err)
		if p.Kinds[notify.Mention] != (notify.Channels{App: true, Push: true, Email: true}) || p.Digest.Hour != 7 || p.Digest.Enabled {
			t.Fatalf("defaults = %+v", p)
		}
		// Mentions cannot leave the app; comments can, and email instead.
		p, _, err = w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{
			Kinds: map[notify.Kind]notify.Channels{
				notify.Mention: {App: false, Email: false},
				notify.Comment: {App: false, Email: true},
			},
			Digest: notify.Digest{Enabled: true, Hour: 18},
		}, nil)
		must(t, err)
		if p.Kinds[notify.Mention] != (notify.Channels{App: true}) || p.Kinds[notify.Comment] != (notify.Channels{Email: true}) ||
			p.Kinds[notify.Assigned] != (notify.Channels{App: true, Push: true}) || p.Digest != (notify.Digest{Enabled: true, Hour: 18}) {
			t.Fatalf("saved = %+v", p)
		}
		if _, _, err := w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Digest: notify.Digest{Hour: 24}}, nil); err == nil {
			t.Fatal("hour 24 was accepted")
		}
		if _, _, err := w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{"weather": {}}}, nil); err == nil {
			t.Fatal("an unknown kind was accepted")
		}
		// Other settings survive.
		u, err := accounts.UpdateSettings(w.ctx, w.st, w.users["member"], func(s accounts.Settings) error {
			s["timezone"] = json.RawMessage(`"Europe/Berlin"`)
			return nil
		})
		must(t, err)
		if notify.PrefsOf(u).Digest.Hour != 18 || accounts.Timezone(u) != "Europe/Berlin" {
			t.Fatalf("settings = %s", u.Settings)
		}

		task := w.task("member", w.project, "Fix the gate", nil)
		w.comment("owner", task, "Done?")
		w.run()
		want(t, w.inbox("member"))
		sent := w.mail.take()
		if len(sent) != 1 || sent[0].Subject != "Shawn commented on Fix the gate" {
			t.Fatalf("emailed %v", sent)
		}
		// Off everywhere means no row at all.
		_, _, err = w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{notify.Comment: {}}}, nil)
		must(t, err)
		rows := w.stored("member")
		w.comment("owner", task, "Hello?")
		w.run()
		if w.stored("member") != rows || len(w.mail.take()) != 0 {
			t.Fatal("a notification turned off everywhere was delivered")
		}
	})
}

func TestListAndRead(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("member", w.project, "Plan the trip", nil)
		for i := range 5 {
			w.comment("owner", task, fmt.Sprintf("Note %d", i))
			w.run()
			w.clock = w.clock.Add(time.Second)
		}
		member := w.users["member"]
		var bodies []string
		cursor := ""
		for {
			page, err := w.svc.List(w.ctx, member, cursor, 2)
			must(t, err)
			if page.Unread != 5 {
				t.Fatalf("unread = %d", page.Unread)
			}
			for _, n := range page.Items {
				bodies = append(bodies, n.Body)
			}
			if page.Next == "" {
				break
			}
			cursor = page.Next
		}
		want(t, bodies, "Note 4", "Note 3", "Note 2", "Note 1", "Note 0")

		page, _ := w.svc.List(w.ctx, member, "", 10)
		if err := w.svc.MarkRead(w.ctx, w.users["owner"], page.Items[0].ID); err == nil {
			t.Fatal("someone else marked a notification read")
		}
		must(t, w.svc.MarkRead(w.ctx, member, page.Items[0].ID))
		page, _ = w.svc.List(w.ctx, member, "", 10)
		if page.Unread != 4 || !page.Items[0].ReadAt.Valid || page.Items[1].ReadAt.Valid {
			t.Fatalf("after one read: unread %d", page.Unread)
		}
		// "Mark all read" covers what the user has seen, not one that
		// arrived since.
		seen := page.Items[0].ID
		w.comment("owner", task, "Note 5")
		w.run()
		must(t, w.svc.MarkAllRead(w.ctx, member, seen))
		if page, _ = w.svc.List(w.ctx, member, "", 10); page.Unread != 1 || page.Items[0].Body != "Note 5" || page.Items[0].ReadAt.Valid {
			t.Fatalf("after all read: unread %d, newest %q", page.Unread, page.Items[0].Body)
		}
		if err := w.svc.MarkAllRead(w.ctx, w.users["owner"], page.Items[0].ID); err == nil {
			t.Fatal("someone else's cursor marked notifications read")
		}
		must(t, w.svc.MarkAllRead(w.ctx, member, page.Items[0].ID))
		if page, _ = w.svc.List(w.ctx, member, "", 10); page.Unread != 0 {
			t.Fatalf("after all read: unread %d", page.Unread)
		}
		if _, err := w.svc.List(w.ctx, w.users["owner"], page.Items[0].ID, 10); err == nil {
			t.Fatal("someone else's cursor was accepted")
		}
	})
}

func TestClear(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("member", w.project, "Plan the trip", nil)
		for i := range 4 {
			w.comment("owner", task, fmt.Sprintf("Note %d", i))
			w.run()
			w.clock = w.clock.Add(time.Second)
		}
		member, owner := w.users["member"], w.users["owner"]
		page, _ := w.svc.List(w.ctx, member, "", 10)
		want(t, []string{page.Items[0].Body, page.Items[3].Body}, "Note 3", "Note 0")

		// Someone else's stays, asked for directly or through their cursor.
		must(t, w.svc.Clear(w.ctx, owner, page.Items[0].ID))
		if err := w.svc.ClearAll(w.ctx, owner, page.Items[0].ID); err == nil {
			t.Fatal("someone else's cursor cleared notifications")
		}
		if page, _ = w.svc.List(w.ctx, member, "", 10); len(page.Items) != 4 {
			t.Fatalf("after someone else tried: %d", len(page.Items))
		}

		// One goes, and takes its place in the count; asking again does nothing.
		must(t, w.svc.Clear(w.ctx, member, page.Items[1].ID))
		must(t, w.svc.Clear(w.ctx, member, page.Items[1].ID))
		if page, _ = w.svc.List(w.ctx, member, "", 10); len(page.Items) != 3 || page.Unread != 3 {
			t.Fatalf("after one: %d, unread %d", len(page.Items), page.Unread)
		}

		// Clear all covers what the user has seen, not one that arrived since.
		seen := page.Items[0].ID
		w.comment("owner", task, "Note 4")
		w.run()
		must(t, w.svc.ClearAll(w.ctx, member, seen))
		page, _ = w.svc.List(w.ctx, member, "", 10)
		if len(page.Items) != 1 || page.Items[0].Body != "Note 4" || page.Unread != 1 {
			t.Fatalf("after all: %d, unread %d", len(page.Items), page.Unread)
		}
		if err := w.svc.ClearAll(w.ctx, member, seen); err == nil {
			t.Fatal("a cursor already gone was accepted")
		}
	})
}

// Access lost after a notification was made hides it and stops its
// delivery: nobody hears about what they can no longer see.
func TestLostAccess(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		task := w.task("owner", w.project, "Repaint the porch", nil)
		w.mail.down = true
		w.comment("owner", task, "Ready, "+w.mention("guest")+" and "+w.mention("member")+"?")
		w.run()
		want(t, w.inbox("guest"), "mention: Shawn mentioned you in Repaint the porch")
		want(t, w.inbox("member"), "mention: Shawn mentioned you in Repaint the porch")

		// The guest loses the project and Dana leaves the workspace before
		// the mail server comes back.
		q := "DELETE FROM project_access WHERE project_id = ? AND user_id = ?"
		if d.Dialect == db.Postgres {
			q = "DELETE FROM project_access WHERE project_id = $1 AND user_id = $2"
		}
		_, err := d.ExecContext(w.ctx, q, w.project, w.users["guest"])
		must(t, err)
		must(t, w.st.DeleteMembership(w.ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: w.ws, UserID: w.users["member"]}))
		w.mail.down = false
		w.clock = w.clock.Add(time.Hour)
		w.run()
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("emailed after access was lost: %v", sent)
		}
		for _, role := range []string{"guest", "member"} {
			want(t, w.inbox(role))
			if n, _ := w.st.CountUnreadNotifications(w.ctx, w.users[role]); n != 0 {
				t.Fatalf("%s still counts %d unread", role, n)
			}
			unread, err := w.st.ListUnreadNotificationsSince(w.ctx, sqlitedb.ListUnreadNotificationsSinceParams{UserID: w.users[role], Since: time.Time{}})
			must(t, err)
			if len(unread) != 0 {
				t.Fatalf("%s's digest would list %d", role, len(unread))
			}
		}

		// Shared again, the guest sees it again.
		w.share(w.project, "guest")
		want(t, w.inbox("guest"), "mention: Shawn mentioned you in Repaint the porch")
	})
}

func TestRemovedThings(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		mention := "mention: Shawn mentioned you in Repaint the porch"
		task := w.task("owner", w.project, "Repaint the porch", nil)
		w.mail.down = true
		w.comment("owner", task, "Ready, "+w.mention("member")+"?")
		w.run()
		want(t, w.inbox("member"), mention)

		hidden := func(gone bool) {
			t.Helper()
			n, err := w.st.CountUnreadNotifications(w.ctx, w.users["member"])
			must(t, err)
			unread, err := w.st.ListUnreadNotificationsSince(w.ctx, sqlitedb.ListUnreadNotificationsSinceParams{
				UserID: w.users["member"], Since: time.Time{}})
			must(t, err)
			if gone {
				want(t, w.inbox("member"))
				if n != 0 || len(unread) != 0 {
					t.Fatalf("still counts %d unread, and the digest lists %d", n, len(unread))
				}
				return
			}
			want(t, w.inbox("member"), mention)
			if n != 1 || len(unread) != 1 {
				t.Fatalf("counts %d unread, and the digest lists %d", n, len(unread))
			}
		}
		// Someone holding one hears of it, so their count changes at once.
		poked := func() {
			t.Helper()
			w.poker.mu.Lock()
			w.poker.poked = nil
			w.poker.mu.Unlock()
			w.run()
			w.poker.mu.Lock()
			defer w.poker.mu.Unlock()
			if !slices.Contains(w.poker.poked, w.users["member"]) {
				t.Fatalf("not told: %v", w.poker.poked)
			}
		}
		hidden(false)

		// Each thing it depends on, removed and then restored.
		for _, gone := range []struct{ entity, id string }{{"project", w.project}, {"task", task}} {
			w.push("owner", gone.entity, gone.id, map[string]any{"deleted_at": store.Now().Format(time.RFC3339Nano)})
			poked()
			hidden(true)
			w.push("owner", gone.entity, gone.id, map[string]any{"deleted_at": nil})
			poked()
			hidden(false)
		}

		must(t, w.st.DeleteWorkspace(w.ctx, sqlitedb.DeleteWorkspaceParams{ID: w.ws, Now: store.NullNow()}))
		hidden(true)
		must(t, w.st.ShowWorkspace(w.ctx, sqlitedb.ShowWorkspaceParams{ID: w.ws, Now: store.Now()}))
		hidden(false)
	})
}

func TestInviteAccepted(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.svc.InviteAccepted(w.ctx, w.ws, w.users["owner"], w.users["member"])
		w.run()
		want(t, w.inbox("owner"), "invite_accepted: Dana joined Team")
		sent := w.mail.take()
		if len(sent) != 1 || sent[0].Subject != "Dana joined Team" || !strings.Contains(sent[0].Text, "Open Team: "+base+"/?ws="+w.ws) {
			t.Fatalf("emailed %v", sent)
		}
		// An inviter who has left hears nothing.
		must(t, w.st.DeleteMembership(w.ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: w.ws, UserID: w.users["guest"]}))
		w.svc.InviteAccepted(w.ctx, w.ws, w.users["guest"], w.users["member"])
		w.run()
		if w.stored("guest") != 0 {
			t.Fatal("a former member was notified")
		}
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("a former member was emailed %q", sent[0].Subject)
		}
	})
}

func TestDigest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		member := w.users["member"]
		_, err := accounts.UpdateSettings(w.ctx, w.st, member, func(s accounts.Settings) error {
			s["timezone"] = json.RawMessage(`"America/Chicago"`)
			return nil
		})
		must(t, err)
		_, _, err = w.svc.SavePrefs(w.ctx, member, notify.Prefs{Digest: notify.Digest{Enabled: true, Hour: 7}}, nil)
		must(t, err)

		// Chicago is five hours behind UTC until November 1.
		w.clock = time.Date(2026, 10, 28, 11, 0, 0, 0, time.UTC)
		w.task("member", w.project, "Pay rent", map[string]any{"due_date": "2026-10-26"})
		w.task("member", "", "Call Dana", map[string]any{"due_date": "2026-10-28", "due_time": "15:00"})
		w.task("member", w.project, "Later", map[string]any{"due_date": "2026-10-29"})
		w.task("member", w.project, "Done", map[string]any{"due_date": "2026-10-27", "completed_at": time.Now().UTC().Format(time.RFC3339)})
		w.task("owner", w.project, "Not theirs", map[string]any{"due_date": "2026-10-27"})
		assigned := w.task("owner", w.project, "Theirs by assignment", map[string]any{"due_date": "2026-10-27"})
		w.assign("owner", assigned, "member", false)
		taken := w.task("member", w.project, "Made by them, assigned away", map[string]any{"due_date": "2026-10-27"})
		w.assign("member", taken, "owner", false)

		w.clock = time.Date(2026, 10, 28, 11, 59, 0, 0, time.UTC)
		w.run()
		w.mail.take()
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("a digest before 7:00 local: %v", sent)
		}
		w.clock = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		sent := w.mail.take()
		if len(sent) != 1 {
			t.Fatalf("sent %d digests", len(sent))
		}
		m := sent[0]
		if m.Subject != "Your Mila digest: 2 overdue, 1 due today, 1 unread" {
			t.Fatalf("subject = %q", m.Subject)
		}
		for _, line := range []string{
			"Here is your day in Mila, Wednesday, October 28.",
			"Overdue\n\n- Pay rent, due Mon, Oct 26 (Home)\n",
			"- Theirs by assignment, due Tue, Oct 27 (Home)\n",
			"Due today\n\n- Call Dana at 15:00 (Inbox)\n",
			"Unread notifications\n\n- Shawn assigned you to Theirs by assignment\n",
		} {
			if !strings.Contains(m.Text, line) {
				t.Fatalf("digest lacks %q:\n%s", line, m.Text)
			}
		}
		for _, absent := range []string{"Later", "Done", "Not theirs", "assigned away"} {
			if strings.Contains(m.Text, absent) {
				t.Fatalf("digest has %q:\n%s", absent, m.Text)
			}
		}
		// Once a day.
		w.clock = w.clock.Add(time.Hour)
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatal("a second digest the same day")
		}
		// Nothing to say means no email.
		page, err := w.svc.List(w.ctx, member, "", 1)
		must(t, err)
		must(t, w.svc.MarkAllRead(w.ctx, member, page.Items[0].ID))
		for _, task := range listTasks(t, w) {
			w.push("member", "task", task, map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339)})
		}
		w.clock = time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC)
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatalf("an empty digest: %v", sent)
		}
		// Too long after the hour (the server was down) waits for tomorrow.
		w.clock = time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC)
		w.task("member", "", "Overdue again", map[string]any{"due_date": "2026-10-20"})
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		if sent := w.mail.take(); len(sent) != 0 {
			t.Fatal("a digest at 10:00 for a 7:00 digest")
		}
	})
}

func listTasks(t *testing.T, w *world) []string {
	tasks, err := w.sync.DueTasks(w.ctx, w.users["member"], "2026-12-31")
	must(t, err)
	var ids []string
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestTestEmail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		must(t, w.svc.SendTestEmail(w.ctx, w.users["member"]))
		sent := w.mail.take()
		if len(sent) != 1 || !strings.HasPrefix(sent[0].To, "member-") || sent[0].Subject != "Mila test email" {
			t.Fatalf("sent %v", sent)
		}
	})
}

func TestExcerptAndMentions(t *testing.T) {
	if got := notify.Mentions("@[A](user:1) @[B](user:2) @[A again](user:1) @[x](user:) [y](user:3)"); !slices.Equal(got, []string{"1", "2"}) {
		t.Fatalf("Mentions = %v", got)
	}
	if got := notify.Excerpt("one\n\n  two   three", 100); got != "one two three" {
		t.Fatalf("Excerpt = %q", got)
	}
	if got := notify.Excerpt("See [Paint the shed](/?task=abc) and @[Ana](user:1)", 100); got != "See Paint the shed and @Ana" {
		t.Fatalf("Excerpt with a reference = %q", got)
	}
	if got := notify.Excerpt("Read [this](https://example.com) now", 100); got != "Read [this](https://example.com) now" {
		t.Fatalf("Excerpt left an outside link as typed = %q", got)
	}
	if got := notify.Excerpt("abcdefghij", 5); got != "abcd…" {
		t.Fatalf("Excerpt cut = %q", got)
	}
}

func TestNewDeviceSignIn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		u, err := w.st.GetUser(w.ctx, w.users["member"])
		must(t, err)
		// Even with every notification turned off, security mail goes out.
		_, err = accounts.UpdateSettings(w.ctx, w.st, u.ID, func(s accounts.Settings) error {
			s["notifications"] = json.RawMessage(`{"kinds":{"mention":{"app":false,"email":false}}}`)
			return nil
		})
		must(t, err)
		w.svc.NewDeviceSignIn(w.ctx, u, accounts.SecurityEvent{
			Kind: accounts.EventSignIn, NewDevice: true, Ip: "198.51.100.7", CreatedAt: store.Now(),
			UserAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0",
		})
		w.run()
		sent := w.mail.take()
		if len(sent) != 1 || sent[0].To != u.Email || sent[0].Subject != "Mila: new sign-in on Firefox on Linux" ||
			!strings.Contains(sent[0].Text, "198.51.100.7") || !strings.Contains(sent[0].Text, base+"/settings/security") {
			t.Fatalf("emailed %+v", sent)
		}
		if w.stored("member") != 0 {
			t.Fatal("a new device made an in-app notification")
		}
	})
}

func TestDeviceName(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Safari/537.36":               "Chrome on Windows",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/604.1": "Safari on iPhone",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Safari/537.36 Edg/141.0":                 "Edge on Windows",
		"curl/8.0": "an unknown device",
	} {
		if got := notify.DeviceName(ua); got != want {
			t.Errorf("DeviceName(%q) = %q, want %q", ua, got, want)
		}
	}
}

func (w *world) waitFor(role, task, blocker string) {
	w.push(role, "task_dependency", pairID(task, blocker), map[string]any{"task_id": task, "blocker_id": blocker})
}

func (w *world) finish(role, task string) {
	w.push(role, "task", task, map[string]any{"completed_at": time.Now().UTC().Format(time.RFC3339Nano)})
}

func TestUnblocked(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		elec := w.task("owner", w.project, "Electrical", nil)
		plumb := w.task("owner", w.project, "Plumbing", nil)
		insp := w.task("owner", w.project, "Inspection", nil)
		w.assign("owner", insp, "member", false)
		w.waitFor("owner", insp, elec)
		w.waitFor("owner", insp, plumb)
		w.run()
		w.inbox("member") // the assignment
		before := len(w.inbox("member"))

		w.finish("owner", elec)
		w.run()
		got := w.inbox("member")
		if len(got) != before+1 || got[0] != "unblocked: Shawn finished Electrical: Inspection still waits for Plumbing" {
			t.Fatalf("after the first blocker: %q", got)
		}
		w.finish("owner", plumb)
		w.run()
		if got := w.inbox("member"); got[0] != "unblocked: Shawn finished Plumbing: Inspection can start" {
			t.Fatalf("after the last blocker: %q", got)
		}

		// Someone who finishes the blocker themselves hears nothing.
		paint := w.task("owner", w.project, "Paint", nil)
		trim := w.task("owner", w.project, "Trim", nil)
		w.assign("owner", trim, "member", false)
		w.waitFor("owner", trim, paint)
		w.run()
		n := len(w.inbox("member"))
		w.finish("member", paint)
		w.run()
		if got := w.inbox("member"); len(got) != n {
			t.Fatalf("finishing it yourself notified you: %q", got)
		}

		// A guest waiting on a task they cannot see hears nothing of it.
		hidden := uuid.NewString()
		w.push("owner", "project", hidden, map[string]any{"name": "Hidden", "sort_key": "a1"})
		secret := w.task("owner", hidden, "Secret", nil)
		shown := w.task("owner", w.project, "Shown", nil)
		w.assign("owner", shown, "guest", false)
		w.waitFor("owner", shown, secret)
		w.run()
		g := len(w.inbox("guest"))
		w.finish("owner", secret)
		w.run()
		if got := w.inbox("guest"); len(got) != g {
			t.Fatalf("the guest heard of a hidden task: %q", got)
		}
	})
}

// pushes records what would go to people's devices.
type pushes struct{ sent []notify.PushMessage }

func (p *pushes) Push(_ context.Context, _ string, m notify.PushMessage) error {
	p.sent = append(p.sent, m)
	return nil
}

func TestAChangeToManyIsPushedAsOne(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		p := &pushes{}
		w.svc.SetPusher(p)
		var tasks []string
		for _, title := range []string{"Order seeds", "Mend the fence", "Plant garlic"} {
			tasks = append(tasks, w.task("owner", w.project, title, nil))
		}
		// One change assigns all three; another, alone, a fourth task.
		batch := uuid.NewString()
		var ops []syncer.Op
		for _, task := range tasks {
			raw := map[string]json.RawMessage{}
			for k, v := range map[string]any{"task_id": task, "user_id": w.users["member"]} {
				b, _ := json.Marshal(v)
				raw[k] = b
			}
			seq++
			ops = append(ops, syncer.Op{
				OpID: uuid.NewString(), Entity: "task_assignee", ID: pairID(task, w.users["member"]),
				HLC: fmt.Sprintf("%013d-%04x-%s", time.Now().UnixMilli(), seq, "owner"), Fields: raw, Batch: batch,
			})
		}
		_, _, err := w.sync.Push(w.ctx, w.users["owner"], w.ws, ops)
		must(t, err)
		alone := w.task("owner", w.project, "Turn the compost", nil)
		w.assign("owner", alone, "member", false)
		w.run()
		w.run()
		// Each task is in the list; the lone one is pushed at once, the batch waits.
		if got := w.inbox("member"); len(got) != 4 {
			t.Fatalf("inbox: %q", got)
		}
		if len(p.sent) != 1 || p.sent[0].Title != "Shawn assigned you to Turn the compost" {
			t.Fatalf("pushed at once: %+v", p.sent)
		}
		w.clock = w.clock.Add(10 * time.Second)
		w.run()
		w.run()
		if len(p.sent) != 2 || p.sent[1].Title != "Shawn assigned you 3 tasks" || p.sent[1].Body != "In Home" {
			t.Fatalf("pushed for the batch: %+v", p.sent)
		}
		// Nothing more goes later.
		w.clock = w.clock.Add(time.Minute)
		w.run()
		if len(p.sent) != 2 {
			t.Fatalf("pushed again: %+v", p.sent[2:])
		}
	})
}
