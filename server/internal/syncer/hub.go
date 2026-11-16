package syncer

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/locodarwin/mila/server/internal/store"
)

// Hub holds every open WebSocket and tells clients when to pull. Messages
// carry no data, only a nudge, so a missed one costs a moment's delay and
// nothing else: clients also pull when they reconnect.
//
//	{"type":"poke","workspace":"…","seq":42}  changes up to 42 exist
//	{"type":"workspaces"}                     your memberships changed
//	{"type":"notify"}                         you have a new notification
//	{"type":"settings"}                       your preferences changed
//	{"type":"people"}                         someone you share a workspace
//	                                          with changed their name or picture
//	{"type":"reset","workspace":"…"}          your access changed: pull, and
//	                                          load the workspace afresh on 410
//
// Devices speak too, about presence only (presence.go): who has a live
// document open, and where their cursor is.
type Hub struct {
	st  *store.Store
	log *slog.Logger

	mu    sync.Mutex
	conns map[*client]struct{}
	docs  DocAccess
	rooms map[string]*room
}

type client struct {
	userID    string
	sessionID string
	send      chan []byte
	cancel    context.CancelFunc
	// rooms are the documents the device has open, by workspace/doc;
	// guarded by Hub.mu.
	rooms map[string]time.Time
	// window and sent count its messages this second (presenceRate).
	window time.Time
	sent   int
}

// maxSocketsPerUser is how many sync sockets one person may hold open.
const maxSocketsPerUser = 20

// NewHub makes an empty hub.
func NewHub(st *store.Store, log *slog.Logger) *Hub {
	return &Hub{st: st, log: log, conns: map[*client]struct{}{}, rooms: map[string]*room{}}
}

type message struct {
	Type      string `json:"type"`
	Workspace string `json:"workspace,omitempty"`
	Seq       int64  `json:"seq,omitempty"`
	// ID names the one thing that changed, for messages about one (a theme).
	ID string `json:"id,omitempty"`
}

// Changed pokes every connected member of the workspace.
func (h *Hub) Changed(workspaceID string, seq int64) {
	members, err := h.st.ListMemberIDs(context.Background(), workspaceID)
	if err != nil {
		h.log.Warn("poke: list members", "workspace", workspaceID, "err", err)
		return
	}
	want := make(map[string]bool, len(members))
	for _, id := range members {
		want[id] = true
	}
	h.broadcast(func(c *client) bool { return want[c.userID] }, message{Type: "poke", Workspace: workspaceID, Seq: seq})
}

// MembershipChanged tells users to reload their workspace list.
func (h *Hub) MembershipChanged(userIDs ...string) {
	want := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		want[id] = true
	}
	h.broadcast(func(c *client) bool { return want[c.userID] }, message{Type: "workspaces"})
}

// PeopleChanged tells users that someone they share a workspace with
// changed their name or picture, so they load people afresh.
func (h *Hub) PeopleChanged(userIDs ...string) {
	want := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		want[id] = true
	}
	h.broadcast(func(c *client) bool { return want[c.userID] }, message{Type: "people"})
}

// Notify tells users they have new notifications.
func (h *Hub) Notify(userIDs ...string) {
	want := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		want[id] = true
	}
	h.broadcast(func(c *client) bool { return want[c.userID] }, message{Type: "notify"})
}

// SettingsChanged tells a user's devices to reload their preferences.
func (h *Hub) SettingsChanged(userID string) {
	h.broadcast(func(c *client) bool { return c.userID == userID }, message{Type: "settings"})
}

// ThemesChanged tells a user's devices that one of their themes was
// saved or deleted, so they load that one afresh.
func (h *Hub) ThemesChanged(userID, themeID string) {
	h.broadcast(func(c *client) bool { return c.userID == userID }, message{Type: "themes", ID: themeID})
}

// AccessChanged tells a member their access to a workspace's projects
// changed. Their next pull answers 410 whether or not this arrives (the
// membership's reset_seq), so the message only makes it happen now.
func (h *Hub) AccessChanged(workspaceID, userID string) {
	// Their open documents in the workspace are left; the device opens
	// them again, and is let in again where it still may be.
	h.mu.Lock()
	for c := range h.conns {
		if c.userID == userID {
			h.leaveAll(c, workspaceID)
		}
	}
	h.mu.Unlock()
	h.broadcast(func(c *client) bool { return c.userID == userID }, message{Type: "reset", Workspace: workspaceID})
}

// SessionEnded closes the sockets of a signed-out or revoked session.
func (h *Hub) SessionEnded(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		if c.sessionID == sessionID {
			c.cancel()
		}
	}
}

func (h *Hub) broadcast(match func(*client) bool, m message) {
	raw, _ := json.Marshal(m)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		if !match(c) {
			continue
		}
		select {
		case c.send <- raw:
		default:
			// A client too slow to take a nudge gets disconnected; it
			// pulls everything when it reconnects.
			c.cancel()
		}
	}
}

// Serve upgrades the request to a WebSocket for a signed-in user and keeps
// it until either side closes. originHosts are the hosts browsers may
// connect from, beyond the server's own.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, userID, sessionID string, originHosts []string) {
	// Every open tab and device holds one; more than this is no person's.
	// The place is taken as it is counted, so upgrades that arrive
	// together cannot all pass.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	c := &client{userID: userID, sessionID: sessionID, send: make(chan []byte, 64), cancel: cancel, rooms: map[string]time.Time{}}
	h.mu.Lock()
	open := 0
	for o := range h.conns {
		if o.userID == userID {
			open++
		}
	}
	if open < maxSocketsPerUser {
		h.conns[c] = struct{}{}
	}
	h.mu.Unlock()
	if open >= maxSocketsPerUser {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originHosts})
	if err != nil {
		h.mu.Lock()
		delete(h.conns, c)
		h.mu.Unlock()
		return // Accept has already answered
	}
	// What devices say is presence; reading is also how a close is noticed.
	ws.SetReadLimit(maxPresenceMessage)
	reading := make(chan struct{})
	defer func() {
		_ = ws.CloseNow()
		// The reader stops first, so it cannot open a room after this.
		<-reading
		h.mu.Lock()
		delete(h.conns, c)
		h.leaveAll(c, "")
		h.mu.Unlock()
	}()
	go func() {
		defer close(reading)
		defer cancel()
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageText {
				h.receive(ctx, c, data)
			}
		}
	}()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	hello, _ := json.Marshal(message{Type: "hello"})
	c.send <- hello
	for {
		select {
		case <-ctx.Done():
			_ = ws.Close(websocket.StatusNormalClosure, "")
			return
		case raw := <-c.send:
			wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
			err := ws.Write(wctx, websocket.MessageText, raw)
			wcancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			err := ws.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}
