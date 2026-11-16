// Package push delivers web push notifications (RFC 8030) to the browsers
// people turned them on in. Messages are encrypted for each browser (RFC
// 8291) and signed for the push service with the instance's VAPID key
// (RFC 8292), which is made on first start and kept in the database.
package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	// keySetting is where the instance keeps its VAPID private key.
	keySetting = "vapid_private_key"
	// jobSend delivers one message to one browser.
	jobSend = "push.send"
	// ttl is how long a push service keeps a message for a browser that
	// is offline.
	ttl = 24 * time.Hour
	// maxPayload keeps a message well inside the one record push services
	// take.
	maxPayload = 3000
	// maxDevices is how many browsers one person can have push on; the
	// least recently used goes first.
	maxDevices  = 20
	maxEndpoint = 2048
	// senders is how many pushes go out at once. Push services are other
	// people's servers, so sending runs beside the other jobs.
	senders = 8
	// timeout bounds one send, connecting included.
	timeout = 10 * time.Second
	// quiet is how long a push service that did not answer is left alone.
	// Pushes for it meanwhile fail at once and are retried later, so one
	// that hangs costs a sender a few seconds now and then, not every
	// message's worth.
	quiet = 2 * time.Minute
)

// Options adjust the service, for tests.
type Options struct {
	// Subject tells push services how to reach the operator: a mailto:
	// or https: URL.
	Subject string
	// Client sends to push services. Nil means one that only reaches
	// public addresses.
	Client *http.Client
	// AllowLocal accepts http: endpoints, for a fake push service in tests.
	AllowLocal bool
	// Timeout bounds one send; zero means 10 seconds.
	Timeout time.Duration
}

// Service keeps subscriptions and sends pushes.
type Service struct {
	st      *store.Store
	log     *slog.Logger
	jobs    *jobs.Runner
	key     *ecdsa.PrivateKey
	public  string // base64url, uncompressed point
	subject string
	client  *http.Client
	local   bool
	timeout time.Duration

	mu    sync.Mutex
	hosts map[string]hostState // push services that did not answer lately
}

// hostState is what is known of a push service that did not answer.
type hostState struct {
	until   time.Time // left alone until then
	probing bool      // one send is finding out whether it answers again
}

// New loads the instance's VAPID key, making it on first start, and
// registers the delivery job and the hourly tidy with the runner.
func New(ctx context.Context, st *store.Store, log *slog.Logger, runner *jobs.Runner, opts Options) (*Service, error) {
	fresh, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, err := fresh.Bytes()
	if err != nil {
		return nil, err
	}
	// Concurrent first starts agree on whichever key was stored first.
	stored, err := st.InitInstanceSetting(ctx, keySetting, b64.EncodeToString(raw))
	if err != nil {
		return nil, fmt.Errorf("push key: %w", err)
	}
	raw, err = decodeKey(stored)
	if err != nil {
		return nil, fmt.Errorf("push key: %w", err)
	}
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, fmt.Errorf("push key: %w", err)
	}
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	s := &Service{st: st, log: log, jobs: runner, key: key, public: b64.EncodeToString(pub),
		subject: opts.Subject, client: opts.Client, local: opts.AllowLocal, timeout: opts.Timeout, hosts: map[string]hostState{}}
	if s.client == nil {
		s.client = publicClient()
	}
	if s.timeout <= 0 {
		s.timeout = timeout
	}
	runner.HandleConcurrent(jobSend, senders, s.handleSend)
	runner.Every("push-tidy", time.Hour, s.tidy)
	runner.Every("push-lapse", time.Hour, s.warnLapsing)
	return s, nil
}

// Subject is the contact for push services: the sender's address when it
// has one, the instance's address otherwise.
func Subject(mailFrom, baseURL string) string {
	if a, err := parseAddress(mailFrom); err == nil {
		return "mailto:" + a
	}
	return strings.TrimSuffix(baseURL, "/")
}

// PublicKey is the key browsers subscribe with (applicationServerKey).
func (s *Service) PublicKey() string { return s.public }

// Subscription is what a browser's PushSubscription.toJSON() gives.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Subscribe records a browser for the user's session, taking over the row
// if the same browser subscribed before.
func (s *Service) Subscribe(ctx context.Context, userID, sessionID, userAgent string, sub Subscription) error {
	if err := s.checkEndpoint(sub.Endpoint); err != nil {
		return err
	}
	if p, err := decodeKey(sub.P256dh); err != nil || len(p) != 65 {
		return apperr.New(apperr.Invalid, "The browser's push key is not valid.")
	} else if _, err := ecdh.P256().NewPublicKey(p); err != nil {
		return apperr.New(apperr.Invalid, "The browser's push key is not valid.")
	}
	if a, err := decodeKey(sub.Auth); err != nil || len(a) != 16 {
		return apperr.New(apperr.Invalid, "The browser's push secret is not valid.")
	}
	if utf8.RuneCountInString(userAgent) > 300 {
		userAgent = string([]rune(userAgent)[:300])
	}
	return s.st.InTx(ctx, func(q store.Queries) error {
		err := q.UpsertPushSubscription(ctx, sqlitedb.UpsertPushSubscriptionParams{
			ID: uuid.Must(uuid.NewV7()).String(), UserID: userID, SessionID: sessionID, Endpoint: sub.Endpoint,
			P256dh: sub.P256dh, Auth: sub.Auth, UserAgent: userAgent, Now: s.jobs.Now(),
		})
		if err != nil {
			return err
		}
		return q.TrimPushSubscriptions(ctx, sqlitedb.TrimPushSubscriptionsParams{OwnerID: userID, Keep: maxDevices})
	})
}

// pushServices are the push services browsers use, by the end of their
// host names: Chrome and other Chromium browsers (Google), Firefox
// (Mozilla), Safari (Apple) and Edge (Microsoft). A browser hands the
// server an address to send to, and on a home server an address of any
// other kind could reach into the house's network.
var pushServices = []string{
	"fcm.googleapis.com", "android.googleapis.com",
	"push.services.mozilla.com",
	"push.apple.com",
	"notify.windows.com",
}

// checkEndpoint accepts only https URLs on port 443 at a known push
// service, which is what browsers give; sending reaches only public
// addresses besides.
func (s *Service) checkEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	bad := apperr.New(apperr.Invalid, "That is not a push service address.")
	if err != nil {
		return bad
	}
	if s.local && u.Scheme == "http" && u.Host != "" {
		return nil
	}
	if u.Scheme != "https" || len(endpoint) > maxEndpoint || u.Host == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return bad
	}
	host := strings.ToLower(u.Hostname())
	// A name under .invalid never resolves (RFC 2606), so it can reach
	// nothing; the browser tests' made-up push service uses one.
	if strings.HasSuffix(host, ".invalid") {
		return nil
	}
	for _, service := range pushServices {
		if host == service || strings.HasSuffix(host, "."+service) {
			return nil
		}
	}
	return apperr.New(apperr.Invalid, "Mila sends only to the push services browsers use (Google, Mozilla, Apple, Microsoft).")
}

// Unsubscribe forgets one of the user's browsers.
func (s *Service) Unsubscribe(ctx context.Context, userID, endpoint string) error {
	return s.st.DeleteUserPushSubscription(ctx, sqlitedb.DeleteUserPushSubscriptionParams{UserID: userID, Endpoint: endpoint})
}

// Devices counts the user's browsers that get pushes.
func (s *Service) Devices(ctx context.Context, userID string) (int, error) {
	subs, err := s.st.ListLivePushSubscriptions(ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: userID, Now: s.jobs.Now()})
	return len(subs), err
}

// message is a push as the service worker reads it.
type message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// maxTitle is as much of a title as a notification shows.
const maxTitle = 200

// payload is the message as JSON, its title cut to maxTitle characters,
// then its body and if need be its title shortened until it fits.
func payload(m notify.PushMessage) []byte {
	msg := message{Title: cut(m.Title, maxTitle), Body: m.Body, URL: m.URL, Tag: m.Tag}
	if raw := encode(msg); len(raw) <= maxPayload {
		return raw
	}
	fit := func(field *string) {
		full := []rune(*field)
		// The longest start of the text that fits; escaping makes the JSON
		// longer than the text by an amount that depends on the text.
		lo, hi := 0, len(full)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			*field = cut(string(full[:mid]), mid-1)
			if len(encode(msg)) <= maxPayload {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		*field = cut(string(full[:lo]), lo-1)
	}
	msg.Body = ""
	if len(encode(msg)) <= maxPayload {
		msg.Body = m.Body
		fit(&msg.Body)
	} else {
		fit(&msg.Title)
	}
	return encode(msg)
}

func encode(m message) []byte {
	raw, _ := json.Marshal(m)
	return raw
}

// cut shortens s to at most n characters, ending in an ellipsis when it
// was cut.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// send is the payload of a push.send job. It names whose device it is
// for, since the same browser can be taken over by another account, or
// signed in again, before a retry.
type send struct {
	Subscription string             `json:"subscription"`
	User         string             `json:"user"`
	Session      string             `json:"session"`
	Message      notify.PushMessage `json:"message"`
	Urgent       bool               `json:"urgent"`
}

// Push queues a message for each of the user's browsers, so a browser
// whose push service is down is retried without repeating the message to
// the others. It implements notify.Pusher.
func (s *Service) Push(ctx context.Context, userID string, m notify.PushMessage) error {
	now := s.jobs.Now()
	subs, err := s.st.ListLivePushSubscriptions(ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: userID, Now: now})
	if err != nil || len(subs) == 0 {
		return err
	}
	err = s.st.InTx(ctx, func(q store.Queries) error {
		for _, sub := range subs {
			j := send{Subscription: sub.ID, User: sub.UserID, Session: sub.SessionID, Message: m, Urgent: m.Urgent}
			if err := jobs.Enqueue(ctx, q, jobSend, j, now, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.jobs.Wake()
	}
	return err
}

func (s *Service) handleSend(ctx context.Context, raw []byte) error {
	var j send
	if err := json.Unmarshal(raw, &j); err != nil {
		return err
	}
	sub, err := s.st.GetLivePushSubscription(ctx, sqlitedb.GetLivePushSubscriptionParams{ID: j.Subscription, Now: s.jobs.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil // turned off, or signed out, since
	}
	if err != nil {
		return err
	}
	if sub.UserID != j.User || sub.SessionID != j.Session {
		return nil // the browser has been signed in again since, maybe as someone else
	}
	_, err = s.deliver(ctx, sub, j.Message, j.Urgent)
	return err
}

// errGone is a push service saying the subscription no longer exists.
var errGone = errors.New("push: subscription expired")

// deliver sends one message to one browser. A subscription its push
// service no longer knows is deleted, and reported as delivered false
// with no error; other failures are errors, for the job to retry.
func (s *Service) deliver(ctx context.Context, sub sqlitedb.PushSubscription, m notify.PushMessage, urgent bool) (bool, error) {
	err := s.post(ctx, sub, payload(m), urgent)
	if errors.Is(err, errGone) {
		s.log.Info("push: subscription expired", "user", sub.UserID)
		return false, s.st.DeletePushSubscription(ctx, sub.ID)
	}
	if err != nil {
		return false, err
	}
	return true, s.st.TouchPushSubscription(ctx, sqlitedb.TouchPushSubscriptionParams{ID: sub.ID, Now: sql.NullTime{Time: s.jobs.Now(), Valid: true}})
}

func (s *Service) post(ctx context.Context, sub sqlitedb.PushSubscription, plaintext []byte, urgent bool) error {
	p256dh, err := decodeKey(sub.P256dh)
	if err != nil {
		return err
	}
	secret, err := decodeKey(sub.Auth)
	if err != nil {
		return err
	}
	body, err := seal(plaintext, p256dh, secret)
	if err != nil {
		return err
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil {
		return err
	}
	auth, err := vapid(s.key, u.Scheme+"://"+u.Host, s.subject, s.jobs.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.Itoa(int(ttl/time.Second)))
	if urgent {
		req.Header.Set("Urgency", "high")
	} else {
		req.Header.Set("Urgency", "normal")
	}
	if !s.try(u.Host) {
		return fmt.Errorf("push: %s did not answer lately; trying again later", u.Host)
	}
	// Shutting down says nothing about the push service.
	stopped := func() bool { return errors.Is(ctx.Err(), context.Canceled) }
	resp, err := s.client.Do(req)
	if err != nil {
		s.answered(u.Host, stopped())
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// A service that answers and then stalls is as good as silent.
	reason, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	s.answered(u.Host, err == nil || stopped())
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return errGone
	default:
		return fmt.Errorf("push: %s answered %d: %s", u.Host, resp.StatusCode, strings.TrimSpace(string(reason)))
	}
}

// try reports whether to send to a push service now: yes unless it did
// not answer lately, and then only one send at a time once it has been
// left alone for a while.
func (s *Service) try(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hosts[host]
	if !ok {
		return true
	}
	if h.probing || s.jobs.Now().Before(h.until) {
		return false
	}
	h.probing = true
	s.hosts[host] = h
	return true
}

// answered records whether a push service answered a send at all.
func (s *Service) answered(host string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		delete(s.hosts, host)
		return
	}
	s.hosts[host] = hostState{until: s.jobs.Now().Add(quiet)}
}

// TestResult says how a test push went.
type TestResult struct {
	Sent    int // browsers the push service took it for
	Failed  int // browsers whose push service refused it or was unreachable
	Removed int // browsers the push service no longer knows, now forgotten
}

// SendTest pushes a test message to each of the user's browsers at once,
// so the settings page can say what happened.
func (s *Service) SendTest(ctx context.Context, userID string) (TestResult, error) {
	var r TestResult
	subs, err := s.st.ListLivePushSubscriptions(ctx, sqlitedb.ListLivePushSubscriptionsParams{OwnerID: userID, Now: s.jobs.Now()})
	if err != nil {
		return r, err
	}
	m := notify.PushMessage{
		Title: "Mila test push", Body: "Push notifications reach this device.",
		URL: "/settings/notifications", Tag: "mila-test",
	}
	for _, sub := range subs {
		sent, err := s.deliver(ctx, sub, m, true)
		switch {
		case err != nil:
			s.log.Warn("push: test failed", "user", userID, "err", err)
			r.Failed++
		case sent:
			r.Sent++
		default:
			r.Removed++
		}
	}
	return r, nil
}

// warnBefore is how long before a session ends its browsers are told that
// their pushes are about to stop.
const warnBefore = 3 * 24 * time.Hour

// warnLapsing tells each browser whose session ends soon, once for each
// time it comes near, that its pushes will stop with it. The message says
// nothing about anyone's work. A session nearing its 90 days cannot be
// kept by using it, only by signing in again, which the app answers by
// moving this browser's subscription to the new session (refreshPush).
func (s *Service) warnLapsing(ctx context.Context) error {
	now := s.jobs.Now()
	subs, err := s.st.ListLapsingPushSubscriptions(ctx, sqlitedb.ListLapsingPushSubscriptionsParams{Now: now, Soon: now.Add(warnBefore)})
	if err != nil || len(subs) == 0 {
		return err
	}
	warned := false
	for _, sub := range subs {
		// A warning from before the session last moved was for an end
		// that has since been put off.
		if sub.WarnedAt.Valid && !sub.WarnedAt.Time.Before(sub.SessionExpiresAt.Add(-warnBefore)) {
			continue
		}
		m := notify.PushMessage{Title: "Mila", Body: "Open Mila to keep notifications on this device.", URL: "/", Tag: "session-ending"}
		if !sub.SessionExpiresAt.Before(sub.SessionCreatedAt.Add(accounts.SessionMaxAge)) {
			m.Body = "Sign in to Mila again to keep notifications on this device."
			m.URL = "/login?again=1"
		}
		j := send{Subscription: sub.ID, User: sub.UserID, Session: sub.SessionID, Message: m}
		err := s.st.InTx(ctx, func(q store.Queries) error {
			if err := q.SetPushSubscriptionWarned(ctx, sqlitedb.SetPushSubscriptionWarnedParams{ID: sub.ID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
				return err
			}
			return jobs.Enqueue(ctx, q, jobSend, j, now, now)
		})
		if err != nil {
			return err
		}
		warned = true
	}
	if warned {
		s.jobs.Wake()
	}
	return nil
}

// tidy removes subscriptions whose session has ended.
func (s *Service) tidy(ctx context.Context) error {
	n, err := s.st.DeleteDeadPushSubscriptions(ctx, s.jobs.Now())
	if n > 0 {
		s.log.Info("push: removed signed-out devices", "count", n)
	}
	return err
}
