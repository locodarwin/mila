package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/httpapi/api"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

func (h *handlers) emailAddressJSON(a sqlitedb.InboundAddress) api.EmailAddress {
	out := api.EmailAddress{
		Id: a.ID, Address: strings.Replace(h.inboundAddress, "{token}", a.Token, 1),
		AssignToMe: a.AssignSelf, Senders: []string{}, CreatedAt: a.CreatedAt,
	}
	_ = json.Unmarshal([]byte(a.Senders), &out.Senders)
	if a.ProjectID.Valid {
		out.ProjectId = &a.ProjectID.String
	}
	return out
}

func (h *handlers) GetEmailToTask(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	addresses, recent, own, err := h.acc.EmailAddresses(r.Context(), a.User.ID, workspaceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := api.EmailToTask{
		Enabled: h.inboundAddress != "", SignInAddress: own,
		Addresses: make([]api.EmailAddress, len(addresses)), Recent: make([]api.EmailReceived, len(recent)),
	}
	for i, x := range addresses {
		out.Addresses[i] = h.emailAddressJSON(x)
	}
	for i, m := range recent {
		out.Recent[i] = api.EmailReceived{
			Id: m.ID, AddressId: m.AddressID, From: m.FromAddress, Subject: m.Subject,
			Outcome: api.EmailReceivedOutcome(m.Outcome), ReceivedAt: m.ReceivedAt,
		}
		if m.TaskID.Valid {
			out.Recent[i].TaskId = &m.TaskID.String
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) CreateEmailAddress(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId) {
	a := h.requireUser(w, r)
	var body api.CreateEmailAddressJSONRequestBody
	if a == nil || !h.requireRecentSignIn(w, r, a) || !h.decode(w, r, &body) {
		return
	}
	if h.inboundAddress == "" {
		writeError(w, http.StatusBadRequest, "invalid", "This server has no mailbox set up to receive email.")
		return
	}
	in := accounts.EmailAddressInput{AssignToMe: body.AssignToMe}
	if body.ProjectId != nil {
		in.ProjectID = *body.ProjectId
	}
	made, err := h.acc.CreateEmailAddress(r.Context(), a.User.ID, workspaceID, in, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.emailAddressJSON(made))
}

func (h *handlers) UpdateEmailAddress(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, addressID string) {
	a := h.requireUser(w, r)
	var body api.UpdateEmailAddressJSONRequestBody
	if a == nil || !h.decode(w, r, &body) {
		return
	}
	in := accounts.EmailAddressInput{AssignToMe: body.AssignToMe, Senders: body.Senders}
	if body.ProjectId != nil {
		in.ProjectID = *body.ProjectId
	}
	// More senders let more people add tasks as this person.
	grows, err := h.acc.EmailSendersAdded(r.Context(), a.User.ID, workspaceID, addressID, body.Senders)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if grows && !h.requireRecentSignIn(w, r, a) {
		return
	}
	out, err := h.acc.UpdateEmailAddress(r.Context(), a.User.ID, workspaceID, addressID, in, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.emailAddressJSON(out))
}

func (h *handlers) DeleteEmailAddress(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, addressID string) {
	a := h.requireUser(w, r)
	if a == nil {
		return
	}
	if err := h.acc.DeleteEmailAddress(r.Context(), a.User.ID, workspaceID, addressID, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) ReplaceEmailAddress(w http.ResponseWriter, r *http.Request, workspaceID api.WorkspaceId, addressID string) {
	a := h.requireUser(w, r)
	if a == nil || !h.requireRecentSignIn(w, r, a) {
		return
	}
	out, err := h.acc.ReplaceEmailAddress(r.Context(), a.User.ID, workspaceID, addressID, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.emailAddressJSON(out))
}
