package accounts

import (
	"context"
	"database/sql"
	"strings"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Content is what deleting an account needs from sync: taking the person
// out of the content of a workspace they leave behind (their assignments
// end on every device; what only they saw goes).
type Content interface {
	Forget(ctx context.Context, workspaceID, userID string) error
}

// SetContent connects sync for account deletion.
func (s *Service) SetContent(c Content) { s.content = c }

// DeletedName is what a deleted account is called wherever its work stays.
const DeletedName = "Deleted user"

// DeleteAccount removes a person from Mila, once they have typed their
// email address to confirm. Their personal workspace goes, and so does
// every shared one where they are the only member. Shared workspaces with
// others in them keep what the person wrote, credited to "Deleted user",
// while their assignments end and their private rows go. It is refused
// while they are the last owner of a workspace others use (ownership must
// move first) or the instance's last admin; a workspace already deleted
// never stops it. Every session ends.
func (s *Service) DeleteAccount(ctx context.Context, userID, typedEmail string) error {
	var pausedIn []string
	defer func() {
		for _, ws := range pausedIn {
			s.notices.WebhooksPaused(ctx, ws)
		}
	}()
	user, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.DeletedAt.Valid {
		return fail(NotFound, "This account is already deleted.")
	}
	if !strings.EqualFold(strings.TrimSpace(typedEmail), user.Email) {
		return fail(Invalid, "Type your email address, %s, to confirm.", user.Email)
	}
	if user.IsInstanceAdmin {
		n, err := s.st.CountInstanceAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return fail(Conflict, "You are this instance's only admin. Make someone else an admin before deleting your account.")
		}
	}
	memberships, err := s.st.ListUserMemberships(ctx, userID)
	if err != nil {
		return err
	}
	var blockers []string
	var purge, leave []string
	owned := map[string]bool{} // workspaces left behind that they own
	coMembers := map[string]bool{}
	for _, m := range memberships {
		lastOwner := authz.Role(m.Role) == authz.Owner && m.Owners <= 1
		switch {
		case m.IsPersonal || m.Members <= 1:
			purge = append(purge, m.ID)
		case lastOwner && m.DeletedAt.Valid:
			// Deleted already, by them: nobody can reach it to take it
			// over, so it goes now rather than block them for good.
			purge = append(purge, m.ID)
		case lastOwner:
			blockers = append(blockers, m.Name)
		default:
			leave = append(leave, m.ID)
			owned[m.ID] = authz.Role(m.Role) == authz.Owner
		}
	}
	if len(blockers) > 0 {
		return fail(Conflict, "You are the only owner of %s. Make someone else an owner, or remove the other members, first.",
			strings.Join(blockers, ", "))
	}
	for _, ws := range leave {
		ids, err := s.st.ListMemberIDs(ctx, ws)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if id != userID {
				coMembers[id] = true
			}
		}
	}

	// Workspaces still being imported for them go with the rest.
	importing, err := s.st.ListUnfinishedImportWorkspaces(ctx, userID)
	if err != nil {
		return err
	}
	for _, ws := range importing {
		purge = append(purge, ws.String)
	}
	sessions, err := s.st.ListActiveSessions(ctx, sqlitedb.ListActiveSessionsParams{UserID: userID, Now: store.Now()})
	if err != nil {
		return err
	}

	// Content first, through sync, so the other members' devices hear of
	// it; a failure leaves the account as it was, and trying again picks
	// up where this stopped.
	if s.content != nil {
		for _, ws := range leave {
			if err := s.content.Forget(ctx, ws, userID); err != nil {
				return err
			}
		}
	}
	err = s.st.InTx(ctx, func(q store.Queries) error {
		// The checks again, under locks, now that nothing can change
		// under them: an admin or co-owner leaving at the same moment
		// must not leave the instance without an admin, or a workspace
		// without an owner.
		if user.IsInstanceAdmin {
			if err := q.LockInstanceAdmins(ctx); err != nil {
				return err
			}
			n, err := q.CountInstanceAdmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return fail(Conflict, "You are this instance's only admin. Make someone else an admin before deleting your account.")
			}
		}
		for _, ws := range leave {
			if !owned[ws] {
				continue
			}
			if err := q.LockOwners(ctx, ws); err != nil {
				return err
			}
			n, err := q.CountOwners(ctx, ws)
			if err != nil {
				return err
			}
			if n <= 1 {
				return fail(Conflict, "Another owner of a workspace you share just left. Make someone else an owner there, then try again.")
			}
		}
		if err := forgetInNotifications(ctx, q, user); err != nil {
			return err
		}
		for _, ws := range purge {
			if err := q.PurgeWorkspace(ctx, ws); err != nil {
				return err
			}
		}
		hooked, err := q.ListWebhookWorkspacesOf(ctx, userID)
		if err != nil {
			return err
		}
		for _, ws := range hooked {
			paused := false
			if err := s.pauseWebhooksOf(ctx, q, ws, userID, &paused); err != nil {
				return err
			}
			if paused {
				pausedIn = append(pausedIn, ws)
			}
		}
		for _, del := range []func(context.Context, string) error{
			q.DeleteUserMemberships, q.DeleteUserProjectAccess, q.DeleteUserNotifications, q.DeleteUserDigest, q.DeleteUserWeeklyReports,
			q.DeleteUserDueNotices, q.DeleteUserIcalTokens, q.DeleteUserApiTokens, q.DeleteUserOAuthCodes, q.DeleteUserDesktopCodes, q.DeleteUserInboundAddresses, q.DeleteUserInboundMail, q.DeleteUserPushSubscriptions, q.DeleteUserExports,
			q.DeleteUserImports, q.DeleteUserPasskeys, q.DeleteUserSessions, q.DeleteUserSecurityEvents,
			q.DeleteUserKnownDevices, q.DeleteUserThemes, q.DeleteUserEmailChanges,
		} {
			if err := del(ctx, userID); err != nil {
				return err
			}
		}
		if err := q.DeleteUserChallenges(ctx, sql.NullString{String: userID, Valid: true}); err != nil {
			return err
		}
		if err := q.DeleteMagicLinks(ctx, user.Email); err != nil {
			return err
		}
		return q.AnonymizeUser(ctx, sqlitedb.AnonymizeUserParams{
			ID: userID, Email: "deleted-" + userID + "@deleted.invalid", Now: store.NullNow(),
		})
	})
	if err != nil {
		return err
	}
	for _, sess := range sessions {
		s.events.SessionEnded(sess.ID)
	}
	others := make([]string, 0, len(coMembers)+1)
	for id := range coMembers {
		others = append(others, id)
	}
	s.events.MembershipChanged(append(others, userID)...)
	s.log.Info("account deleted", "user", userID, "workspaces_removed", len(purge), "workspaces_left", len(leave))
	return nil
}

// inviteAccepted is notify.InviteAccepted (notify imports accounts).
const inviteAccepted = "invite_accepted"

// forgetInNotifications takes a person's name and address out of the
// notifications their actions sent others, which name them in their text:
// they read "Deleted user" there as everywhere else.
func forgetInNotifications(ctx context.Context, q store.Queries, user store.User) error {
	list, err := q.ListActorNotifications(ctx, sql.NullString{String: user.ID, Valid: true})
	if err != nil {
		return err
	}
	for _, n := range list {
		title, body := n.Title, n.Body
		if rest, ok := strings.CutPrefix(title, user.Name); ok && user.Name != "" {
			title = DeletedName + rest
		}
		if n.Kind == inviteAccepted {
			body = DeletedName + " accepted your invitation."
		}
		if title == n.Title && body == n.Body {
			continue
		}
		if err := q.UpdateNotificationText(ctx, sqlitedb.UpdateNotificationTextParams{ID: n.ID, Title: title, Body: body}); err != nil {
			return err
		}
	}
	return nil
}

// FormerMember is someone whose work is in a workspace but who no longer
// belongs to it: they left, or deleted their account.
type FormerMember = sqlitedb.ListFormerMembersRow

// FormerMembers lists the people who once worked in a workspace, so their
// comments keep a name.
func (s *Service) FormerMembers(ctx context.Context, userID, workspaceID string) ([]FormerMember, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ViewPeople); err != nil {
		return nil, err
	}
	return s.st.ListFormerMembers(ctx, workspaceID)
}
