package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/locodarwin/mila/server/internal/authz"
	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// WorkspaceSummary is a workspace as one member sees it.
type WorkspaceSummary = sqlitedb.ListWorkspacesForUserRow

// Member is a workspace member with their role.
type Member = sqlitedb.ListMembersRow

// Workspaces lists the workspaces a user belongs to, personal first.
func (s *Service) Workspaces(ctx context.Context, userID string) ([]WorkspaceSummary, error) {
	return s.st.ListWorkspacesForUser(ctx, userID)
}

// access loads the workspace and the user's role in it, and checks the
// role allows action. Non-members get NotFound, so workspace IDs reveal
// nothing to outsiders.
func (s *Service) access(ctx context.Context, q store.Queries, userID, workspaceID string, action authz.Action) (store.Workspace, authz.Role, error) {
	ws, err := q.GetWorkspace(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ws, "", fail(NotFound, "No such workspace.")
	}
	if err != nil {
		return ws, "", err
	}
	m, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return ws, "", fail(NotFound, "No such workspace.")
	}
	if err != nil {
		return ws, "", err
	}
	role := authz.Role(m.Role)
	if !authz.Can(role, action) {
		return ws, role, fail(Forbidden, "Your role in this workspace does not allow that.")
	}
	return ws, role, nil
}

// CreateWorkspace makes a shared workspace owned by the user.
func (s *Service) CreateWorkspace(ctx context.Context, userID, name string) (WorkspaceSummary, error) {
	name, err := cleanName(name, "Workspace name")
	if err != nil {
		return WorkspaceSummary{}, err
	}
	id := newID()
	slug := slugFor(name)
	err = s.st.InTx(ctx, func(q store.Queries) error {
		now := store.Now()
		if err := q.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: id, Name: name, Slug: slug, IsPersonal: false, Now: now}); err != nil {
			return err
		}
		return q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: id, UserID: userID, Role: string(authz.Owner), Now: now})
	})
	if err == nil {
		s.events.MembershipChanged(userID)
	}
	return WorkspaceSummary{ID: id, Name: name, Slug: slug, IsPersonal: false, Role: string(authz.Owner)}, err
}

// RenameWorkspace changes a workspace's name.
func (s *Service) RenameWorkspace(ctx context.Context, userID, workspaceID, name string) error {
	name, err := cleanName(name, "Workspace name")
	if err != nil {
		return err
	}
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.RenameWorkspace); err != nil {
		return err
	}
	return s.st.RenameWorkspace(ctx, sqlitedb.RenameWorkspaceParams{ID: workspaceID, Name: name, Now: store.Now()})
}

// DeleteWorkspace removes a shared workspace. A personal workspace stays
// as long as its user does.
func (s *Service) DeleteWorkspace(ctx context.Context, userID, workspaceID string) error {
	ws, _, err := s.access(ctx, s.st, userID, workspaceID, authz.DeleteWorkspace)
	if err != nil {
		return err
	}
	if ws.IsPersonal {
		return fail(Invalid, "Your personal workspace cannot be deleted.")
	}
	members, err := s.st.ListMemberIDs(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.st.DeleteWorkspace(ctx, sqlitedb.DeleteWorkspaceParams{ID: workspaceID, Now: store.NullNow()}); err != nil {
		return err
	}
	for _, id := range members {
		s.events.AccessChanged(workspaceID, id)
	}
	s.events.MembershipChanged(members...)
	return nil
}

// Members lists a workspace's members.
func (s *Service) Members(ctx context.Context, userID, workspaceID string) ([]Member, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ViewMembers); err != nil {
		return nil, err
	}
	return s.st.ListMembers(ctx, workspaceID)
}

// Person is someone in a workspace as anyone in it may know them: a name
// and a picture, no email address. A guest's Projects are those they are
// given, as far as the one asking may know.
type Person struct {
	ID       string
	Name     string
	Avatar   sql.NullString
	Guest    bool
	Projects []string
}

// People lists who the asker can see in a workspace. Members see everyone.
// A guest sees the members, who reach every project, and only the guests who
// share a project with them, and of those only the projects they share.
func (s *Service) People(ctx context.Context, userID, workspaceID string) ([]Person, error) {
	_, actor, err := s.access(ctx, s.st, userID, workspaceID, authz.ViewPeople)
	if err != nil {
		return nil, err
	}
	members, err := s.st.ListMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	access, err := s.st.ListGuestAccess(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	projects := map[string][]string{}
	for _, a := range access {
		if a.ProjectID != "" {
			projects[a.UserID] = append(projects[a.UserID], a.ProjectID)
		}
	}
	out := make([]Person, 0, len(members))
	for _, m := range members {
		p := Person{ID: m.ID, Name: m.Name, Avatar: m.Avatar, Guest: m.Role == string(authz.Guest)}
		if p.Guest {
			p.Projects = projects[m.ID]
			if actor == authz.Guest && m.ID != userID {
				p.Projects = slices.DeleteFunc(slices.Clone(p.Projects), func(id string) bool {
					return !slices.Contains(projects[userID], id)
				})
				if len(p.Projects) == 0 {
					continue
				}
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// ChangeRole moves a member to a new role. A workspace always keeps at
// least one owner. A move to or from guest changes which projects the
// member sees, so their devices load the workspace afresh; a guest who
// moves up loses the projects given to them one by one.
func (s *Service) ChangeRole(ctx context.Context, userID, workspaceID, memberID string, to authz.Role) error {
	defer s.membershipChanged(memberID)
	reset, paused := false, false
	defer func() {
		if paused {
			s.notices.WebhooksPaused(ctx, workspaceID)
		}
	}()
	err := s.st.InTx(ctx, func(q store.Queries) error {
		_, actor, err := s.access(ctx, q, userID, workspaceID, authz.ManageMembers)
		if err != nil {
			return err
		}
		target, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: memberID})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "That person is not a member of this workspace.")
		}
		if err != nil {
			return err
		}
		from := authz.Role(target.Role)
		if !to.Valid() {
			return fail(Invalid, "%q is not a role.", to)
		}
		if !authz.CanChangeRole(actor, from, to) {
			return fail(Forbidden, "Your role does not allow that change.")
		}
		if err := keepAnOwner(ctx, q, workspaceID, from, to); err != nil {
			return err
		}
		if err := q.UpdateMemberRole(ctx, sqlitedb.UpdateMemberRoleParams{WorkspaceID: workspaceID, UserID: memberID, Role: string(to)}); err != nil {
			return err
		}
		if !authz.Can(to, authz.ReceiveEmail) {
			if err := q.DeleteMemberInboundAddresses(ctx, sqlitedb.DeleteMemberInboundAddressesParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
				return err
			}
		}
		if !authz.Can(to, authz.ManageWebhooks) {
			if err := s.pauseWebhooksOf(ctx, q, workspaceID, memberID, &paused); err != nil {
				return err
			}
		}
		if reset = from != to && (from == authz.Guest || to == authz.Guest); !reset {
			return nil
		}
		// Projects are given to guests only. A guest who becomes a member
		// sees them all anyway, and should they become a guest again, they
		// start with none rather than with whatever they had before.
		if from == authz.Guest {
			if err := q.DeleteMemberProjectAccess(ctx, sqlitedb.DeleteMemberProjectAccessParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
				return err
			}
		}
		// A member's export holds what a guest may not see.
		if to == authz.Guest {
			if err := q.DeleteMemberExports(ctx, sqlitedb.DeleteMemberExportsParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
				return err
			}
		}
		return store.ResetMember(ctx, q, workspaceID, memberID)
	})
	if err == nil && reset {
		s.events.AccessChanged(workspaceID, memberID)
	}
	return err
}

// pauseWebhooksOf pauses the webhooks someone made in a workspace, as they stop
// being able to manage it: one they set up must not go on sending the workspace's
// work to an address of theirs. Another owner or admin can turn it back on, which
// makes it theirs.
func (s *Service) pauseWebhooksOf(ctx context.Context, q store.Queries, workspaceID, userID string, paused *bool) error {
	n, err := q.PauseMemberWebhooks(ctx, sqlitedb.PauseMemberWebhooksParams{
		CreatedBy: userID, WorkspaceID: workspaceID, Now: store.NullNow(),
	})
	if n > 0 {
		*paused = true
	}
	return err
}

// RemoveMember takes someone out of a workspace. Members may always remove
// themselves (leave), except the last owner.
func (s *Service) RemoveMember(ctx context.Context, userID, workspaceID, memberID string) error {
	paused := false
	defer func() {
		if paused {
			s.notices.WebhooksPaused(ctx, workspaceID)
		}
	}()
	defer s.membershipChanged(memberID)
	err := s.st.InTx(ctx, func(q store.Queries) error {
		action := authz.ManageMembers
		if memberID == userID {
			action = authz.ViewWorkspace
		}
		ws, actor, err := s.access(ctx, q, userID, workspaceID, action)
		if err != nil {
			return err
		}
		if ws.IsPersonal {
			return fail(Invalid, "Nobody can leave or be removed from a personal workspace.")
		}
		target, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: memberID})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "That person is not a member of this workspace.")
		}
		if err != nil {
			return err
		}
		from := authz.Role(target.Role)
		if memberID != userID && !authz.CanRemove(actor, from) {
			return fail(Forbidden, "Your role does not allow removing that member.")
		}
		if err := keepAnOwner(ctx, q, workspaceID, from, ""); err != nil {
			return err
		}
		if err := q.DeleteMembership(ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: workspaceID, UserID: memberID}); err != nil {
			return err
		}
		if err := q.DeleteMemberProjectAccess(ctx, sqlitedb.DeleteMemberProjectAccessParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
			return err
		}
		if err := q.DeleteMemberNotifications(ctx, sqlitedb.DeleteMemberNotificationsParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
			return err
		}
		// Their calendar feeds end with the membership, rather than waiting
		// to serve again should they rejoin.
		if err := s.pauseWebhooksOf(ctx, q, workspaceID, memberID, &paused); err != nil {
			return err
		}
		if err := q.RevokeMemberApiTokens(ctx, sqlitedb.RevokeMemberApiTokensParams{
			UserID: memberID, WorkspaceID: sql.NullString{String: workspaceID, Valid: true}, Now: store.NullNow(),
		}); err != nil {
			return err
		}
		if err := q.DeleteMemberInboundAddresses(ctx, sqlitedb.DeleteMemberInboundAddressesParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
			return err
		}
		if err := q.DeleteMemberInboundMail(ctx, sqlitedb.DeleteMemberInboundMailParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
			return err
		}
		// An export made while they were a member is not theirs to keep.
		if err := q.DeleteMemberExports(ctx, sqlitedb.DeleteMemberExportsParams{UserID: memberID, WorkspaceID: workspaceID}); err != nil {
			return err
		}
		return q.RevokeMemberIcalTokens(ctx, sqlitedb.RevokeMemberIcalTokensParams{UserID: memberID, WorkspaceID: workspaceID, Now: store.NullNow()})
	})
	if err == nil {
		// Their open sockets leave the workspace's documents now.
		s.events.AccessChanged(workspaceID, memberID)
	}
	return err
}

// membershipChanged notifies a user; a no-op change costs one harmless
// workspace reload on their devices.
func (s *Service) membershipChanged(userID string) { s.events.MembershipChanged(userID) }

func keepAnOwner(ctx context.Context, q store.Queries, workspaceID string, from, to authz.Role) error {
	if from != authz.Owner || to == authz.Owner {
		return nil
	}
	n, err := q.CountOwners(ctx, workspaceID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return fail(Conflict, "A workspace needs at least one owner. Make someone else an owner first.")
	}
	return nil
}

// Invite emails someone a link to join a shared workspace.
func (s *Service) Invite(ctx context.Context, inviter store.User, workspaceID, rawEmail string, role authz.Role) (store.Invite, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return store.Invite{}, err
	}
	ws, actor, err := s.access(ctx, s.st, inviter.ID, workspaceID, authz.InviteMembers)
	if err != nil {
		return store.Invite{}, err
	}
	if ws.IsPersonal {
		return store.Invite{}, fail(Invalid, "Personal workspaces are just for you. Create a shared workspace to invite people.")
	}
	if !role.Valid() {
		return store.Invite{}, fail(Invalid, "%q is not a role.", role)
	}
	if !authz.CanInviteAs(actor, role) {
		return store.Invite{}, fail(Forbidden, "Only an owner can invite another owner.")
	}
	if existing, err := s.st.GetUserByEmail(ctx, email); err == nil {
		_, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: existing.ID})
		if err == nil {
			return store.Invite{}, fail(Conflict, "%s is already a member.", email)
		}
	}

	now := store.Now()
	// Invitations go out by email under the instance's name, with words
	// the inviter chose (their name, the workspace's): a cap on how many
	// can wait keeps a workspace from sending mail in bulk.
	pending, err := s.st.ListPendingInvites(ctx, sqlitedb.ListPendingInvitesParams{WorkspaceID: workspaceID, Now: now})
	if err != nil {
		return store.Invite{}, err
	}
	if len(pending) >= maxPendingInvites {
		return store.Invite{}, &Error{Kind: Conflict, Code: "too_many_invites",
			Message: fmt.Sprintf("%s has %d invitations waiting, the most it can have. Cancel some, or wait for them to be accepted.", ws.Name, maxPendingInvites)}
	}
	token, hash := newToken()
	id := newID()
	if err := s.st.CreateInvite(ctx, sqlitedb.CreateInviteParams{
		ID: id, WorkspaceID: workspaceID, Email: email, Role: string(role), TokenHash: hash,
		InvitedBy: inviter.ID, Now: now, ExpiresAt: now.Add(inviteLifetime),
	}); err != nil {
		return store.Invite{}, err
	}
	link := s.cfg.BaseURL + "/invite?token=" + token
	err = s.mail.Send(ctx, mailer.Message{
		To:      email,
		Subject: inviter.Name + " invited you to " + ws.Name + " on Mila",
		Text: inviter.Name + " invited you to join the workspace " + ws.Name + " on Mila.\n\nOpen this link to accept:\n\n" + link +
			"\n\nThe invitation expires in 7 days.\n",
	})
	if err != nil {
		return store.Invite{}, err
	}
	invites, err := s.st.ListPendingInvites(ctx, sqlitedb.ListPendingInvitesParams{WorkspaceID: workspaceID, Now: now})
	for _, inv := range invites {
		if inv.ID == id {
			return inv, err
		}
	}
	return store.Invite{}, err
}

// PendingInvites lists a workspace's open invitations.
func (s *Service) PendingInvites(ctx context.Context, userID, workspaceID string) ([]store.Invite, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.InviteMembers); err != nil {
		return nil, err
	}
	return s.st.ListPendingInvites(ctx, sqlitedb.ListPendingInvitesParams{WorkspaceID: workspaceID, Now: store.Now()})
}

// RevokeInvite cancels an open invitation.
func (s *Service) RevokeInvite(ctx context.Context, userID, workspaceID, inviteID string) error {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.InviteMembers); err != nil {
		return err
	}
	n, err := s.st.RevokeInvite(ctx, sqlitedb.RevokeInviteParams{ID: inviteID, WorkspaceID: workspaceID, Now: store.NullNow()})
	if err == nil && n == 0 {
		return fail(NotFound, "No such invitation.")
	}
	return err
}

// InvitePreview is what the invite page shows before accepting.
type InvitePreview = sqlitedb.GetPendingInviteRow

// PreviewInvite describes an invitation from its link token.
func (s *Service) PreviewInvite(ctx context.Context, token string) (InvitePreview, error) {
	inv, err := s.st.GetPendingInvite(ctx, sqlitedb.GetPendingInviteParams{TokenHash: hashToken(token), Now: store.Now()})
	if errors.Is(err, sql.ErrNoRows) {
		return inv, fail(NotFound, "This invitation has expired, was canceled or was already used.")
	}
	return inv, err
}

// AcceptInvite joins the invited workspace. Holding the link proves the
// invited address, so someone new gets an account for it on the spot. A
// signed-in user joins as themselves, whatever address the invite named,
// as a member or guest: a link to run the workspace (admin, owner) is for
// the address it was sent to, since anyone it was passed on to would
// otherwise take the role.
// The result carries a session token only when a new account was made.
// Joining goes in the person's security log.
func (s *Service) AcceptInvite(ctx context.Context, token string, current *store.User, name string, c Client) (*SignedIn, string, error) {
	var user store.User
	var workspaceID, inviterID string
	created := false
	err := s.st.InTx(ctx, func(q store.Queries) error {
		inv, err := q.GetPendingInvite(ctx, sqlitedb.GetPendingInviteParams{TokenHash: hashToken(token), Now: store.Now()})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "This invitation has expired, was canceled or was already used.")
		}
		if err != nil {
			return err
		}
		workspaceID, inviterID = inv.WorkspaceID, inv.InvitedBy
		// Whoever sent it must still be able to: an invitation is not a way
		// back in for someone removed, or to a role they no longer hold.
		gone := fail(NotFound, "This invitation no longer works: whoever sent it can no longer invite you to %s. Ask for a new one.", inv.WorkspaceName)
		sender, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: inv.WorkspaceID, UserID: inv.InvitedBy})
		if errors.Is(err, sql.ErrNoRows) {
			return gone
		}
		if err != nil {
			return err
		}
		if role := authz.Role(sender.Role); !authz.Can(role, authz.InviteMembers) || !authz.CanInviteAs(role, authz.Role(inv.Role)) {
			return gone
		}
		switch {
		case current != nil:
			if role := authz.Role(inv.Role); role != authz.Member && role != authz.Guest && !strings.EqualFold(current.Email, inv.Email) {
				return fail(Forbidden, "This invitation is for %s, to help run %s. Sign in as %s to accept it, or ask for one to your own address.", inv.Email, inv.WorkspaceName, inv.Email)
			}
			user = *current
		default:
			user, err = q.GetUserByEmail(ctx, inv.Email)
			if errors.Is(err, sql.ErrNoRows) {
				clean, nameErr := cleanName(name, "Name")
				if nameErr != nil {
					return nameErr
				}
				user, err = createUser(ctx, q, clean, inv.Email, false)
				created = true
			}
			if err != nil {
				return err
			}
		}
		if _, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: inv.WorkspaceID, UserID: user.ID}); err == nil {
			return fail(Conflict, "You are already a member of %s.", inv.WorkspaceName)
		}
		if n, err := q.AcceptInvite(ctx, sqlitedb.AcceptInviteParams{ID: inv.ID, Now: store.NullNow()}); err != nil {
			return err
		} else if n == 0 {
			return fail(Conflict, "This invitation was just used.")
		}
		if err := q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{
			WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role, Now: store.Now(),
		}); err != nil {
			return err
		}
		// Someone who was a member before may have a device that never
		// heard they left, still holding what they saw then. Their pulls
		// from before now answer 410, so it loads afresh.
		if err := store.ResetMember(ctx, q, inv.WorkspaceID, user.ID); err != nil {
			return err
		}
		_, err = record(ctx, q, user.ID, EventInviteAccepted, "", inv.WorkspaceName, c, false, store.Now())
		return err
	})
	if err != nil {
		return nil, "", err
	}
	s.events.MembershipChanged(user.ID)
	s.events.AccessChanged(workspaceID, user.ID)
	s.notices.InviteAccepted(ctx, workspaceID, inviterID, user.ID)
	// An existing user who is not signed in proves themselves the usual
	// way; only a brand-new account is signed in by the invite itself.
	if current == nil && created {
		in, err := s.startSession(ctx, user, c, MethodInvite)
		return in, workspaceID, err
	}
	return &SignedIn{User: user}, workspaceID, nil
}

// AutomationAccess says whether someone may see what a workspace's automations did:
// as for its rules, members and not guests.
func (s *Service) AutomationAccess(ctx context.Context, userID, workspaceID string) error {
	_, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ViewMembers)
	return err
}
