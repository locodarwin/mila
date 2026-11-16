package accounts

import (
	"context"
	"database/sql"
	"errors"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// People's pictures. The file itself is kept by the files store under a
// random key; the user row names the key, and a new picture is a new key.

// SetAvatar gives someone a picture by its file key, or removes theirs
// when key is empty, and tells the people they share a workspace with.
// Removing a picture that is not there changes nothing and tells nobody.
func (s *Service) SetAvatar(ctx context.Context, userID, key string) (store.User, error) {
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return store.User{}, err
	}
	if u.Avatar.String == key {
		return u, nil
	}
	if err := s.setAvatar(ctx, userID, key); err != nil {
		return store.User{}, err
	}
	return s.st.GetUser(ctx, userID)
}

func (s *Service) setAvatar(ctx context.Context, userID, key string) error {
	err := s.st.SetUserAvatar(ctx, sqlitedb.SetUserAvatarParams{
		ID: userID, Avatar: sql.NullString{String: key, Valid: key != ""}, Now: store.Now(),
	})
	if err != nil {
		return err
	}
	s.peopleChanged(ctx, userID)
	return nil
}

// peopleChanged tells everyone who shares a workspace with a user, the
// user included, that their name or picture changed. It is best effort:
// a failure is logged, and the next reconnect catches up.
func (s *Service) peopleChanged(ctx context.Context, userID string) {
	ids, err := s.st.ListCoMemberIDs(ctx, userID)
	if err != nil {
		s.log.Warn("listing the people to tell of a change", "user", userID, "err", err)
		return
	}
	if len(ids) == 0 {
		ids = []string{userID}
	}
	s.events.PeopleChanged(ids...)
}

// AvatarVisible is the one rule for who may see a picture: its owner,
// anyone who shares a live workspace with them (guests included), and
// the instance's admins. A key no live account names is seen by nobody.
func (s *Service) AvatarVisible(ctx context.Context, viewer store.User, key string) (bool, error) {
	owner, err := s.st.GetAvatarOwner(ctx, sql.NullString{String: key, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner == viewer.ID {
		return true, nil
	}
	if authz.CanInstance(viewer.IsInstanceAdmin && !viewer.DeletedAt.Valid, authz.ViewInstance) {
		return true, nil
	}
	n, err := s.st.CountSharedWorkspaces(ctx, sqlitedb.CountSharedWorkspacesParams{OwnerID: owner, ViewerID: viewer.ID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// RemoveAvatarAsAdmin takes down someone's picture, for an instance admin
// who finds it unsuitable.
func (s *Service) RemoveAvatarAsAdmin(ctx context.Context, adminID, targetID string) (AdminUser, error) {
	if _, err := s.admin(ctx, s.st, adminID, authz.ManageUsers); err != nil {
		return AdminUser{}, err
	}
	target, err := s.st.GetUser(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && target.DeletedAt.Valid) {
		return AdminUser{}, fail(NotFound, "No such user.")
	}
	if err != nil {
		return AdminUser{}, err
	}
	if target.Avatar.Valid {
		if err := s.setAvatar(ctx, targetID, ""); err != nil {
			return AdminUser{}, err
		}
	}
	users, err := s.users(ctx)
	if err != nil {
		return AdminUser{}, err
	}
	for _, u := range users {
		if u.ID == targetID {
			return u, nil
		}
	}
	return AdminUser{}, fail(NotFound, "No such user.")
}
