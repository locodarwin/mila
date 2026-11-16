package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/version"
)

// The admin page, the security log and sharing projects with guests.

func (h *handlers) GetAdminStatus(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	ctx := r.Context()
	if err := h.acc.InstanceAccess(ctx, a.User.ID, authz.ViewInstance); err != nil {
		h.fail(w, r, err)
		return
	}
	users, err := h.acc.CountUsers(ctx, a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.AdminStatus{
		Version: version.Version, Commit: version.Commit, Dialect: api.AdminStatusDialect(h.dialect),
		EmailMode: api.AdminStatusEmailMode(h.mailMode), Passkeys: h.acc.PasskeysAvailable(),
		BaseUrl: h.baseURL, Users: users,
	}
	if h.emailIn != nil {
		at, why := h.emailIn()
		auth := api.Password
		out.EmailIn = &struct {
			Auth      *api.AdminStatusEmailInAuth `json:"auth,omitempty"`
			Enabled   bool                        `json:"enabled"`
			LastCheck *time.Time                  `json:"lastCheck,omitempty"`
			LastError *string                     `json:"lastError,omitempty"`
			Microsoft *api.MicrosoftMailbox       `json:"microsoft,omitempty"`
		}{Enabled: true, Auth: &auth}
		if h.microsoft != nil {
			auth = api.Microsoft
			// The sign-in's trouble is the mailbox's, said there, never the
			// whole page's: the rest of the status still shows.
			st, err := h.microsoft.State(ctx)
			if err != nil {
				h.log.Error("admin status: reading the Microsoft sign-in", "err", err)
				st = inbound.MicrosoftState{State: "lost", Problem: "Mila couldn't read the mailbox's sign-in; the server's log says why."}
			}
			m := microsoftJSON(h.microsoft.Mailbox(), st)
			out.EmailIn.Microsoft = &m
		}
		if !at.IsZero() {
			out.EmailIn.LastCheck = &at
		}
		if why != "" {
			out.EmailIn.LastError = &why
		}
	}
	if h.updates != nil {
		if r := h.updates(); r != nil {
			out.Update = &api.UpdateNotice{Version: r.Version, Url: r.URL, PublishedAt: r.PublishedAt}
		}
	}
	if h.schema != nil {
		if out.Schema, out.LatestSchema, err = h.schema.SchemaVersion(ctx); err != nil {
			h.fail(w, r, err)
			return
		}
		at, err := h.schema.LastMigrated(ctx)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if !at.IsZero() {
			out.MigratedAt = &at
		}
	}
	if h.backup != nil {
		st, err := h.backup.Status(ctx)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		nightly, hour := h.backup.Nightly()
		b := api.BackupStatus{
			Nightly: nightly, Hour: hour, Dir: h.backup.Dir(), LastOkAt: st.LastOKAt, LastErrorAt: st.LastErrorAt,
			Recent: make([]api.Backup, len(st.Recent)),
		}
		if st.LastError != "" {
			b.LastError = &st.LastError
		}
		for i, info := range st.Recent {
			b.Recent[i] = api.Backup{Name: info.Name, At: info.At, Kind: api.BackupKind(info.Kind), Schema: info.Schema, Size: info.Size}
		}
		out.Backup = &b
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) RequestBackup(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.InstanceAccess(r.Context(), a.User.ID, authz.BackUpInstance); err != nil {
		h.fail(w, r, err)
		return
	}
	if h.backup == nil {
		h.fail(w, r, apperr.New(apperr.Unavailable, "Backups are not available on this server."))
		return
	}
	if err := h.backup.Request(r.Context()); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func adminUserJSON(u accounts.AdminUser) api.AdminUser {
	return api.AdminUser{
		Id: u.ID, Email: u.Email, Name: u.Name, Avatar: avatarPath(u.Avatar), IsInstanceAdmin: u.IsInstanceAdmin,
		CreatedAt: u.CreatedAt, Devices: u.Devices, LastActiveAt: u.LastActiveAt,
	}
}

func (h *handlers) ListAdminUsers(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	users, err := h.acc.Users(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.AdminUser, len(users))
	for i, u := range users {
		out[i] = adminUserJSON(u)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) UpdateAdminUser(w http.ResponseWriter, r *http.Request, userID api.UserId) {
	a := h.requireUser(w, r)
	var body api.UpdateAdminUserJSONBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	// Making an admin hands over the instance, so a stolen session alone
	// must not be enough to make another.
	if body.IsInstanceAdmin && !h.requireRecentSignIn(w, r, a) {
		return
	}
	u, err := h.acc.SetInstanceAdmin(r.Context(), a.User.ID, userID, body.IsInstanceAdmin, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserJSON(u))
}

func (h *handlers) SignOutUser(w http.ResponseWriter, r *http.Request, userID api.UserId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	n, err := h.acc.SignOutEverywhere(r.Context(), a.User.ID, userID, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if userID == a.User.ID {
		h.clearSessionCookie(w)
	}
	writeJSON(w, http.StatusOK, struct {
		Sessions int `json:"sessions"`
	}{n})
}

func (h *handlers) ListAdminInvites(w http.ResponseWriter, r *http.Request) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	invites, err := h.acc.AllInvites(r.Context(), a.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.AdminInvite, len(invites))
	for i, inv := range invites {
		out[i] = api.AdminInvite{
			Id: inv.ID, WorkspaceId: inv.WorkspaceID, WorkspaceName: inv.WorkspaceName, Email: inv.Email,
			Role: api.Role(inv.Role), InvitedByName: inv.InvitedByName, CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CancelAdminInvite(w http.ResponseWriter, r *http.Request, inviteID api.InviteId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.CancelInvite(r.Context(), a.User.ID, inviteID); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

func (h *handlers) ListSecurityEvents(w http.ResponseWriter, r *http.Request, params api.ListSecurityEventsParams) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	before, limit := "", 0
	if params.Before != nil {
		before = *params.Before
	}
	if params.Limit != nil {
		limit = *params.Limit
	}
	events, more, err := h.acc.SecurityEvents(r.Context(), a.User.ID, before, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.SecurityEventPage{Events: make([]api.SecurityEvent, len(events)), More: more}
	for i, ev := range events {
		out.Events[i] = api.SecurityEvent{
			Id: ev.ID, Kind: api.SecurityEventKind(ev.Kind), Method: ev.Method, NewDevice: ev.NewDevice,
			Detail: ev.Detail, Ip: ev.Ip, UserAgent: ev.UserAgent, CreatedAt: ev.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ListProjectGuests(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, projectID api.ProjectId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	guests, err := h.acc.ProjectGuests(r.Context(), a.User.ID, workspaceID, projectID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]api.ProjectGuest, len(guests))
	for i, g := range guests {
		out[i] = api.ProjectGuest{UserId: g.ID, Name: g.Name, Email: g.Email, Avatar: avatarPath(g.Avatar), Shared: g.SharedAt.Valid}
		if g.SharedAt.Valid {
			out[i].SharedAt = &g.SharedAt.Time
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) ShareProject(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, projectID api.ProjectId, userID api.UserId) {
	h.share(w, r, workspaceID, projectID, userID, true)
}

func (h *handlers) UnshareProject(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, projectID api.ProjectId, userID api.UserId) {
	h.share(w, r, workspaceID, projectID, userID, false)
}

func (h *handlers) share(w http.ResponseWriter, r *http.Request, workspaceID, projectID, userID string, on bool) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.ShareProject(r.Context(), a.User.ID, workspaceID, projectID, userID, on); err != nil {
		h.fail(w, r, err)
		return
	}
	noContent(w)
}

// MicrosoftMailbox keeps Mila's sign-in to a Microsoft mailbox for email to
// task (inbound.Microsoft).
type MicrosoftMailbox interface {
	Mailbox() string
	State(ctx context.Context) (inbound.MicrosoftState, error)
	Connect(ctx context.Context, by string) (inbound.MicrosoftState, error)
	Cancel()
	Disconnect(ctx context.Context) error
}

func microsoftJSON(mailbox string, s inbound.MicrosoftState) api.MicrosoftMailbox {
	out := api.MicrosoftMailbox{
		State: api.MicrosoftMailboxState(s.State), Mailbox: mailbox,
		ConnectedAt: s.ConnectedAt, LostAt: s.LostAt, ExpiresAt: s.ExpiresAt,
	}
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	out.Account, out.ConnectedBy, out.LostWhy = str(s.Account), str(s.ConnectedBy), str(s.LostWhy)
	out.UserCode, out.VerificationUri, out.Problem = str(s.UserCode), str(s.VerificationURI), str(s.Problem)
	if s.KeyMissing {
		out.KeyMissing = &s.KeyMissing
	}
	return out
}

// microsoftAdmin is the admin asking to change the Microsoft sign-in, or nil
// once it has answered.
func (h *handlers) microsoftAdmin(w http.ResponseWriter, r *http.Request) *accounts.Authenticated {
	a := h.requireUser(w, r)
	if a == nil {
		return nil
	}
	if err := h.acc.InstanceAccess(r.Context(), a.User.ID, authz.ConnectMailbox); err != nil {
		h.fail(w, r, err)
		return nil
	}
	if h.microsoft == nil {
		h.fail(w, r, apperr.New(apperr.Invalid, "This server's mailbox takes a password: set MILA_IMAP_AUTH=microsoft to sign in to a Microsoft mailbox."))
		return nil
	}
	return a
}

func (h *handlers) ConnectMicrosoftMailbox(w http.ResponseWriter, r *http.Request) {
	a := h.microsoftAdmin(w, r)
	if a == nil {
		return
	}
	st, err := h.microsoft.Connect(r.Context(), a.User.Name)
	if err != nil {
		h.log.Warn("email to task: asking Microsoft for a code", "err", err)
		h.fail(w, r, apperr.New(apperr.Unavailable, "%s", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, microsoftJSON(h.microsoft.Mailbox(), st))
}

func (h *handlers) CancelMicrosoftSignIn(w http.ResponseWriter, r *http.Request) {
	if h.microsoftAdmin(w, r) == nil {
		return
	}
	h.microsoft.Cancel()
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) DisconnectMicrosoftMailbox(w http.ResponseWriter, r *http.Request) {
	if h.microsoftAdmin(w, r) == nil {
		return
	}
	if err := h.microsoft.Disconnect(r.Context()); err != nil {
		h.fail(w, r, err)
		return
	}
	h.log.Info("email to task: the Microsoft mailbox was disconnected")
	w.WriteHeader(http.StatusNoContent)
}
