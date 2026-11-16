package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strings"

	"github.com/locodarwin/mila/server/internal/authz"
	mailer "github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	maxEmailAddresses = 10
	maxEmailSenders   = 10
)

// EmailAddressInput is what a person chooses about an address.
type EmailAddressInput struct {
	ProjectID  string
	AssignToMe bool
	Senders    []string
}

// newInboundToken is 26 lowercase letters and digits, about 130 bits: lowercase
// because mail systems often change the case of an address.
func newInboundToken() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	b := make([]byte, 26)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[b[i]%32]
	}
	return string(b)
}

// checkEmailProject holds an address to somewhere it can make tasks.
func checkEmailProject(ctx context.Context, q store.Queries, workspaceID, projectID string) (sql.NullString, error) {
	if projectID == "" {
		return sql.NullString{}, nil
	}
	n, err := q.CountLiveProject(ctx, sqlitedb.CountLiveProjectParams{ID: projectID, WorkspaceID: workspaceID})
	if err != nil {
		return sql.NullString{}, err
	}
	if n == 0 {
		return sql.NullString{}, fail(Invalid, "That project is no longer here.")
	}
	return sql.NullString{String: projectID, Valid: true}, nil
}

// cleanSenders is a list of senders as stored: lowercase, bare addresses, each
// once, leaving out the person's own, which is always accepted.
func cleanSenders(list []string, own string) (string, error) {
	out := []string{}
	for _, raw := range list {
		parsed, err := mail.ParseAddress(strings.TrimSpace(raw))
		if err != nil || len(parsed.Address) > 254 {
			return "", fail(Invalid, "%q is not an email address.", strings.TrimSpace(raw))
		}
		address := strings.ToLower(parsed.Address)
		if !strings.EqualFold(address, own) && !slices.Contains(out, address) {
			out = append(out, address)
		}
	}
	if len(out) > maxEmailSenders {
		return "", fail(Invalid, "An address accepts mail from 10 others at most.")
	}
	raw, err := json.Marshal(out)
	return string(raw), err
}

// EmailAddresses lists a person's addresses in a workspace and the mail lately.
func (s *Service) EmailAddresses(ctx context.Context, userID, workspaceID string) ([]sqlitedb.InboundAddress, []sqlitedb.InboundMail, string, error) {
	if _, _, err := s.access(ctx, s.st, userID, workspaceID, authz.ReceiveEmail); err != nil {
		return nil, nil, "", err
	}
	user, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return nil, nil, "", err
	}
	addresses, err := s.st.ListInboundAddresses(ctx, sqlitedb.ListInboundAddressesParams{UserID: userID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, nil, "", err
	}
	made, err := s.st.ListInboundMailMade(ctx, sqlitedb.ListInboundMailMadeParams{UserID: userID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, nil, "", err
	}
	refused, err := s.st.ListInboundMailRefused(ctx, sqlitedb.ListInboundMailRefusedParams{UserID: userID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, nil, "", err
	}
	recent := append(made, refused...)
	slices.SortFunc(recent, func(a, b sqlitedb.InboundMail) int {
		if c := b.ReceivedAt.Compare(a.ReceivedAt); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	return addresses, recent, user.Email, nil
}

// EmailSendersAdded says whether a new list of senders adds any to an address,
// which lets more people add tasks as its owner, so it asks for a recent sign-in.
func (s *Service) EmailSendersAdded(ctx context.Context, userID, workspaceID, id string, senders []string) (bool, error) {
	a, err := s.ownEmailAddress(ctx, s.st, userID, workspaceID, id)
	if err != nil {
		return false, err
	}
	user, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return false, err
	}
	cleaned, err := cleanSenders(senders, user.Email)
	if err != nil {
		return false, err
	}
	return sendersGrew(a.Senders, cleaned), nil
}

func sendersGrew(before, after string) bool {
	var was, now []string
	_ = json.Unmarshal([]byte(before), &was)
	_ = json.Unmarshal([]byte(after), &now)
	return slices.ContainsFunc(now, func(x string) bool { return !slices.Contains(was, x) })
}

// CreateEmailAddress makes an address, says so in the security log and tells
// the person, since it can add tasks as them.
func (s *Service) CreateEmailAddress(ctx context.Context, userID, workspaceID string, in EmailAddressInput, c Client) (sqlitedb.InboundAddress, error) {
	now := store.Now()
	made := sqlitedb.InboundAddress{ID: newID(), UserID: userID, WorkspaceID: workspaceID, Token: newInboundToken(), AssignSelf: in.AssignToMe, Senders: "[]", CreatedAt: now}
	var user store.User
	var name string
	err := s.st.InTx(ctx, func(q store.Queries) error {
		ws, _, err := s.access(ctx, q, userID, workspaceID, authz.ReceiveEmail)
		if err != nil {
			return err
		}
		name = ws.Name
		if user, err = q.GetUser(ctx, userID); err != nil {
			return err
		}
		n, err := q.CountInboundAddresses(ctx, sqlitedb.CountInboundAddressesParams{UserID: userID, WorkspaceID: workspaceID})
		if err != nil {
			return err
		}
		if n >= maxEmailAddresses {
			return fail(Invalid, "You can have 10 addresses in a workspace at most. Delete one you no longer use.")
		}
		if made.ProjectID, err = checkEmailProject(ctx, q, workspaceID, in.ProjectID); err != nil {
			return err
		}
		if err := q.InsertInboundAddress(ctx, sqlitedb.InsertInboundAddressParams{
			ID: made.ID, UserID: userID, WorkspaceID: workspaceID, Token: made.Token, ProjectID: made.ProjectID,
			AssignSelf: made.AssignSelf, Senders: made.Senders, Now: now,
		}); err != nil {
			return err
		}
		_, err = record(ctx, q, userID, EventEmailAddressMade, "", name, c, false, now)
		return err
	})
	if err != nil {
		return made, err
	}
	s.tellOfEmailAddress(ctx, user, workspaceID, name)
	return made, nil
}

func (s *Service) tellOfEmailAddress(ctx context.Context, user store.User, workspaceID, workspace string) {
	if err := s.mail.Send(ctx, mailer.Message{
		To:      user.Email,
		Subject: "A new address for making tasks by email",
		Text: "Hi " + user.Name + ",\n\nAn address that turns email into tasks was just made for your account in " + workspace +
			". If this wasn't you, sign in and delete it, and check your recent activity:\n" + s.cfg.BaseURL + "/settings/w/" + workspaceID + "/email\n",
	}); err != nil {
		// The address is made, and is in the security log; that is enough.
		s.log.Warn("email address notice not sent", "err", err)
	}
}

func (s *Service) ownEmailAddress(ctx context.Context, q store.Queries, userID, workspaceID, id string) (sqlitedb.InboundAddress, error) {
	if _, _, err := s.access(ctx, q, userID, workspaceID, authz.ReceiveEmail); err != nil {
		return sqlitedb.InboundAddress{}, err
	}
	a, err := q.GetInboundAddress(ctx, sqlitedb.GetInboundAddressParams{ID: id, UserID: userID, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return a, fail(NotFound, "No such address.")
	}
	return a, err
}

// UpdateEmailAddress changes where an address goes and whom it accepts.
func (s *Service) UpdateEmailAddress(ctx context.Context, userID, workspaceID, id string, in EmailAddressInput, c Client) (sqlitedb.InboundAddress, error) {
	var out sqlitedb.InboundAddress
	err := s.st.InTx(ctx, func(q store.Queries) error {
		a, err := s.ownEmailAddress(ctx, q, userID, workspaceID, id)
		if err != nil {
			return err
		}
		user, err := q.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		senders, err := cleanSenders(in.Senders, user.Email)
		if err != nil {
			return err
		}
		project, err := checkEmailProject(ctx, q, workspaceID, in.ProjectID)
		if err != nil {
			return err
		}
		if err := q.UpdateInboundAddress(ctx, sqlitedb.UpdateInboundAddressParams{ID: id, ProjectID: project, AssignSelf: in.AssignToMe, Senders: senders}); err != nil {
			return err
		}
		if sendersGrew(a.Senders, senders) {
			ws, err := q.GetWorkspace(ctx, workspaceID)
			if err != nil {
				return err
			}
			if _, err := record(ctx, q, userID, EventEmailSenderAdded, "", ws.Name, c, false, store.Now()); err != nil {
				return err
			}
		}
		a.ProjectID, a.AssignSelf, a.Senders = project, in.AssignToMe, senders
		out = a
		return nil
	})
	return out, err
}

// ReplaceEmailAddress gives an address a new token, ending the old one at once.
func (s *Service) ReplaceEmailAddress(ctx context.Context, userID, workspaceID, id string, c Client) (sqlitedb.InboundAddress, error) {
	var out sqlitedb.InboundAddress
	var user store.User
	var name string
	err := s.st.InTx(ctx, func(q store.Queries) error {
		a, err := s.ownEmailAddress(ctx, q, userID, workspaceID, id)
		if err != nil {
			return err
		}
		ws, err := q.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		name = ws.Name
		if user, err = q.GetUser(ctx, userID); err != nil {
			return err
		}
		a.Token = newInboundToken()
		if err := q.SetInboundAddressToken(ctx, sqlitedb.SetInboundAddressTokenParams{ID: id, Token: a.Token}); err != nil {
			return err
		}
		out = a
		_, err = record(ctx, q, userID, EventEmailAddressMade, "", name, c, false, store.Now())
		return err
	})
	if err != nil {
		return out, err
	}
	s.tellOfEmailAddress(ctx, user, workspaceID, name)
	return out, nil
}

// DeleteEmailAddress ends an address, at once.
func (s *Service) DeleteEmailAddress(ctx context.Context, userID, workspaceID, id string, c Client) error {
	return s.st.InTx(ctx, func(q store.Queries) error {
		if _, err := s.ownEmailAddress(ctx, q, userID, workspaceID, id); err != nil {
			return err
		}
		ws, err := q.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if err := q.DeleteInboundAddress(ctx, id); err != nil {
			return err
		}
		_, err = record(ctx, q, userID, EventEmailAddressRemoved, "", ws.Name, c, false, store.Now())
		return err
	})
}
