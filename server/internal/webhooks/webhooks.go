// Package webhooks tells other apps when something changes in a workspace. A
// change that a webhook hears about writes a delivery in its own transaction, so
// a delivery exists exactly when the change does; sending happens afterward,
// beside the other jobs, and is tried six times.
package webhooks

import (
	"bytes"
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/ratelimit"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	jobDeliver = "webhook.deliver"
	// Sending waits on servers outside anyone's control, so it has a lane of
	// its own, and a burst waits its turn rather than holding up reminders.
	workers = 4

	maxWebhooks = 10
	// A workspace sends this many at once, so hooks at a server that never answers
	// cannot take up every worker; the rest wait a few seconds and look again.
	perWorkspace = 2
	waitForSlot  = 5 * time.Second
	// sendsAnHour is how many tries a workspace's webhooks make an hour, a
	// quarter of them at once; past it a delivery waits, untried.
	sendsAnHour  = 2000
	waitForSends = time.Minute
	// A webhook's log keeps its newest, however busy the workspace.
	keepPerHook  = 1000
	maxURL       = 2000
	keepLog      = 30 * 24 * time.Hour
	stopAfter    = 3 * 24 * time.Hour
	listLimit    = 100
	maxBody      = 256 << 10
	keepResponse = 1 << 10
	sendTimeout  = 10 * time.Second
)

// Families are what a webhook can choose to hear about.
var Families = []string{"task", "comment", "project", "page", "time"}

// retries are the waits after each failed try: six tries over about a day.
var retries = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// Notices tells someone their webhook was stopped.
type Notices interface {
	WebhookStopped(ctx context.Context, userID, workspaceID, url string) error
	// WebhookTaken tells whoever had a webhook that someone else (by,
	// their name) set where it sends or made its secret anew, so it is
	// theirs now.
	WebhookTaken(ctx context.Context, userID, workspaceID, url, by string) error
}

// Service is the webhooks of every workspace.
type Service struct {
	st           *store.Store
	log          *slog.Logger
	jobs         *jobs.Runner
	notices      Notices
	baseURL      string
	allowPrivate bool
	client       *http.Client
	limit        int
	mu           sync.Mutex
	sending      map[string]int
	// sends is each workspace's tries an hour: with ten webhooks and six
	// tries each, an address that keeps failing would otherwise take a
	// stream of them from anyone who can make a workspace.
	sends *ratelimit.Limiter
}

// Options are the operator's settings.
type Options struct {
	BaseURL string
	// AllowPrivate lets addresses inside the server's own network through, and
	// plain http with them.
	AllowPrivate bool
	// PerWorkspace is how many a workspace sends at once; 0 for the usual.
	PerWorkspace int
}

// New makes the service and registers its jobs.
func New(st *store.Store, log *slog.Logger, runner *jobs.Runner, notices Notices, o Options) *Service {
	s := &Service{
		st: st, log: log, jobs: runner, notices: notices, baseURL: strings.TrimRight(o.BaseURL, "/"),
		allowPrivate: o.AllowPrivate, client: newClient(o.AllowPrivate),
		limit: cmp.Or(o.PerWorkspace, perWorkspace), sending: map[string]int{},
		sends: ratelimit.NewPerHour(sendsAnHour, sendsAnHour/4),
	}
	runner.HandleConcurrent(jobDeliver, workers, s.handleDeliver)
	runner.Every("webhooks.tidy", time.Hour, func(ctx context.Context) error {
		if err := s.st.DeleteOldWebhookDeliveries(ctx, runner.Now().Add(-keepLog)); err != nil {
			return err
		}
		hooks, err := s.st.ListAllWebhookIDs(ctx)
		if err != nil {
			return err
		}
		for _, id := range hooks {
			if err := s.st.TrimWebhookDeliveries(ctx, sqlitedb.TrimWebhookDeliveriesParams{Webhook: id, MaxRows: keepPerHook}); err != nil {
				return err
			}
		}
		return nil
	})
	return s
}

// access checks that the user may manage the workspace's webhooks.
func (s *Service) access(ctx context.Context, userID, workspaceID string) error {
	if _, err := s.st.GetWorkspace(ctx, workspaceID); errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "No such workspace.")
	} else if err != nil {
		return err
	}
	m, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return err
	}
	if !authz.Can(authz.Role(m.Role), authz.ManageWebhooks) {
		return apperr.New(apperr.Forbidden, "Only owners and admins can manage this workspace's webhooks.")
	}
	return nil
}

// owned returns a webhook of the workspace, once the user may manage it.
func (s *Service) owned(ctx context.Context, userID, workspaceID, id string) (sqlitedb.Webhook, error) {
	if err := s.access(ctx, userID, workspaceID); err != nil {
		return sqlitedb.Webhook{}, err
	}
	h, err := s.st.GetWebhook(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && h.WorkspaceID != workspaceID) {
		return sqlitedb.Webhook{}, apperr.New(apperr.NotFound, "No such webhook.")
	}
	return h, err
}

func cleanFamilies(in []string) (string, error) {
	var out []string
	for _, f := range Families {
		if slices.Contains(in, f) {
			out = append(out, f)
		}
	}
	for _, f := range in {
		if !slices.Contains(Families, f) {
			return "", apperr.New(apperr.Invalid, "Unknown kind of event: %s", f)
		}
	}
	if len(out) == 0 {
		return "", apperr.New(apperr.Invalid, "Choose at least one thing for it to hear about.")
	}
	return strings.Join(out, ","), nil
}

// FamiliesOf reads a webhook's stored choice.
func FamiliesOf(h sqlitedb.Webhook) []string { return strings.Split(h.Events, ",") }

func newSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b)
}

// Input is what a new webhook is to be.
type Input struct {
	URL       string
	Events    []string
	ProjectID string
}

// List returns the workspace's webhooks.
func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]sqlitedb.Webhook, error) {
	if err := s.access(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	return s.st.ListWebhooks(ctx, workspaceID)
}

// Create adds a webhook, returning it with its secret.
func (s *Service) Create(ctx context.Context, userID, workspaceID string, in Input) (sqlitedb.Webhook, error) {
	if err := s.access(ctx, userID, workspaceID); err != nil {
		return sqlitedb.Webhook{}, err
	}
	address, err := checkURL(in.URL, s.allowPrivate)
	if err != nil {
		return sqlitedb.Webhook{}, err
	}
	events, err := cleanFamilies(in.Events)
	if err != nil {
		return sqlitedb.Webhook{}, err
	}
	var id string
	err = s.st.InTx(ctx, func(q store.Queries) error {
		n, err := q.CountWebhooks(ctx, workspaceID)
		if err != nil {
			return err
		}
		if n >= maxWebhooks {
			return apperr.New(apperr.Invalid, "A workspace can have 10 webhooks at most.")
		}
		id = uuid.Must(uuid.NewV7()).String()
		return q.InsertWebhook(ctx, sqlitedb.InsertWebhookParams{
			ID: id, WorkspaceID: workspaceID, CreatedBy: userID, Url: address, Secret: newSecret(), Events: events,
			ProjectID: sql.NullString{String: in.ProjectID, Valid: in.ProjectID != ""}, Now: s.jobs.Now(),
		})
	})
	if err != nil {
		return sqlitedb.Webhook{}, err
	}
	return s.st.GetWebhook(ctx, id)
}

// Patch is what a change names; nil leaves a thing as it is.
type Patch struct {
	URL       *string
	Events    *[]string
	ProjectID *string
	Paused    *bool
}

// Update changes a webhook. Unpausing also turns back on one stopped for
// failing, with its count of failures begun afresh.
func (s *Service) Update(ctx context.Context, userID, workspaceID, id string, p Patch) (sqlitedb.Webhook, error) {
	h, err := s.owned(ctx, userID, workspaceID, id)
	if err != nil {
		return h, err
	}
	had := h.CreatedBy
	if p.URL != nil {
		url, err := checkURL(*p.URL, s.allowPrivate)
		if err != nil {
			return h, err
		}
		// Whoever says where it sends has it, so it is paused when they can
		// no longer manage it, as one they made would be.
		if url != h.Url {
			h.Url, h.CreatedBy = url, userID
		}
	}
	if p.Events != nil {
		if h.Events, err = cleanFamilies(*p.Events); err != nil {
			return h, err
		}
	}
	if p.ProjectID != nil {
		h.ProjectID = sql.NullString{String: *p.ProjectID, Valid: *p.ProjectID != ""}
	}
	if p.Paused != nil {
		h.PausedAt = sql.NullTime{}
		if *p.Paused {
			h.PausedAt = sql.NullTime{Time: s.jobs.Now(), Valid: true}
		} else {
			// Turned back on by someone who can manage it, it is now theirs: what
			// they can no longer manage is what gets paused.
			h.DisabledAt, h.FailingSince, h.CreatedBy = sql.NullTime{}, sql.NullTime{}, userID
		}
	}
	err = s.st.UpdateWebhook(ctx, sqlitedb.UpdateWebhookParams{
		ID: id, CreatedBy: h.CreatedBy, Url: h.Url, Events: h.Events, ProjectID: h.ProjectID,
		PausedAt: h.PausedAt, DisabledAt: h.DisabledAt, FailingSince: h.FailingSince,
	})
	if err == nil && h.CreatedBy != had && p.URL != nil {
		s.tellTaken(ctx, had, userID, h)
	}
	return h, err
}

// tellTaken tells who had a webhook that by has it now. That it is theirs
// is what matters, so a failure to tell is only logged.
func (s *Service) tellTaken(ctx context.Context, had, by string, h sqlitedb.Webhook) {
	name := "Someone"
	if u, err := s.st.GetUser(ctx, by); err == nil && u.Name != "" {
		name = u.Name
	}
	if err := s.notices.WebhookTaken(ctx, had, h.WorkspaceID, h.Url, name); err != nil {
		s.log.Warn("telling of a webhook taken on", "err", err)
	}
}

// Delete removes a webhook and its log.
func (s *Service) Delete(ctx context.Context, userID, workspaceID, id string) error {
	if _, err := s.owned(ctx, userID, workspaceID, id); err != nil {
		return err
	}
	return s.st.DeleteWebhook(ctx, id)
}

// RotateSecret makes a new secret, returning the webhook with it. A try already
// waiting is signed with the new one.
func (s *Service) RotateSecret(ctx context.Context, userID, workspaceID, id string) (sqlitedb.Webhook, error) {
	h, err := s.owned(ctx, userID, workspaceID, id)
	if err != nil {
		return h, err
	}
	h.Secret = newSecret()
	had := h.CreatedBy
	// Whoever knows its secret could sign deliveries of their own to its
	// address, so it is theirs, as with a new address.
	h.CreatedBy = userID
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.SetWebhookSecret(ctx, sqlitedb.SetWebhookSecretParams{ID: id, Secret: h.Secret}); err != nil {
			return err
		}
		return q.UpdateWebhook(ctx, sqlitedb.UpdateWebhookParams{
			ID: id, CreatedBy: h.CreatedBy, Url: h.Url, Events: h.Events, ProjectID: h.ProjectID,
			PausedAt: h.PausedAt, DisabledAt: h.DisabledAt, FailingSince: h.FailingSince,
		})
	})
	if err == nil && had != userID {
		s.tellTaken(ctx, had, userID, h)
	}
	return h, err
}

// Test sends a ping, whatever the webhook has chosen to hear about.
func (s *Service) Test(ctx context.Context, userID, workspaceID, id string) error {
	h, err := s.owned(ctx, userID, workspaceID, id)
	if err != nil {
		return err
	}
	now := s.jobs.Now()
	delivery := uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{
		"id": delivery, "event": "ping", "at": now.Format(time.RFC3339Nano), "workspace_id": workspaceID,
	})
	if err != nil {
		return err
	}
	err = s.st.InTx(ctx, func(q store.Queries) error {
		return s.enqueue(ctx, q, h.ID, delivery, "ping", "A test from Mila", body, now)
	})
	if err == nil {
		s.jobs.Wake()
	}
	return err
}

func (s *Service) enqueue(ctx context.Context, q store.Queries, webhookID, id, event, summary string, body []byte, now time.Time) error {
	if err := q.InsertWebhookDelivery(ctx, sqlitedb.InsertWebhookDeliveryParams{
		ID: id, WebhookID: webhookID, Event: event, Summary: summary, Body: string(body), Now: now,
	}); err != nil {
		return err
	}
	return jobs.Enqueue(ctx, q, jobDeliver, ref{ID: id}, now, now)
}

// Deliveries lists a webhook's latest deliveries, newest first.
func (s *Service) Deliveries(ctx context.Context, userID, workspaceID, id string) ([]sqlitedb.WebhookDelivery, error) {
	if _, err := s.owned(ctx, userID, workspaceID, id); err != nil {
		return nil, err
	}
	return s.st.ListWebhookDeliveries(ctx, sqlitedb.ListWebhookDeliveriesParams{WebhookID: id, MaxRows: listLimit})
}

// Resend sends a delivery again and starts its tries afresh.
func (s *Service) Resend(ctx context.Context, userID, workspaceID, id, deliveryID string) error {
	if _, err := s.owned(ctx, userID, workspaceID, id); err != nil {
		return err
	}
	d, err := s.st.GetWebhookDelivery(ctx, deliveryID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && d.WebhookID != id) {
		return apperr.New(apperr.NotFound, "No such delivery.")
	}
	if err != nil {
		return err
	}
	if d.Status == "pending" {
		return nil // already on its way
	}
	now := s.jobs.Now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.SetWebhookDeliveryResult(ctx, sqlitedb.SetWebhookDeliveryResultParams{
			ID: deliveryID, Status: "pending", Attempts: 0, ResponseBody: d.ResponseBody, ResponseCode: d.ResponseCode,
			AttemptedAt: d.AttemptedAt, NextAt: sql.NullTime{Time: now, Valid: true},
		}); err != nil {
			return err
		}
		return jobs.Enqueue(ctx, q, jobDeliver, ref{ID: deliveryID, Resend: true}, now, now)
	})
	if err == nil {
		s.jobs.Wake()
	}
	return err
}

type ref struct {
	ID string `json:"id"`
	// Attempts is how many tries there had been when this job was queued, so a job
	// left over from before a resend can never send as well.
	Attempts int64 `json:"attempts,omitempty"`
	// Resend goes out even if the webhook is paused or stopped: someone asked.
	Resend bool `json:"resend,omitempty"`
}

// Sign is the signature of a body sent at a time: an HMAC-SHA256 of the time, a
// dot and the body, so a receiver can check the sender and refuse an old replay.
func Sign(secret string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", at.Unix())
	_, _ = mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", at.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func (s *Service) handleDeliver(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	d, err := s.st.GetWebhookDelivery(ctx, r.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted with its webhook
	}
	if err != nil {
		return err
	}
	if d.Status != "pending" || d.Attempts != r.Attempts {
		return nil
	}
	h, err := s.st.GetWebhook(ctx, d.WebhookID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	now := s.jobs.Now()
	result := sqlitedb.SetWebhookDeliveryResultParams{
		ID: d.ID, Attempts: d.Attempts, AttemptedAt: sql.NullTime{Time: now, Valid: true},
	}
	if _, err := s.st.GetWorkspace(ctx, h.WorkspaceID); errors.Is(err, sql.ErrNoRows) {
		result.Status, result.ResponseBody = "failed", "Not sent: the workspace was deleted."
		return s.st.SetWebhookDeliveryResult(ctx, result)
	} else if err != nil {
		return err
	}
	if !r.Resend && (h.PausedAt.Valid || h.DisabledAt.Valid) {
		result.Status, result.ResponseBody = "failed", "Not sent: the webhook was off."
		return s.st.SetWebhookDeliveryResult(ctx, result)
	}
	if ok, _ := s.sends.Take(h.WorkspaceID, 1); !ok {
		later := now.Add(waitForSends)
		return s.st.InTx(ctx, func(q store.Queries) error {
			return jobs.Enqueue(ctx, q, jobDeliver, r, later, now)
		})
	}
	if !s.takeSlot(h.WorkspaceID) {
		later := now.Add(waitForSlot)
		return s.st.InTx(ctx, func(q store.Queries) error {
			return jobs.Enqueue(ctx, q, jobDeliver, r, later, now)
		})
	}
	defer s.giveSlot(h.WorkspaceID)
	claimed, err := s.st.ClaimWebhookDelivery(ctx, sqlitedb.ClaimWebhookDeliveryParams{ID: d.ID, Attempts: d.Attempts})
	if err != nil || claimed == 0 {
		return err
	}
	result.Attempts++
	code, reply, sendErr := s.send(ctx, h, d, now)
	if code != 0 {
		result.ResponseCode = sql.NullInt64{Int64: int64(code), Valid: true}
	}
	result.ResponseBody = reply
	if sendErr == nil {
		result.Status = "delivered"
		if err := s.st.SetWebhookDeliveryResult(ctx, result); err != nil {
			return err
		}
		if h.FailingSince.Valid {
			return s.st.SetWebhookFailing(ctx, sqlitedb.SetWebhookFailingParams{ID: h.ID, DisabledAt: h.DisabledAt})
		}
		return nil
	}
	// The server stopping says nothing of the address, but the try was still made:
	// it is noted past the cancel, so the delivery goes on after the restart.
	stopping := ctx.Err() != nil
	ctx = context.WithoutCancel(ctx)
	if int(result.Attempts) <= len(retries) {
		next := now.Add(retries[result.Attempts-1])
		result.Status, result.NextAt = "pending", sql.NullTime{Time: next, Valid: true}
		r.Attempts = result.Attempts
		err = s.st.InTx(ctx, func(q store.Queries) error {
			if err := q.SetWebhookDeliveryResult(ctx, result); err != nil {
				return err
			}
			return jobs.Enqueue(ctx, q, jobDeliver, r, next, now)
		})
	} else {
		result.Status = "failed"
		err = s.st.SetWebhookDeliveryResult(ctx, result)
	}
	if err != nil || stopping {
		return err
	}
	return s.failing(ctx, h, now)
}

// takeSlot notes one more being sent for a workspace, if there is room.
func (s *Service) takeSlot(workspaceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sending[workspaceID] >= s.limit {
		return false
	}
	s.sending[workspaceID]++
	return true
}

func (s *Service) giveSlot(workspaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sending[workspaceID]--; s.sending[workspaceID] <= 0 {
		delete(s.sending, workspaceID)
	}
}

// failing notes a failure against the webhook, stopping one that has done
// nothing but fail for three days.
func (s *Service) failing(ctx context.Context, h sqlitedb.Webhook, now time.Time) error {
	since := h.FailingSince
	if !since.Valid {
		since = sql.NullTime{Time: now, Valid: true}
	}
	stopped := h.DisabledAt
	stopping := !h.DisabledAt.Valid && now.Sub(since.Time) >= stopAfter
	if stopping {
		stopped = sql.NullTime{Time: now, Valid: true}
	}
	if err := s.st.SetWebhookFailing(ctx, sqlitedb.SetWebhookFailingParams{ID: h.ID, FailingSince: since, DisabledAt: stopped}); err != nil {
		return err
	}
	if stopping && s.notices != nil {
		return s.notices.WebhookStopped(ctx, h.CreatedBy, h.WorkspaceID, h.Url)
	}
	return nil
}

// send makes one try. An answer other than 2xx is an error, with its code.
func (s *Service) send(ctx context.Context, h sqlitedb.Webhook, d sqlitedb.WebhookDelivery, now time.Time) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Url, bytes.NewReader([]byte(d.Body)))
	if err != nil {
		return 0, "The address could not be used.", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mila-Webhooks")
	req.Header.Set("X-Mila-Event", d.Event)
	req.Header.Set("X-Mila-Delivery", d.ID)
	req.Header.Set("X-Mila-Signature", Sign(h.Secret, now, []byte(d.Body)))
	res, err := s.client.Do(req)
	if err != nil {
		// Only the reason: an address can hold a secret in its query, and errors quote it.
		s.log.Debug("webhook not delivered", "webhook", h.ID, "why", reason(err))
		return 0, reason(err), err
	}
	defer func() { _ = res.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(res.Body, keepResponse))
	// Postgres refuses a NUL in text, and a delivery must not jam on what a receiver says.
	text := strings.ReplaceAll(strings.ToValidUTF8(string(reply), ""), "\x00", "")
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, text, fmt.Errorf("answered %d", res.StatusCode)
	}
	return res.StatusCode, text, nil
}

// reason says why no answer came, for the log, without a server's internals.
func reason(err error) string {
	switch {
	case errors.Is(err, errRefusedAddress):
		return "Refused: the address is inside this server's network."
	case errors.Is(err, context.DeadlineExceeded):
		return "No answer within 10 seconds."
	}
	return "Could not connect."
}
