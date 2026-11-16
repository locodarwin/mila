package push

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/push/pushtest"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

type world struct {
	t      *testing.T
	ctx    context.Context
	d      *db.DB
	st     *store.Store
	runner *jobs.Runner
	svc    *Service
	fake   *pushtest.Server

	mu  sync.Mutex
	now time.Time
}

func newWorld(t *testing.T, d *db.DB) *world {
	w := &world{t: t, ctx: context.Background(), d: d, st: store.New(d), now: store.Now()}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w.runner = jobs.New(w.st, log, w.clock)
	w.fake = pushtest.New(t)
	svc, err := New(w.ctx, w.st, log, w.runner, Options{Subject: "mailto:mila@example.com", Client: w.fake.Client(), AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	w.svc = svc
	w.fake.Key = svc.PublicKey()
	return w
}

func (w *world) clock() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *world) advance(d time.Duration) {
	w.mu.Lock()
	w.now = w.now.Add(d)
	w.mu.Unlock()
}

// user makes a person with one session, and returns both IDs.
func (w *world) user(name string) (userID, sessionID string) {
	userID = uuid.NewString()
	err := w.st.CreateUser(w.ctx, sqlitedb.CreateUserParams{ID: userID, Email: name + "@example.com", Name: name, Now: store.Now()})
	if err != nil {
		w.t.Fatal(err)
	}
	return userID, w.session(userID)
}

func (w *world) session(userID string) string {
	id := uuid.NewString()
	err := w.st.CreateSession(w.ctx, sqlitedb.CreateSessionParams{
		ID: id, TokenHash: uuid.NewString(), UserID: userID, UserAgent: "test", Now: store.Now(), ExpiresAt: store.Now().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) subscribe(userID, sessionID, device string) {
	w.t.Helper()
	sub := w.fake.Subscribe(device)
	if err := w.svc.Subscribe(w.ctx, userID, sessionID, "Firefox", Subscription(sub)); err != nil {
		w.t.Fatal(err)
	}
}

// run does the due jobs and returns how many are left to retry.
func (w *world) run() int {
	w.t.Helper()
	if _, err := w.runner.RunDue(w.ctx); err != nil {
		w.t.Fatal(err)
	}
	var n int
	if err := w.d.QueryRowContext(w.ctx, "SELECT COUNT(*) FROM jobs WHERE done_at IS NULL").Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

func (w *world) devices(userID string) int {
	n, err := w.svc.Devices(w.ctx, userID)
	if err != nil {
		w.t.Fatal(err)
	}
	return n
}

func TestKeyIsMadeOnceAndKept(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		again, err := New(w.ctx, w.st, slog.New(slog.NewTextHandler(io.Discard, nil)), jobs.New(w.st, slog.Default(), nil), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if again.PublicKey() != w.svc.PublicKey() {
			t.Fatal("a restart made a new key; every browser would have to subscribe again")
		}
		if raw, err := b64.DecodeString(w.svc.PublicKey()); err != nil || len(raw) != 65 || raw[0] != 4 {
			t.Fatalf("public key %q is not an uncompressed P-256 point", w.svc.PublicKey())
		}
	})
}

func TestPushReachesEveryDevice(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shawn, laptop := w.user("shawn")
		phone := w.session(shawn)
		dana, danaSession := w.user("dana")
		w.subscribe(shawn, laptop, "laptop")
		w.subscribe(shawn, phone, "phone")
		w.subscribe(dana, danaSession, "dana")
		if n := w.devices(shawn); n != 2 {
			t.Fatalf("devices = %d", n)
		}

		err := w.svc.Push(w.ctx, shawn, notify.PushMessage{Title: "Reminder: Water the plants", Body: "Due today at 14:30. In Home",
			URL: "/?task=t1&ws=w1", Tag: "t1", Urgent: true})
		if err != nil {
			t.Fatal(err)
		}
		if failed := w.run(); failed != 0 {
			t.Fatalf("%d jobs failed", failed)
		}
		got := w.fake.Received()
		if len(got) != 2 {
			t.Fatalf("received %d pushes: %+v", len(got), got)
		}
		for _, m := range got {
			if m.Device == "dana" {
				t.Fatal("someone else's device got the push")
			}
			if m.Title != "Reminder: Water the plants" || m.Body != "Due today at 14:30. In Home" || m.URL != "/?task=t1&ws=w1" || m.Tag != "t1" {
				t.Fatalf("message %+v", m)
			}
			if m.Urgency != "high" || m.TTL != "86400" {
				t.Fatalf("urgency %q, ttl %q", m.Urgency, m.TTL)
			}
		}
		subs, _ := w.st.ListLivePushSubscriptions(w.ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: shawn, Now: w.clock()})
		for _, s := range subs {
			if !s.LastUsedAt.Valid {
				t.Fatalf("%s has no last use", s.Endpoint)
			}
		}

		// Ordinary notifications go at normal urgency.
		_ = w.svc.Push(w.ctx, dana, notify.PushMessage{Title: "Shawn assigned you to Fix the gate", Tag: "t2"})
		w.run()
		if got := w.fake.Received(); len(got) != 1 || got[0].Urgency != "normal" {
			t.Fatalf("received %+v", got)
		}

		// Someone with no devices is no work at all.
		nobody, _ := w.user("nobody")
		if err := w.svc.Push(w.ctx, nobody, notify.PushMessage{Title: "x"}); err != nil {
			t.Fatal(err)
		}
		if ran, _ := w.runner.RunDue(w.ctx); ran != 0 {
			t.Fatalf("ran %d jobs for nobody", ran)
		}
	})
}

func TestFailingAndExpiredDevices(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shawn, laptop := w.user("shawn")
		phone, tablet := w.session(shawn), w.session(shawn)
		w.subscribe(shawn, laptop, "laptop")
		w.subscribe(shawn, phone, "phone")
		w.subscribe(shawn, tablet, "tablet")
		w.fake.Answer("phone", http.StatusGone)
		w.fake.Answer("tablet", http.StatusServiceUnavailable)

		_ = w.svc.Push(w.ctx, shawn, notify.PushMessage{Title: "Dana mentioned you", Tag: "t1"})
		if failed := w.run(); failed != 1 {
			t.Fatalf("failed = %d, want the tablet's", failed)
		}
		if got := w.fake.Received(); len(got) != 1 || got[0].Device != "laptop" {
			t.Fatalf("received %+v", got)
		}
		// The push service forgot the phone, so Mila does too.
		if n := w.devices(shawn); n != 2 {
			t.Fatalf("devices = %d after a 410", n)
		}

		// The tablet's service recovers; the retry reaches it alone.
		w.fake.Answer("tablet", 0)
		w.advance(time.Minute)
		if failed := w.run(); failed != 0 {
			t.Fatalf("retry failed %d", failed)
		}
		if got := w.fake.Received(); len(got) != 1 || got[0].Device != "tablet" || got[0].Title != "Dana mentioned you" {
			t.Fatalf("retry received %+v", got)
		}

		// A test push says what happened on each device.
		w.fake.Answer("tablet", http.StatusNotFound)
		w.fake.Answer("laptop", http.StatusInternalServerError)
		w.fake.Answer("phone", 0)
		w.subscribe(shawn, phone, "phone")
		r, err := w.svc.SendTest(w.ctx, shawn)
		if err != nil {
			t.Fatal(err)
		}
		if r != (TestResult{Sent: 1, Failed: 1, Removed: 1}) {
			t.Fatalf("test result %+v", r)
		}
		if got := w.fake.Received(); len(got) != 1 || got[0].Device != "phone" || got[0].Title != "Mila test push" || got[0].URL != "/settings/notifications" {
			t.Fatalf("test push received %+v", got)
		}
	})
}

func TestSignedOutDevicesGetNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shawn, laptop := w.user("shawn")
		phone := w.session(shawn)
		w.subscribe(shawn, laptop, "laptop")
		w.subscribe(shawn, phone, "phone")

		// Queued while both were signed in; the phone signs out before
		// the job runs.
		_ = w.svc.Push(w.ctx, shawn, notify.PushMessage{Title: "Assigned", Tag: "t1"})
		if _, err := w.st.RevokeSession(w.ctx, sqlitedb.RevokeSessionParams{ID: phone, UserID: shawn, Now: store.NullNow()}); err != nil {
			t.Fatal(err)
		}
		w.run()
		if got := w.fake.Received(); len(got) != 1 || got[0].Device != "laptop" {
			t.Fatalf("received %+v", got)
		}
		if n := w.devices(shawn); n != 1 {
			t.Fatalf("devices = %d", n)
		}
		if err := w.svc.tidy(w.ctx); err != nil {
			t.Fatal(err)
		}
		subs, _ := w.st.ListLivePushSubscriptions(w.ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: shawn, Now: w.clock()})
		var count int
		_ = d.QueryRowContext(w.ctx, "SELECT COUNT(*) FROM push_subscriptions").Scan(&count)
		if len(subs) != 1 || count != 1 {
			t.Fatalf("after tidy: %d live, %d rows", len(subs), count)
		}

		// Signing in again on the phone and subscribing brings it back.
		again := w.session(shawn)
		w.subscribe(shawn, again, "phone")
		if n := w.devices(shawn); n != 2 {
			t.Fatalf("devices = %d after signing in again", n)
		}
	})
}

// A message queued for one person never reaches the next one to sign in
// on the same browser, even when it is retried after the takeover.
func TestRetryNeverReachesTheNextAccount(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shawn, laptop := w.user("shawn")
		dana, danaSession := w.user("dana")
		sub := Subscription(w.fake.Subscribe("laptop"))
		if err := w.svc.Subscribe(w.ctx, shawn, laptop, "Firefox", sub); err != nil {
			t.Fatal(err)
		}
		w.fake.Answer("laptop", http.StatusServiceUnavailable)
		_ = w.svc.Push(w.ctx, shawn, notify.PushMessage{Title: "Lee mentioned you", Body: "the gate code is 4412", Tag: "t1"})
		if failed := w.run(); failed != 1 {
			t.Fatalf("failed = %d, want the laptop's", failed)
		}

		// Shawn's laptop session ends; Dana signs in on it and the app
		// sends the browser's subscription for her.
		if _, err := w.st.RevokeSession(w.ctx, sqlitedb.RevokeSessionParams{ID: laptop, UserID: shawn, Now: store.NullNow()}); err != nil {
			t.Fatal(err)
		}
		if err := w.svc.Subscribe(w.ctx, dana, danaSession, "Firefox", sub); err != nil {
			t.Fatal(err)
		}
		w.fake.Answer("laptop", 0)
		w.advance(time.Minute)
		if left := w.run(); left != 0 {
			t.Fatalf("%d jobs left", left)
		}
		if got := w.fake.Received(); len(got) != 0 {
			t.Fatalf("Dana's browser got Shawn's message: %+v", got)
		}

		// The same goes for the same person signing in again: the message
		// was for a session that has ended.
		_ = w.svc.Push(w.ctx, dana, notify.PushMessage{Title: "For Dana", Tag: "t2"})
		w.fake.Answer("laptop", http.StatusServiceUnavailable)
		w.run()
		again := w.session(dana)
		if err := w.svc.Subscribe(w.ctx, dana, again, "Firefox", sub); err != nil {
			t.Fatal(err)
		}
		w.fake.Answer("laptop", 0)
		w.advance(time.Minute)
		w.run()
		if got := w.fake.Received(); len(got) != 0 {
			t.Fatalf("received %+v after signing in again", got)
		}
	})
}

// A push service that never answers costs one send now and then, not one
// per message, and holds up no other device.
// only is the ID of a person's one browser.
func (w *world) only(userID string) string {
	w.t.Helper()
	subs, err := w.st.ListLivePushSubscriptions(w.ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: userID, Now: w.clock()})
	if err != nil || len(subs) != 1 {
		w.t.Fatalf("browsers = %v (%v)", subs, err)
	}
	return subs[0].ID
}

// A browser whose session is about to end is told once, with nothing about
// anyone's work, and told again only if the end was put off and comes near
// again. A session at its 90 days needs a new sign-in, and says so.
func TestLapsingSessionsAreWarned(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		const day = 24 * time.Hour
		const open = "Open Mila to keep notifications on this device."
		const again = "Sign in to Mila again to keep notifications on this device."
		told := func(want ...string) {
			t.Helper()
			if err := w.svc.warnLapsing(w.ctx); err != nil {
				t.Fatal(err)
			}
			if failed := w.run(); failed != 0 {
				t.Fatalf("%d jobs failed", failed)
			}
			var bodies []string
			for _, m := range w.fake.Received() {
				bodies = append(bodies, m.Body)
			}
			if !slices.Equal(bodies, want) {
				t.Fatalf("pushes = %q, want %q", bodies, want)
			}
		}
		ends := func(session string, in time.Duration) {
			t.Helper()
			err := w.st.TouchSession(w.ctx, sqlitedb.TouchSessionParams{ID: session, Now: w.clock(), ExpiresAt: w.clock().Add(in)})
			if err != nil {
				t.Fatal(err)
			}
		}

		shawn, session := w.user("shawn")
		w.subscribe(shawn, session, "laptop")
		ends(session, 10*day)
		told()

		// Two days left, and told once however often the job runs.
		ends(session, 2*day)
		told(open)
		told()

		// Opened: 30 more days, and nothing until the end is near again.
		// The warning is moved back by hand rather than the clock forward:
		// the fake push service checks signatures against the real time.
		ends(session, 30*day)
		told()
		err := w.st.SetPushSubscriptionWarned(w.ctx, sqlitedb.SetPushSubscriptionWarnedParams{
			ID: w.only(shawn), Now: sql.NullTime{Time: w.clock().Add(-28 * day), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		ends(session, 2*day)
		told(open)

		// Nearing 90 days, using it cannot help.
		dana, _ := w.user("dana")
		old := uuid.NewString()
		started := w.clock().Add(2 * day).Add(-accounts.SessionMaxAge)
		err = w.st.CreateSession(w.ctx, sqlitedb.CreateSessionParams{ID: old, TokenHash: uuid.NewString(), UserID: dana, UserAgent: "test", Now: started, ExpiresAt: w.clock().Add(2 * day)})
		if err != nil {
			t.Fatal(err)
		}
		phone := Subscription(w.fake.Subscribe("phone"))
		if err := w.svc.Subscribe(w.ctx, dana, old, "Firefox", phone); err != nil {
			t.Fatal(err)
		}
		told(again)

		// The same browser under a new session starts afresh.
		fresh := w.session(dana)
		if err := w.svc.Subscribe(w.ctx, dana, fresh, "Firefox", phone); err != nil {
			t.Fatal(err)
		}
		ends(fresh, 2*day)
		told(open)
	})
}

func TestSilentPushServiceIsLeftAlone(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		w.svc.timeout = 200 * time.Millisecond
		var mu sync.Mutex
		hits, answer := 0, false
		stop := make(chan struct{})
		silent := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Reading the body lets the server see the client give up.
			_, _ = io.Copy(io.Discard, r.Body)
			mu.Lock()
			hits++
			ok := answer
			mu.Unlock()
			if ok {
				rw.WriteHeader(http.StatusCreated)
				return
			}
			select {
			case <-r.Context().Done():
			case <-stop:
			}
		}))
		t.Cleanup(silent.Close)
		t.Cleanup(func() { close(stop) })
		count := func() int {
			mu.Lock()
			defer mu.Unlock()
			return hits
		}

		shawn, session := w.user("shawn")
		w.subscribe(shawn, session, "laptop")
		for _, name := range []string{"phone", "tablet"} {
			sub := Subscription(w.fake.Subscribe(name))
			sub.Endpoint = silent.URL + "/send/" + name
			if err := w.svc.Subscribe(w.ctx, shawn, session, "Firefox", sub); err != nil {
				t.Fatal(err)
			}
		}
		push := func(title string) {
			t.Helper()
			if err := w.svc.Push(w.ctx, shawn, notify.PushMessage{Title: title}); err != nil {
				t.Fatal(err)
			}
		}

		push("one")
		if left := w.run(); left != 2 {
			t.Fatalf("%d jobs left, want the silent devices'", left)
		}
		if got := w.fake.Received(); len(got) != 1 || got[0].Title != "one" {
			t.Fatalf("laptop received %+v", got)
		}
		first := count()
		if first < 1 || first > 2 {
			t.Fatalf("the silent service was tried %d times", first)
		}

		// While it is left alone, its messages fail at once. The run also
		// seals, signs and does its job bookkeeping, which take hundreds of
		// milliseconds on a loaded machine under the race detector, so the
		// timeout is made far longer than that for the limit to mean anything.
		push("two")
		short := w.svc.timeout
		w.svc.timeout = 10 * time.Second
		start := time.Now()
		w.run()
		if took := time.Since(start); took >= w.svc.timeout/2 {
			t.Fatalf("sending took %v; the silent service was waited for again", took)
		}
		w.svc.timeout = short
		if count() != first {
			t.Fatal("the silent service was tried again at once")
		}
		if got := w.fake.Received(); len(got) != 1 || got[0].Title != "two" {
			t.Fatalf("laptop received %+v", got)
		}

		// After a while one send finds out whether it answers again.
		w.advance(quiet)
		w.run()
		if n := count() - first; n != 1 {
			t.Fatalf("the silent service was tried %d times after the quiet spell, want 1", n)
		}

		// Once it answers, everything waiting goes out.
		mu.Lock()
		answer = true
		mu.Unlock()
		for range jobs.MaxAttempts {
			w.advance(quiet + jobs.Backoff(jobs.MaxAttempts))
			if w.run() == 0 {
				break
			}
		}
		var failed int
		if err := d.QueryRowContext(w.ctx, "SELECT COUNT(*) FROM jobs WHERE done_at IS NULL").Scan(&failed); err != nil || failed != 0 {
			t.Fatalf("%d pushes never went out (%v)", failed, err)
		}
	})
}

func TestSubscribe(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		shawn, laptop := w.user("shawn")
		dana, danaSession := w.user("dana")
		good := w.fake.Subscribe("laptop")
		invalid := func(sub Subscription, why string) {
			t.Helper()
			err := w.svc.Subscribe(w.ctx, shawn, laptop, "", sub)
			var e *apperr.Error
			if !errors.As(err, &e) || e.Kind != apperr.Invalid {
				t.Errorf("%s: err = %v", why, err)
			}
		}
		with := func(f func(*Subscription)) Subscription { s := Subscription(good); f(&s); return s }
		invalid(with(func(s *Subscription) { s.Endpoint = "ftp://push.example.com/x" }), "ftp endpoint")
		invalid(with(func(s *Subscription) { s.Endpoint = "https://user:pw@push.example.com/x" }), "credentials in the endpoint")
		invalid(with(func(s *Subscription) { s.Endpoint = "https://push.example.com/" + strings.Repeat("x", maxEndpoint) }), "long endpoint")
		invalid(with(func(s *Subscription) { s.Endpoint = "/relative" }), "relative endpoint")
		invalid(with(func(s *Subscription) { s.P256dh = "BAAA" }), "short key")
		invalid(with(func(s *Subscription) { s.P256dh = b64.EncodeToString(append([]byte{4}, make([]byte, 64)...)) }), "key off the curve")
		invalid(with(func(s *Subscription) { s.Auth = "AAAA" }), "short secret")
		invalid(with(func(s *Subscription) { s.Auth = "!!!" }), "secret not base64")

		// Browsers may pad their keys.
		padded := with(func(s *Subscription) { s.Auth += "==" })
		if err := w.svc.Subscribe(w.ctx, shawn, laptop, strings.Repeat("Mozilla ", 100), padded); err != nil {
			t.Fatal(err)
		}
		// Only https endpoints, outside tests.
		strict := &Service{}
		if err := strict.checkEndpoint(good.Endpoint); err == nil {
			t.Error("an http endpoint was accepted")
		}
		if err := strict.checkEndpoint("https://fcm.googleapis.com/fcm/send/abc"); err != nil {
			t.Error(err)
		}

		// The same browser signed in as Dana takes the row over.
		if err := w.svc.Subscribe(w.ctx, dana, danaSession, "", Subscription(good)); err != nil {
			t.Fatal(err)
		}
		if w.devices(shawn) != 0 || w.devices(dana) != 1 {
			t.Fatalf("devices: shawn %d, dana %d", w.devices(shawn), w.devices(dana))
		}
		// Unsubscribing only reaches one's own.
		if err := w.svc.Unsubscribe(w.ctx, shawn, good.Endpoint); err != nil || w.devices(dana) != 1 {
			t.Fatalf("someone else's unsubscribe removed Dana's device: %v", err)
		}
		if err := w.svc.Unsubscribe(w.ctx, dana, good.Endpoint); err != nil || w.devices(dana) != 0 {
			t.Fatalf("unsubscribe: %v, devices %d", err, w.devices(dana))
		}

		// One person keeps at most maxDevices, the least recently used
		// going first.
		for i := range maxDevices + 3 {
			w.subscribe(shawn, laptop, "d"+string(rune('a'+i)))
		}
		if n := w.devices(shawn); n != maxDevices {
			t.Fatalf("devices = %d", n)
		}
	})
}

// The client that reaches real push services refuses private addresses,
// including names that resolve to one, and does not follow redirects.
func TestPublicClientRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := publicClient()
	for _, u := range []string{srv.URL, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1), "http://[::1]:9/", "http://10.0.0.1:9/", "http://169.254.169.254/"} {
		resp, err := c.Get(u)
		if err == nil {
			_ = resp.Body.Close()
			t.Errorf("%s was reached", u)
		} else if !errors.Is(err, errPrivate) {
			t.Errorf("%s: %v", u, err)
		}
	}
	if err := c.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Error("redirects are followed")
	}
	for addr, want := range map[string]bool{"8.8.8.8": true, "2606:4700::1": true, "100.64.1.1": false, "192.168.1.1": false, "::ffff:127.0.0.1": false, "fd00::1": false, "0.0.0.0": false} {
		if got := public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("public(%s) = %v", addr, got)
		}
	}
}
