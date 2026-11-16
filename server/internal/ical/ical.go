// Package ical serves calendar feeds: read-only iCalendar (RFC 5545) links
// that calendar apps subscribe to, of a person's own tasks, a project's or
// a saved view's. A feed's link holds a random token, of which only the
// hash is kept; what it shows is worked out at every fetch from what its
// owner can see then, so losing access ends the feed.
package ical

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Scopes: what a feed shows.
const (
	ScopeMe      = "me"      // open tasks assigned to its owner, and their My Day
	ScopeProject = "project" // a project's open tasks with a due date
	ScopeView    = "view"    // a saved view's tasks with a due date
)

const (
	// maxFeeds is how many live feeds one person may have.
	maxFeeds = 100
	// touchEvery is how often a fetch updates last_used_at at most.
	touchEvery = time.Hour
)

// tokenPattern is what a feed token looks like: 32 bytes in base64url.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// Rows is what feeds need from sync: the rows a member sees now.
type Rows interface {
	Rows(ctx context.Context, userID, workspaceID string, names ...string) ([]syncer.Row, error)
}

// Service makes, lists and revokes feeds, and builds them.
type Service struct {
	st      *store.Store
	rows    Rows
	log     *slog.Logger
	baseURL string
	host    string
	now     func() time.Time
}

// New builds the service. baseURL is the public URL, with no trailing
// slash; feed links and the links in events use it.
func New(st *store.Store, rows Rows, log *slog.Logger, baseURL string) *Service {
	host := "mila"
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	return &Service{st: st, rows: rows, log: log, baseURL: strings.TrimSuffix(baseURL, "/"), host: host, now: store.Now}
}

// SetClock replaces the clock, for tests.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Feed is a calendar feed as its owner sees it in settings.
type Feed struct {
	ID         string
	Scope      string
	TargetID   string // the project or saved view; "" for me
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func feedOf(t sqlitedb.IcalToken) Feed {
	f := Feed{ID: t.ID, Scope: t.Scope, TargetID: t.TargetID.String, Name: t.Name, CreatedAt: t.CreatedAt}
	if t.LastUsedAt.Valid {
		used := t.LastUsedAt.Time
		f.LastUsedAt = &used
	}
	return f
}

// URL is a feed's link.
func (s *Service) URL(token string) string { return s.baseURL + "/ical/" + token + ".ics" }

// access checks that the user is a member who may have feeds in the
// workspace.
func (s *Service) access(ctx context.Context, userID, workspaceID string) error {
	m, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return err
	}
	if !authz.Can(authz.Role(m.Role), authz.SubscribeCalendar) {
		return apperr.New(apperr.Forbidden, "You cannot make calendar feeds in this workspace.")
	}
	return nil
}

// List returns the user's live feeds in a workspace, oldest first.
func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]Feed, error) {
	if err := s.access(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.st.ListIcalTokens(ctx, sqlitedb.ListIcalTokensParams{UserID: userID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]Feed, 0, len(rows))
	for _, r := range rows {
		out = append(out, feedOf(r))
	}
	return out, nil
}

// Create makes a feed and returns it with its link, the only time the
// link is known: only the token's hash is kept.
func (s *Service) Create(ctx context.Context, userID, workspaceID, scope, targetID string) (Feed, string, error) {
	if err := s.access(ctx, userID, workspaceID); err != nil {
		return Feed{}, "", err
	}
	var name string
	switch scope {
	case ScopeMe:
		if targetID != "" {
			return Feed{}, "", apperr.New(apperr.Invalid, "A feed of your tasks has no project or view.")
		}
		name = "My tasks"
	case ScopeProject, ScopeView:
		entity := map[string]string{ScopeProject: "project", ScopeView: "saved_view"}[scope]
		rows, err := s.rows.Rows(ctx, userID, workspaceID, entity)
		if err != nil {
			return Feed{}, "", err
		}
		for _, r := range rows {
			if r.ID == targetID {
				name, _ = r.Fields["name"].(string)
			}
		}
		if name == "" {
			return Feed{}, "", apperr.New(apperr.NotFound, "There is no such %s, or you cannot see it.", map[string]string{ScopeProject: "project", ScopeView: "view"}[scope])
		}
	default:
		return Feed{}, "", apperr.New(apperr.Invalid, "A feed is of your tasks, a project or a view.")
	}
	n, err := s.st.CountIcalTokens(ctx, userID)
	if err != nil {
		return Feed{}, "", err
	}
	if n >= maxFeeds {
		return Feed{}, "", apperr.New(apperr.Conflict, "You have %d calendar feeds. Revoke one you no longer use first.", maxFeeds)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	now := s.now()
	t := sqlitedb.IcalToken{
		ID: uuid.Must(uuid.NewV7()).String(), UserID: userID, WorkspaceID: workspaceID, Scope: scope,
		TargetID: sql.NullString{String: targetID, Valid: targetID != ""}, TokenHash: hashToken(token),
		Name: name, CreatedAt: now,
	}
	if err := s.st.InsertIcalToken(ctx, sqlitedb.InsertIcalTokenParams{
		ID: t.ID, UserID: t.UserID, WorkspaceID: t.WorkspaceID, Scope: t.Scope, TargetID: t.TargetID,
		TokenHash: t.TokenHash, Name: t.Name, Now: now,
	}); err != nil {
		return Feed{}, "", err
	}
	return feedOf(t), s.URL(token), nil
}

// Revoke ends one of the user's feeds.
func (s *Service) Revoke(ctx context.Context, userID, workspaceID, feedID string) error {
	n, err := s.st.RevokeIcalToken(ctx, sqlitedb.RevokeIcalTokenParams{
		Now: store.NullNow(), ID: feedID, UserID: userID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(apperr.NotFound, "No such calendar feed.")
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// errNoFeed is every reason a link does not lead to a feed, so a fetch
// learns nothing about why.
var errNoFeed = apperr.New(apperr.NotFound, "No such calendar feed.")

// Calendar builds the feed a token leads to. It fails with NotFound for a
// token that is unknown or revoked, or whose owner can no longer see what
// it shows.
func (s *Service) Calendar(ctx context.Context, token string) ([]byte, error) {
	if !tokenPattern.MatchString(token) {
		return nil, errNoFeed
	}
	t, err := s.st.GetIcalTokenByHash(ctx, hashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoFeed
	}
	if err != nil {
		return nil, err
	}
	if err := s.access(ctx, t.UserID, t.WorkspaceID); err != nil {
		if apperr.KindOf(err) != 0 {
			return nil, errNoFeed
		}
		return nil, err
	}
	u, err := s.st.GetUser(ctx, t.UserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows.Rows(ctx, t.UserID, t.WorkspaceID, feedEntities...)
	if apperr.KindOf(err) == apperr.NotFound {
		return nil, errNoFeed
	}
	if err != nil {
		return nil, err
	}
	// Field values are read only for a view whose filter needs them.
	if needsFields(feedOf(t), rows) {
		fields, err := s.rows.Rows(ctx, t.UserID, t.WorkspaceID, fieldEntities...)
		if err != nil {
			return nil, err
		}
		rows = append(rows, fields...)
	}
	now := s.now()
	body, ok := s.build(feedOf(t), t.WorkspaceID, t.UserID, accounts.Location(u), rows, now)
	if !ok {
		return nil, errNoFeed
	}
	if err := s.st.TouchIcalToken(ctx, sqlitedb.TouchIcalTokenParams{
		Now: sql.NullTime{Time: now, Valid: true}, ID: t.ID, Before: sql.NullTime{Time: now.Add(-touchEvery), Valid: true},
	}); err != nil {
		s.log.Warn("record a calendar feed fetch", "err", err)
	}
	return body, nil
}
