package webhooks_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/webhooks"
)

type stopped struct {
	mu    sync.Mutex
	urls  []string
	taken []string // "user url by"
}

func (s *stopped) WebhookTaken(_ context.Context, userID, _, url, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taken = append(s.taken, userID+" "+url+" "+by)
	return nil
}

func (s *stopped) WebhookStopped(_ context.Context, _, _, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.urls = append(s.urls, url)
	return nil
}

type received struct {
	event, delivery, signature string
	body                       []byte
}

// world is a workspace with an owner and a member, a receiving server and
// webhooks wired to sync.
type world struct {
	t       *testing.T
	ctx     context.Context
	st      *store.Store
	sync    *syncer.Service
	runner  *jobs.Runner
	svc     *webhooks.Service
	notices *stopped
	clock   time.Time
	ws      string
	owner   string
	member  string
	seq     int
	mu      sync.Mutex
	got     []received
	status  int
	server  *httptest.Server
}

func newWorld(t *testing.T, d *db.DB) *world {
	t.Helper()
	ctx := context.Background()
	st := store.New(d)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &world{t: t, ctx: ctx, st: st, ws: uuid.NewString(), clock: time.Now().UTC(), status: 200, notices: &stopped{}}
	w.runner = jobs.New(st, log, func() time.Time { return w.clock })
	w.sync = syncer.New(st, log, nil)
	w.svc = webhooks.New(st, log, w.runner, w.notices, webhooks.Options{BaseURL: "https://mila.example.com", AllowPrivate: true, PerWorkspace: 100})
	w.sync.SetWatcher(w.svc)
	w.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.mu.Lock()
		defer w.mu.Unlock()
		w.got = append(w.got, received{r.Header.Get("X-Mila-Event"), r.Header.Get("X-Mila-Delivery"), r.Header.Get("X-Mila-Signature"), body})
		rw.WriteHeader(w.status)
		_, _ = rw.Write([]byte("an answer"))
	}))
	t.Cleanup(w.server.Close)

	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: w.ws, Name: "Team", Slug: "team-" + w.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member"} {
		id := uuid.NewString()
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "-" + id[:8] + "@example.com", Name: role, Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: id, Role: role, Now: now}))
		if role == "owner" {
			w.owner = id
		} else {
			w.member = id
		}
	}
	return w
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (w *world) push(entity, id string, fields map[string]any) {
	w.t.Helper()
	w.seq++
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	hlc := fmt.Sprintf("%013d-%04x-owner", time.Now().UnixMilli(), w.seq)
	res, _, err := w.sync.Push(w.ctx, w.owner, w.ws, []syncer.Op{{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw}})
	must(w.t, err)
	if res[0].Status != "applied" {
		w.t.Fatalf("push %s: %s %s", entity, res[0].Status, res[0].Message)
	}
}

func (w *world) run() {
	w.t.Helper()
	if _, err := w.runner.RunDue(w.ctx); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) events() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []string{}
	for _, r := range w.got {
		out = append(out, r.event)
	}
	return out
}

func (w *world) hook(events ...string) sqlitedb.Webhook {
	w.t.Helper()
	h, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: w.server.URL, Events: events})
	must(w.t, err)
	return h
}

func want(t *testing.T, got []string, expected ...string) {
	t.Helper()
	if strings.Join(got, " ") != strings.Join(expected, " ") {
		t.Fatalf("got %v, want %v", got, expected)
	}
}

func TestEvents(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.hook("task", "comment", "project", "page", "time")
		project, task, comment, entry := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()

		w.push("project", project, map[string]any{"name": "Home", "sort_key": "a0"})
		w.push("project", project, map[string]any{"name": "House"})
		w.push("task", task, map[string]any{"title": "Paint", "sort_key": "a0", "project_id": project})
		w.push("task", task, map[string]any{"title": "Paint the porch"})
		w.push("task", task, map[string]any{"sort_key": "a1"}) // moved in its list: no news
		now := w.clock.Format(time.RFC3339)
		w.push("task", task, map[string]any{"completed_at": now})
		w.push("task", task, map[string]any{"completed_at": nil})
		w.push("comment", comment, map[string]any{"task_id": task, "author_id": w.owner, "body": "Done?"})
		w.push("comment", comment, map[string]any{"body": "Done yet?"})
		w.push("comment", comment, map[string]any{"deleted_at": now})
		// A timer is not time logged until it stops.
		w.push("time_entry", entry, map[string]any{"task_id": task, "user_id": w.owner, "day": "2026-11-03", "running": true, "started_at": now})
		w.push("time_entry", entry, map[string]any{"active_at": now})
		w.push("time_entry", entry, map[string]any{"running": false, "minutes": 30})
		w.push("time_entry", entry, map[string]any{"note": "Primer"})
		w.push("time_entry", entry, map[string]any{"deleted_at": now})
		// Nobody else's business: a private row sends nothing.
		w.push("reminder", uuid.NewString(), map[string]any{"task_id": task, "user_id": w.owner, "remind_at": now})
		w.push("task", task, map[string]any{"deleted_at": now})
		w.run()

		got := map[string]int{}
		for _, e := range w.events() {
			got[e]++
		}
		for _, e := range []string{
			"project.created", "project.changed", "task.created", "task.changed", "task.completed", "task.reopened",
			"comment.added", "comment.edited", "comment.deleted", "time.logged", "time.changed", "time.deleted", "task.deleted",
		} {
			if got[e] != 1 {
				t.Errorf("%s came %d times, in %v", e, got[e], w.events())
			}
		}
		if len(w.events()) != 13 {
			t.Fatalf("%d deliveries: %v", len(w.events()), w.events())
		}
	})
}

// A comment on a wiki page is a comment to a webhook, and a webhook for one
// project hears of those on its pages only.
func TestCommentsOnPages(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		project, other := uuid.NewString(), uuid.NewString()
		w.push("project", project, map[string]any{"name": "Home", "sort_key": "a0"})
		w.push("project", other, map[string]any{"name": "Away", "sort_key": "a1"})
		_, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: w.server.URL, Events: []string{"comment"}, ProjectID: project})
		must(t, err)

		comment := func(project string) string {
			main, thread, id := syncer.MainPageID(project), uuid.NewString(), uuid.NewString()
			w.push("page", main, map[string]any{"project_id": project, "sort_key": "a0"})
			w.push("page_thread", thread, map[string]any{"page_id": main, "created_by": w.owner, "quote": "words"})
			w.push("page_comment", id, map[string]any{"page_id": main, "thread_id": thread, "author_id": w.owner, "body": "Done?"})
			return id
		}
		comment(other)
		id := comment(project)
		w.push("page_comment", id, map[string]any{"body": "Done yet?"})
		w.run()
		// Order isn't promised: deliveries go out side by side.
		got := w.events()
		slices.Sort(got)
		if !slices.Equal(got, []string{"comment.added", "comment.edited"}) {
			t.Fatalf("deliveries: %v", got)
		}
	})
}

func TestWhatADeliveryIs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		h := w.hook("task")
		task := uuid.NewString()
		w.push("task", task, map[string]any{"title": "Paint", "sort_key": "a0"})
		w.push("comment", uuid.NewString(), map[string]any{"task_id": task, "author_id": w.owner, "body": "Not chosen"})
		w.run()
		want(t, w.events(), "task.created")

		r := w.got[0]
		parts := strings.FieldsFunc(r.signature, func(c rune) bool { return c == ',' || c == '=' })
		if len(parts) != 4 || parts[0] != "t" || parts[2] != "v1" {
			t.Fatalf("signature = %q", r.signature)
		}
		at, err := strconv.ParseInt(parts[1], 10, 64)
		must(t, err)
		if webhooks.Sign(h.Secret, time.Unix(at, 0), r.body) != r.signature {
			t.Fatalf("signature does not check out: %q", r.signature)
		}
		var body struct {
			ID, Event   string
			WorkspaceID string `json:"workspace_id"`
			URL         string
			Actor       map[string]string
			Data        map[string]any
			Changed     []string
		}
		must(t, json.Unmarshal(r.body, &body))
		if body.ID != r.delivery || body.Event != "task.created" || body.WorkspaceID != w.ws ||
			body.Actor["name"] != "owner" || body.Data["title"] != "Paint" || body.Data["id"] != task ||
			!strings.Contains(body.URL, task) || len(body.Changed) == 0 {
			t.Fatalf("body = %s", r.body)
		}

		list, err := w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		if len(list) != 1 || list[0].Status != "delivered" || list[0].Summary != "Paint" || list[0].Attempts != 1 ||
			list[0].ResponseCode.Int64 != 200 || list[0].ResponseBody != "an answer" {
			t.Fatalf("log = %+v", list)
		}
	})
}

func TestOneProject(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		mine, other := uuid.NewString(), uuid.NewString()
		w.push("project", mine, map[string]any{"name": "Mine", "sort_key": "a0"})
		w.push("project", other, map[string]any{"name": "Other", "sort_key": "a1"})
		_, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: w.server.URL, Events: []string{"task", "comment"}, ProjectID: mine})
		must(t, err)

		in, out := uuid.NewString(), uuid.NewString()
		w.push("task", in, map[string]any{"title": "In", "sort_key": "a0", "project_id": mine})
		w.push("task", out, map[string]any{"title": "Out", "sort_key": "a1", "project_id": other})
		w.push("task", uuid.NewString(), map[string]any{"title": "Inbox", "sort_key": "a2"})
		w.push("comment", uuid.NewString(), map[string]any{"task_id": in, "author_id": w.owner, "body": "In"})
		w.push("comment", uuid.NewString(), map[string]any{"task_id": out, "author_id": w.owner, "body": "Out"})
		w.run()
		got := w.events()
		slices.Sort(got)
		want(t, got, "comment.added", "task.created")
	})
}

func TestRetriesThenStops(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		h := w.hook("task")
		w.status = 503
		task := uuid.NewString()
		w.push("task", task, map[string]any{"title": "Paint", "sort_key": "a0"})

		// At once, then 1 minute, 5, 30, 2 hours and 12.
		for i, wait := range []time.Duration{0, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour} {
			w.clock = w.clock.Add(wait + time.Second)
			w.run()
			if len(w.got) != i+1 {
				t.Fatalf("after try %d, %d requests", i+1, len(w.got))
			}
		}
		w.clock = w.clock.Add(48 * time.Hour)
		w.run()
		if len(w.got) != 6 {
			t.Fatalf("a seventh try: %d", len(w.got))
		}
		list, err := w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		if list[0].Status != "failed" || list[0].Attempts != 6 || list[0].ResponseCode.Int64 != 503 {
			t.Fatalf("log = %+v", list[0])
		}
		h, err = w.st.GetWebhook(w.ctx, h.ID)
		must(t, err)
		if !h.FailingSince.Valid {
			t.Fatal("not failing")
		}

		// Three days of nothing but failures stops it, and says so.
		w.clock = h.FailingSince.Time.Add(3*24*time.Hour + time.Minute)
		w.push("task", uuid.NewString(), map[string]any{"title": "Another", "sort_key": "a1"})
		w.run()
		h, err = w.st.GetWebhook(w.ctx, h.ID)
		must(t, err)
		if !h.DisabledAt.Valid || len(w.notices.urls) != 1 || w.notices.urls[0] != w.server.URL {
			t.Fatalf("stopped = %+v, told %v", h.DisabledAt, w.notices.urls)
		}
		before := len(w.got)
		w.push("task", uuid.NewString(), map[string]any{"title": "Heard by nobody", "sort_key": "a2"})
		w.run()
		if len(w.got) != before {
			t.Fatal("a stopped webhook was sent to")
		}

		// Resending goes out all the same, and getting through clears the failures.
		w.status = 200
		off := false
		_, err = w.svc.Update(w.ctx, w.owner, w.ws, h.ID, webhooks.Patch{Paused: &off})
		must(t, err)
		must(t, w.svc.Resend(w.ctx, w.owner, w.ws, h.ID, list[0].ID))
		w.run()
		list, err = w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		var resent sqlitedb.WebhookDelivery
		for _, l := range list {
			if l.Summary == "Paint" {
				resent = l
			}
		}
		if resent.Status != "delivered" || resent.Attempts != 1 {
			t.Fatalf("resent = %+v", resent)
		}
		h, err = w.st.GetWebhook(w.ctx, h.ID)
		must(t, err)
		if h.DisabledAt.Valid || h.FailingSince.Valid {
			t.Fatalf("still off: %+v", h)
		}
	})
}

func TestWhoAndWhere(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		if _, err := w.svc.Create(w.ctx, w.member, w.ws, webhooks.Input{URL: w.server.URL, Events: []string{"task"}}); err == nil {
			t.Fatal("a member made a webhook")
		}
		if _, err := w.svc.Create(w.ctx, uuid.NewString(), w.ws, webhooks.Input{URL: w.server.URL, Events: []string{"task"}}); err == nil {
			t.Fatal("a stranger made a webhook")
		}
		h := w.hook("task")
		if _, err := w.svc.Deliveries(w.ctx, w.member, w.ws, h.ID); err == nil {
			t.Fatal("a member read the log")
		}
		if err := w.svc.Delete(w.ctx, w.owner, uuid.NewString(), h.ID); err == nil {
			t.Fatal("deleted through another workspace")
		}
		for i := 1; i < 10; i++ {
			w.hook("task")
		}
		if _, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: w.server.URL, Events: []string{"task"}}); err == nil {
			t.Fatal("an eleventh webhook")
		}
		if _, err := w.svc.Create(w.ctx, w.owner, uuid.NewString(), webhooks.Input{URL: w.server.URL, Events: []string{"nonsense"}}); err == nil {
			t.Fatal("an unknown workspace")
		}
	})
}

func TestInsideAddressesAreRefused(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		strict := webhooks.New(w.st, log, jobs.New(w.st, log, func() time.Time { return w.clock }), w.notices, webhooks.Options{})
		for _, address := range []string{
			"http://example.com/hook", "ftp://example.com", "https://localhost/hook", "https://nas.local/hook",
			"https://127.0.0.1/hook", "https://10.0.0.5/hook", "https://192.168.1.2/hook", "https://169.254.169.254/latest",
			"https://100.64.0.1/hook", "https://[::1]/hook", "https://[fd00::1]/hook", "https://user:pw@example.com/hook", "nonsense",
		} {
			if _, err := strict.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: address, Events: []string{"task"}}); err == nil {
				t.Errorf("%s was accepted", address)
			}
		}
		if _, err := strict.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: "https://hooks.example.com/mila", Events: []string{"task"}}); err != nil {
			t.Errorf("a public address: %v", err)
		}
	})
}

func TestANameInsideIsRefusedAtTheConnection(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		runner := jobs.New(w.st, log, func() time.Time { return w.clock })
		strict := webhooks.New(w.st, log, runner, w.notices, webhooks.Options{})
		// As if a name had looked fine when added and now answers with an inside
		// address: the stored address goes straight to the connection's check.
		h, err := strict.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: "https://hooks.example.com", Events: []string{"task"}})
		must(t, err)
		must(t, w.st.UpdateWebhook(w.ctx, sqlitedb.UpdateWebhookParams{ID: h.ID, Url: w.server.URL, Events: h.Events}))
		must(t, strict.Test(w.ctx, w.owner, w.ws, h.ID))
		if _, err := runner.RunDue(w.ctx); err != nil {
			t.Fatal(err)
		}
		if len(w.got) != 0 {
			t.Fatal("the request reached an inside address")
		}
		list, err := strict.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		if list[0].Status != "pending" || !strings.Contains(list[0].ResponseBody, "inside this server") {
			t.Fatalf("log = %+v", list[0])
		}
	})
}

func TestAReplyWithANulByteDoesNotJam(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		bad := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = rw.Write([]byte("ok\x00then"))
		}))
		defer bad.Close()
		h, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: bad.URL, Events: []string{"task"}})
		must(t, err)
		w.push("task", uuid.NewString(), map[string]any{"title": "Paint", "sort_key": "a0"})
		w.run()
		list, err := w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		if list[0].Status != "delivered" || list[0].ResponseBody != "okthen" {
			t.Fatalf("log = %+v", list[0])
		}
	})
}

func TestDeletedTextDoesNotGoOut(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.hook("comment", "page")
		task, comment := uuid.NewString(), uuid.NewString()
		w.push("task", task, map[string]any{"title": "Paint", "sort_key": "a0"})
		w.push("comment", comment, map[string]any{"task_id": task, "author_id": w.owner, "body": "A secret thought"})
		w.run()
		w.got = nil
		now := w.clock.Format(time.RFC3339)
		w.push("comment", comment, map[string]any{"deleted_at": now})
		w.run()
		want(t, w.events(), "comment.deleted")
		if strings.Contains(string(w.got[0].body), "secret") {
			t.Fatalf("body = %s", w.got[0].body)
		}
		if !strings.Contains(string(w.got[0].body), comment) || !strings.Contains(string(w.got[0].body), task) {
			t.Fatalf("does not say what was deleted: %s", w.got[0].body)
		}
	})
}

func TestResendingSendsOnce(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		h := w.hook("task")
		w.status = 503
		w.push("task", uuid.NewString(), map[string]any{"title": "Paint", "sort_key": "a0"})
		w.run()
		list, err := w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID)
		must(t, err)
		// Waiting for its next try, so a resend adds nothing.
		must(t, w.svc.Resend(w.ctx, w.owner, w.ws, h.ID, list[0].ID))
		must(t, w.svc.Resend(w.ctx, w.owner, w.ws, h.ID, list[0].ID))
		w.run()
		if len(w.got) != 1 {
			t.Fatalf("%d sends while waiting", len(w.got))
		}
		// Used up: resends together send once, and the old tries are gone.
		for _, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour} {
			w.clock = w.clock.Add(wait + time.Second)
			w.run()
		}
		if len(w.got) != 6 {
			t.Fatalf("%d tries", len(w.got))
		}
		w.got, w.status = nil, 200
		must(t, w.svc.Resend(w.ctx, w.owner, w.ws, h.ID, list[0].ID))
		must(t, w.svc.Resend(w.ctx, w.owner, w.ws, h.ID, list[0].ID))
		w.run()
		if len(w.got) != 1 {
			t.Fatalf("%d sends from two resends", len(w.got))
		}
	})
}

func TestOneWorkspaceCannotTakeEveryWorker(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		runner := jobs.New(w.st, log, func() time.Time { return w.clock })
		limited := webhooks.New(w.st, log, runner, w.notices, webhooks.Options{AllowPrivate: true, PerWorkspace: 1})
		var mu sync.Mutex
		now, most, sent := 0, 0, 0
		slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			mu.Lock()
			now++
			most = max(most, now)
			sent++
			mu.Unlock()
			time.Sleep(30 * time.Millisecond)
			mu.Lock()
			now--
			mu.Unlock()
		}))
		defer slow.Close()
		h, err := limited.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: slow.URL, Events: []string{"task"}})
		must(t, err)
		for range 4 {
			must(t, limited.Test(w.ctx, w.owner, w.ws, h.ID))
		}
		for range 8 {
			if _, err := runner.RunDue(w.ctx); err != nil {
				t.Fatal(err)
			}
			w.clock = w.clock.Add(6 * time.Second)
		}
		if most != 1 || sent != 4 {
			t.Fatalf("at most %d at once, %d sent", most, sent)
		}
	})
}

func TestADeletedWorkspaceSendsNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		h := w.hook("task")
		w.push("task", uuid.NewString(), map[string]any{"title": "Paint", "sort_key": "a0"})
		must(t, w.st.DeleteWorkspace(w.ctx, sqlitedb.DeleteWorkspaceParams{ID: w.ws, Now: store.NullNow()}))
		w.run()
		if len(w.got) != 0 {
			t.Fatal("sent after the workspace was deleted")
		}
		if _, err := w.svc.Deliveries(w.ctx, w.owner, w.ws, h.ID); err == nil {
			t.Fatal("its log is still open")
		}
	})
}

// Whoever sets where a webhook sends, or learns its secret, is who it is
// then: removing or demoting them pauses it, as it would one they made. An
// admin who pointed another's webhook at their own server would otherwise
// keep the workspace's work coming after they left. Who had it is told.
func TestWhoSetsAWebhooksAddressHasIt(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		admin := uuid.NewString()
		now := store.Now()
		must(t, w.st.CreateUser(w.ctx, sqlitedb.CreateUserParams{ID: admin, Email: "admin-" + admin[:8] + "@example.com", Name: "Ada", Now: now}))
		must(t, w.st.CreateMembership(w.ctx, sqlitedb.CreateMembershipParams{WorkspaceID: w.ws, UserID: admin, Role: "admin", Now: now}))
		owner := func(id string) string {
			t.Helper()
			h, err := w.st.GetWebhook(w.ctx, id)
			must(t, err)
			return h.CreatedBy
		}

		moved, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: "https://owner.example.com/hook", Events: []string{"task"}})
		must(t, err)
		elsewhere := "https://admin.example.net/collect"
		_, err = w.svc.Update(w.ctx, admin, w.ws, moved.ID, webhooks.Patch{URL: &elsewhere})
		must(t, err)
		if owner(moved.ID) != admin {
			t.Error("a webhook sent elsewhere by an admin stayed its maker's")
		}
		if want := w.owner + " " + elsewhere + " Ada"; !slices.Contains(w.notices.taken, want) {
			t.Errorf("its maker was told %v, want %q", w.notices.taken, want)
		}

		rotated, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: "https://owner.example.com/other", Events: []string{"task"}})
		must(t, err)
		_, err = w.svc.RotateSecret(w.ctx, admin, w.ws, rotated.ID)
		must(t, err)
		if owner(rotated.ID) != admin {
			t.Error("a webhook whose secret an admin made anew stayed its maker's")
		}

		// Choosing what it hears about sends nothing anywhere new.
		events, err := w.svc.Create(w.ctx, w.owner, w.ws, webhooks.Input{URL: "https://owner.example.com/third", Events: []string{"task"}})
		must(t, err)
		_, err = w.svc.Update(w.ctx, admin, w.ws, events.ID, webhooks.Patch{Events: &[]string{"task", "comment"}})
		must(t, err)
		if owner(events.ID) != w.owner {
			t.Error("a webhook whose events an admin changed became theirs")
		}
		// Nor is anyone told of a change to their own.
		told := len(w.notices.taken)
		mine := "https://owner.example.com/moved"
		_, err = w.svc.Update(w.ctx, w.owner, w.ws, events.ID, webhooks.Patch{URL: &mine})
		must(t, err)
		if len(w.notices.taken) != told {
			t.Errorf("told of a change to one's own webhook: %v", w.notices.taken)
		}

		// The admin leaves: what they hold is paused, and only that.
		n, err := w.st.PauseMemberWebhooks(w.ctx, sqlitedb.PauseMemberWebhooksParams{CreatedBy: admin, WorkspaceID: w.ws, Now: sql.NullTime{Time: now, Valid: true}})
		must(t, err)
		if n != 2 {
			t.Errorf("paused %d webhooks as the admin left, want 2", n)
		}
	})
}
