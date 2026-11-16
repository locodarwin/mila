package inbound_test

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

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

const template = "mila+{token}@example.com"

type world struct {
	t     *testing.T
	ctx   context.Context
	st    *store.Store
	sync  *syncer.Service
	svc   *inbound.Service
	told  *recorder
	ws    string
	user  string
	email string
	files *files.Store
}

type recorder struct{ notices []notify.Notice }

func (r *recorder) Send(_ context.Context, list ...notify.Notice) error {
	r.notices = append(r.notices, list...)
	return nil
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fs, err := files.New(t.TempDir())
	must(t, err)
	w := &world{t: t, ctx: ctx, st: st, sync: syncer.New(st, log, nil), told: &recorder{}, ws: uuid.NewString(), user: uuid.NewString(), files: fs}
	w.email = "owner-" + w.user[:8] + "@example.com"
	w.svc = inbound.New(st, log, w.sync, fs, w.told, template)
	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: w.user, Email: w.email, Name: "Shawn", Now: now}))
	must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: w.user, Role: "owner", Now: now}))
	return w
}

var tokenCount int

// address makes an address with a token of its own.
func (w *world) address(project string, assign bool, senders ...string) (id, token string) {
	w.t.Helper()
	tokenCount++
	token = strings.Repeat("a", 22) + strings.Map(func(r rune) rune { return 'a' + r - '0' }, fmt.Sprintf("%04d", tokenCount))
	id = uuid.NewString()
	raw, _ := json.Marshal(append([]string{}, senders...))
	params := sqlitedb.InsertInboundAddressParams{ID: id, UserID: w.user, WorkspaceID: w.ws, Token: token, AssignSelf: assign, Senders: string(raw), Now: store.Now()}
	if project != "" {
		params.ProjectID.String, params.ProjectID.Valid = project, true
	}
	must(w.t, w.st.InsertInboundAddress(w.ctx, params))
	return id, token
}

// send delivers mail, with its usual headers unless the test gives its own.
// tick waits for the clock to move, so that what's written next sorts after what was
// written last: Windows' clock moves in steps long enough for two writes to share one.
func tick() {
	for start := store.Now(); store.Now().Equal(start); {
		time.Sleep(time.Millisecond)
	}
}

func (w *world) send(token, from, headers, body string) (bool, error) {
	tick()
	w.t.Helper()
	raw := "From: " + from + "\r\nTo: mila+" + token + "@example.com\r\nMessage-ID: <" + uuid.NewString() + "@example.org>\r\nDate: Tue, 3 Nov 2026 09:12:00 -0800\r\n" +
		strings.ReplaceAll(headers, "\n", "\r\n") + "\r\n" + strings.ReplaceAll(body, "\n", "\r\n")
	m, err := inbound.Parse(strings.NewReader(raw))
	must(w.t, err)
	return w.svc.Deliver(w.ctx, m)
}

// outcomes are what the list says of the mail so far, newest first.
func (w *world) outcomes() []string {
	w.t.Helper()
	out := []string{}
	for _, r := range w.mail() {
		out = append(out, r.Outcome)
	}
	return out
}

// mail is the list of mail, as a person's page has it: newest first.
func (w *world) mail() []sqlitedb.InboundMail {
	w.t.Helper()
	made, err := w.st.ListInboundMailMade(w.ctx, sqlitedb.ListInboundMailMadeParams{UserID: w.user, WorkspaceID: w.ws})
	must(w.t, err)
	refused, err := w.st.ListInboundMailRefused(w.ctx, sqlitedb.ListInboundMailRefusedParams{UserID: w.user, WorkspaceID: w.ws})
	must(w.t, err)
	rows := append(made, refused...)
	slices.SortStableFunc(rows, func(a, b sqlitedb.InboundMail) int { return b.ReceivedAt.Compare(a.ReceivedAt) })
	return rows
}

func contextCanceled(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	cancel()
	return ctx, cancel
}

func (w *world) tasks() []syncer.Row {
	w.t.Helper()
	snap, err := w.sync.Bootstrap(w.ctx, w.user, w.ws)
	must(w.t, err)
	out := []syncer.Row{}
	for _, r := range snap.Rows {
		if r.Entity == "task" {
			out = append(out, r)
		}
	}
	return out
}

func (w *world) rows(entity string) []syncer.Row {
	w.t.Helper()
	snap, err := w.sync.Bootstrap(w.ctx, w.user, w.ws)
	must(w.t, err)
	out := []syncer.Row{}
	for _, r := range snap.Rows {
		if r.Entity == entity {
			out = append(out, r)
		}
	}
	return out
}

func wantOutcomes(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("outcomes %v, want %v", got, want)
	}
}

func TestMailBecomesATask(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		named, err := w.send(token, w.email, "Subject: Fwd: Invoice 4521\nContent-Type: text/plain\n", "Please pay.\nBy the 14th.\n\n<b>Not bold</b> and *not* a [link](javascript:x).\n")
		must(t, err)
		if !named {
			t.Fatal("not taken for ours")
		}
		tasks := w.tasks()
		if len(tasks) != 1 {
			t.Fatalf("%d tasks", len(tasks))
		}
		task := tasks[0]
		if task.Fields["title"] != "Invoice 4521" || task.Fields["project_id"] != nil {
			t.Fatalf("%v", task.Fields)
		}
		notes, _ := task.Fields["notes"].(string)
		if !strings.Contains(notes, "By email from "+w.email) || !strings.Contains(notes, "Please pay.") || !strings.Contains(notes, "By the 14th.") {
			t.Fatalf("notes %q", notes)
		}
		// What it said stays words: nothing of it is formatting or a working link.
		for _, not := range []string{"[link](", " *not* ", "<b>"} {
			if strings.Contains(notes, not) {
				t.Fatalf("%q in notes %q", not, notes)
			}
		}
		if got := task.Fields["created_by"]; got != w.user {
			t.Fatalf("made by %v", got)
		}
		if a := w.rows("task_assignee"); len(a) != 1 || a[0].Fields["user_id"] != w.user {
			t.Fatalf("assignees %v", a)
		}
		wantOutcomes(t, w.outcomes(), inbound.Made)
		if len(w.told.notices) != 1 || w.told.notices[0].Kind != notify.EmailTask || w.told.notices[0].UserID != w.user || w.told.notices[0].ActorID != "" {
			t.Fatalf("told %+v", w.told.notices)
		}
	})
}

func TestItsHistorySaysItCameByEmail(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		_, err := w.send(token, w.email, "Subject: A task\n", "Hello.\n")
		must(t, err)
		task := w.tasks()[0]
		if a := w.rows("task_assignee"); len(a) != 0 {
			t.Fatalf("assigned with the switch off: %v", a)
		}
		entries, err := w.sync.Activity(w.ctx, w.user, w.ws, task.ID)
		must(t, err)
		raw, _ := json.Marshal(entries)
		if !strings.Contains(string(raw), `"email"`) {
			t.Fatalf("no mention of email: %s", raw)
		}
	})
}

func TestItGoesToTheProjectAndItsFirstOpenStatus(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		project, backlog, todo := uuid.NewString(), uuid.NewString(), uuid.NewString()
		write := func(entity, id string, fields map[string]any) {
			raw := map[string]json.RawMessage{}
			for k, v := range fields {
				raw[k], _ = json.Marshal(v)
			}
			_, _, err := w.sync.Write(w.ctx, w.user, w.ws, syncer.Write{Entity: entity, ID: id, Mode: syncer.Create, Fields: raw})
			must(t, err)
		}
		write("project", project, map[string]any{"name": "Wholesale"})
		write("status", backlog, map[string]any{"project_id": project, "name": "Backlog", "category": "backlog"})
		write("status", todo, map[string]any{"project_id": project, "name": "To do", "category": "todo"})
		_, token := w.address(project, true)
		_, err := w.send(token, w.email, "Subject: A task\n", "Hello.\n")
		must(t, err)
		task := w.tasks()[0]
		if task.Fields["project_id"] != project || task.Fields["status_id"] != todo {
			t.Fatalf("%v", task.Fields)
		}

		// A project archived since refuses, rather than filling up unseen.
		arch, _ := json.Marshal("2026-11-03T00:00:00Z")
		_, _, err = w.sync.Write(w.ctx, w.user, w.ws, syncer.Write{Entity: "project", ID: project, Mode: syncer.Update, Fields: map[string]json.RawMessage{"archived_at": arch}})
		must(t, err)
		_, err = w.send(token, w.email, "Subject: Another\n", "Hello.\n")
		must(t, err)
		wantOutcomes(t, w.outcomes(), inbound.RefusedPlace, inbound.Made)
	})
}

func TestWhatIsRefused(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true, "listed@example.org")
		cases := []struct {
			from, headers, outcome string
		}{
			{"stranger@example.net", "Subject: Offer\n", inbound.RefusedSender},
			{w.email, "Subject: Away\nAuto-Submitted: auto-replied\n", inbound.RefusedAuto},
			{w.email, "Subject: News\nList-Id: <n.example.com>\n", inbound.RefusedAuto},
			{w.email, "Subject: Junk\nX-Spam-Flag: YES\n", inbound.RefusedSpam},
			{w.email, "Subject: Faked\nAuthentication-Results: mx; dmarc=fail\n", inbound.RefusedForged},
		}
		for _, c := range cases {
			w.told.notices = nil
			named, err := w.send(token, c.from, c.headers, "Hello.\n")
			must(t, err)
			if !named {
				t.Fatalf("%s: not taken for ours", c.outcome)
			}
			if got := w.outcomes()[0]; got != c.outcome {
				t.Fatalf("%q: %s, want %s", c.headers, got, c.outcome)
			}
			if len(w.tasks()) != 0 || len(w.told.notices) != 0 {
				t.Fatalf("%s made a task or told someone", c.outcome)
			}
		}
		// The message of mail refused is not kept: only who and the subject.
		rows := w.mail()
		if last := rows[len(rows)-1]; last.FromAddress != "stranger@example.net" || last.Subject != "Offer" {
			t.Fatalf("%+v", last)
		}

		// A listed sender, in any case, and the person's own address pass.
		_, err := w.send(token, "LISTED@example.org", "Subject: Mine\n", "Hello.\n")
		must(t, err)
		_, err = w.send(token, strings.ToUpper(w.email), "Subject: Mine too\n", "Hello.\n")
		must(t, err)
		if n := len(w.tasks()); n != 2 {
			t.Fatalf("%d tasks", n)
		}
	})
}

func TestSomeoneElsesMailIsNotOurs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		m, err := inbound.Parse(strings.NewReader("From: a@example.org\r\nTo: friend@example.org\r\nSubject: Lunch\r\n\r\nHi\r\n"))
		must(t, err)
		named, err := w.svc.Deliver(w.ctx, m)
		must(t, err)
		if named {
			t.Fatal("taken for ours")
		}
		// An address nobody holds is ours, for deleting, and leaves no trace.
		named, err = w.send(strings.Repeat("b", 26), w.email, "Subject: Lost\n", "Hi\n")
		must(t, err)
		if !named || len(w.outcomes()) != 0 || len(w.tasks()) != 0 {
			t.Fatalf("named %v, %v", named, w.outcomes())
		}
	})
}

func TestTheSameMessageTwiceIsOneTask(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		raw := "From: " + w.email + "\r\nTo: mila+" + token + "@example.com\r\nMessage-ID: <same@example.org>\r\nSubject: Twice\r\n\r\nHello\r\n"
		for range 2 {
			m, err := inbound.Parse(strings.NewReader(raw))
			must(t, err)
			_, err = w.svc.Deliver(w.ctx, m)
			must(t, err)
		}
		if n := len(w.tasks()); n != 1 {
			t.Fatalf("%d tasks", n)
		}
		wantOutcomes(t, w.outcomes(), inbound.Made)

		// In the trash, it stays there rather than coming back as new.
		gone, _ := json.Marshal("2026-11-03T00:00:00Z")
		_, _, err := w.sync.Write(w.ctx, w.user, w.ws, syncer.Write{Entity: "task", ID: w.tasks()[0].ID, Mode: syncer.Update, Fields: map[string]json.RawMessage{"deleted_at": gone}})
		must(t, err)
		m, err := inbound.Parse(strings.NewReader(raw))
		must(t, err)
		_, err = w.svc.Deliver(w.ctx, m)
		must(t, err)
		if got := w.tasks()[0].Fields["deleted_at"]; got == nil {
			t.Fatal("brought back from the trash")
		}
		wantOutcomes(t, w.outcomes(), inbound.Made)

		// The same message to another address is another task.
		_, other := w.address("", true)
		m, err = inbound.Parse(strings.NewReader(strings.Replace(raw, token, other, 1)))
		must(t, err)
		_, err = w.svc.Deliver(w.ctx, m)
		must(t, err)
		if n := len(w.tasks()); n != 2 {
			t.Fatalf("%d tasks", n)
		}
	})
}

func TestADailyLimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id, token := w.address("", false)
		for i := range 50 {
			must(t, w.st.InsertInboundMail(w.ctx, sqlitedb.InsertInboundMailParams{
				ID: uuid.NewString(), UserID: w.user, WorkspaceID: w.ws, AddressID: id, FromAddress: w.email,
				Subject: fmt.Sprint(i), Outcome: inbound.Made, Now: store.Now(),
			}))
		}
		_, err := w.send(token, w.email, "Subject: One more\n", "Hello.\n")
		must(t, err)
		if n := len(w.tasks()); n != 0 {
			t.Fatalf("%d tasks", n)
		}
		rows, err := w.st.CountInboundRefusedSince(w.ctx, sqlitedb.CountInboundRefusedSinceParams{AddressID: id, Since: store.Now().Add(-time.Hour)})
		must(t, err)
		if rows != 1 {
			t.Fatalf("%d refused", rows)
		}
	})
}

func TestRefusalsCannotFillTheList(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id, token := w.address("", false)
		for i := range inbound.MaxRefusedDay {
			must(t, w.st.InsertInboundMail(w.ctx, sqlitedb.InsertInboundMailParams{
				ID: uuid.NewString(), UserID: w.user, WorkspaceID: w.ws, AddressID: id, FromAddress: "x@example.org",
				Subject: fmt.Sprint(i), Outcome: inbound.RefusedSender, Now: store.Now(),
			}))
		}
		_, err := w.send(token, "stranger@example.org", "Subject: Number 101\n", "Hello.\n")
		must(t, err)
		n, err := w.st.CountInboundRefusedSince(w.ctx, sqlitedb.CountInboundRefusedSinceParams{AddressID: id, Since: store.Now().Add(-time.Hour)})
		must(t, err)
		if n != int64(inbound.MaxRefusedDay) {
			t.Fatalf("%d listed", n)
		}
	})
}

func TestAPersonNoLongerAMemberMakesNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		for _, role := range []string{"guest", ""} {
			w := newWorld(t, d)
			_, token := w.address("", true)
			if role == "guest" {
				must(t, w.st.UpdateMemberRole(w.ctx, sqlitedb.UpdateMemberRoleParams{WorkspaceID: w.ws, UserID: w.user, Role: role}))
			} else {
				must(t, w.st.DeleteMembership(w.ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: w.ws, UserID: w.user}))
			}
			named, err := w.send(token, w.email, "Subject: Still here?\n", "Hello.\n")
			must(t, err)
			if !named || len(w.outcomes()) != 0 {
				t.Fatalf("%q: %v %v", role, named, w.outcomes())
			}
		}
	})
}

func TestFilesComeAlong(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		_, err := w.send(token, w.email, "Subject: With a file\nContent-Type: multipart/mixed; boundary=b\n", `--b
Content-Type: text/plain

See this.
--b
Content-Type: application/pdf
Content-Disposition: attachment; filename="../../secret/invoice.pdf"
Content-Transfer-Encoding: base64

JVBERi0=
--b--
`)
		must(t, err)
		rows := w.rows("attachment")
		if len(rows) != 1 || rows[0].Fields["filename"] != "invoice.pdf" || rows[0].Fields["mime"] != "application/pdf" {
			t.Fatalf("%v", rows)
		}
		if rows[0].Fields["task_id"] != w.tasks()[0].ID || rows[0].Fields["uploaded_by"] != w.user {
			t.Fatalf("%v", rows[0].Fields)
		}
	})
}

func TestALongMessageStopsAtTheNotesLimit(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		_, err := w.send(token, w.email, "Subject: Long\n", strings.Repeat("*word* ", 40_000)+"\n")
		must(t, err)
		notes, _ := w.tasks()[0].Fields["notes"].(string)
		if len([]rune(notes)) > 100_000 || !strings.Contains(notes, "too long to keep") {
			t.Fatalf("%d characters, ends %q", len(notes), notes[max(0, len(notes)-60):])
		}
	})
}
