package accounts

import (
	"context"
	"database/sql"
	"errors"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// ProjectGuest is one of a workspace's guests, and whether a project is
// shared with them (SharedAt set).
type ProjectGuest = sqlitedb.ListProjectGuestsRow

// project checks that a project is in the workspace and not deleted.
func project(ctx context.Context, q store.Queries, workspaceID, projectID string) error {
	ws, err := q.GetLiveProjectWorkspace(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && ws != workspaceID) {
		return fail(NotFound, "No such project.")
	}
	return err
}

// ProjectGuests lists a workspace's guests with whether each sees the
// project. Members and above see every project, so only guests are
// listed.
func (s *Service) ProjectGuests(ctx context.Context, userID, workspaceID, projectID string) ([]ProjectGuest, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ShareProjects); err != nil {
		return nil, err
	}
	if err := project(ctx, s.st, workspaceID, projectID); err != nil {
		return nil, err
	}
	return s.st.ListProjectGuests(ctx, sqlitedb.ListProjectGuestsParams{WorkspaceID: workspaceID, ProjectID: projectID})
}

// ShareProject gives a guest a project (on) or takes it away. Either way
// the guest's devices load the workspace afresh: the project's rows
// arrive, or disappear. Asking for what is already so changes nothing.
func (s *Service) ShareProject(ctx context.Context, userID, workspaceID, projectID, guestID string, on bool) error {
	changed := false
	err := s.st.InTx(ctx, func(q store.Queries) error {
		if _, _, err := s.access(ctx, q, userID, workspaceID, authz.ShareProjects); err != nil {
			return err
		}
		if err := project(ctx, q, workspaceID, projectID); err != nil {
			return err
		}
		m, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: guestID})
		if errors.Is(err, sql.ErrNoRows) {
			return fail(NotFound, "That person is not a member of this workspace.")
		}
		if err != nil {
			return err
		}
		if authz.Role(m.Role) != authz.Guest {
			return fail(Invalid, "Members see every project already; only guests are given projects one by one.")
		}
		var n int64
		if on {
			n, err = q.GrantProjectAccess(ctx, sqlitedb.GrantProjectAccessParams{ProjectID: projectID, UserID: guestID, Now: store.Now()})
		} else {
			n, err = q.RevokeProjectAccess(ctx, sqlitedb.RevokeProjectAccessParams{ProjectID: projectID, UserID: guestID})
			if err == nil && n > 0 {
				err = q.DeleteGuestProjectNotifications(ctx, sqlitedb.DeleteGuestProjectNotificationsParams{UserID: guestID, ProjectID: sql.NullString{String: projectID, Valid: true}})
			}
			// Their exports from before hold the project.
			if err == nil && n > 0 {
				err = q.DeleteMemberExports(ctx, sqlitedb.DeleteMemberExportsParams{UserID: guestID, WorkspaceID: workspaceID})
			}
		}
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return store.ResetMember(ctx, q, workspaceID, guestID)
	})
	if err == nil && changed {
		s.events.AccessChanged(workspaceID, guestID)
		// Who can reach the project changed, so who can be mentioned on it did.
		s.peopleChanged(ctx, guestID)
	}
	return err
}
