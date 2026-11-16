package syncer

// Presence (docs/PLAN.md section 7): who has a live document open, and
// where their cursor is. Devices send Yjs awareness updates over the
// WebSocket; the server passes them to the other devices that have the
// same document open and may see it, and stores nothing.
//
//	{"type":"presence","workspace":"…","doc":"task:…","update":"<base64>"}  either way
//	{"type":"leave","workspace":"…","doc":"task:…"}                         device to server
//
// The server stamps every state it passes on with the sender's user ID
// (state.user = {"id": …}) and drops whatever user the device named, so
// nobody shows up as someone else; a Yjs client ID belongs to the first
// connection that uses it in a room. A device that closes the document or
// goes away is announced gone.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	// maxPresenceMessage bounds a message from a device.
	maxPresenceMessage = 16 << 10
	// maxPresenceState bounds one client's state, cursor and all.
	maxPresenceState = 4 << 10
	// maxRoomsPerSocket is how many documents one window can be in.
	maxRoomsPerSocket = 8
	// presenceRate is how many messages a socket may send a second;
	// more are dropped, as a cursor's next move says where it is anyway.
	presenceRate = 40
	// maxClientsPerSocket is how many Yjs clients one window may have in a
	// room (it has one per open document; a few allow for a reload), and
	// maxStatesPerRoom how many states a room holds in all. A client past
	// either is not taken in, so a device claiming new ones forever cannot
	// fill the memory, or what every newcomer is sent.
	maxClientsPerSocket = 4
	maxStatesPerRoom    = 200
)

// DocAccess says whether someone may see a live document.
type DocAccess interface {
	SeesDoc(ctx context.Context, userID, workspaceID, doc string) (bool, error)
}

// SetDocs gives the hub what it asks before letting someone into a document.
func (h *Hub) SetDocs(d DocAccess) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.docs = d
}

// room is a document's open devices and their states.
type room struct {
	members map[*client]bool
	states  map[uint64]presenceEntry
}

type presenceEntry struct {
	clock uint64
	state string // JSON, or "null" for gone
	owner *client
}

type presenceMessage struct {
	Type      string `json:"type"`
	Workspace string `json:"workspace"`
	Doc       string `json:"doc"`
	Update    string `json:"update,omitempty"`
}

// presenceRecheck is how often a device in a room is asked again whether
// it may see the document.
const presenceRecheck = 30 * time.Second

// receive handles a message from a device.
func (h *Hub) receive(ctx context.Context, c *client, raw []byte) {
	now := time.Now()
	if now.Sub(c.window) >= time.Second {
		c.window, c.sent = now, 0
	}
	if c.sent++; c.sent > presenceRate {
		return
	}
	var m presenceMessage
	if json.Unmarshal(raw, &m) != nil || m.Workspace == "" || !validDoc(m.Doc) {
		return
	}
	key := m.Workspace + "/" + m.Doc
	switch m.Type {
	case "leave":
		h.mu.Lock()
		h.leave(c, key)
		h.mu.Unlock()
	case "presence":
		update, err := base64.StdEncoding.DecodeString(m.Update)
		if err != nil {
			return
		}
		entries, err := decodeAwareness(update)
		if err != nil {
			return
		}
		h.mu.Lock()
		checked, joined := c.rooms[key]
		full := len(c.rooms) >= maxRoomsPerSocket
		docs := h.docs
		h.mu.Unlock()
		// Whether they may see it is asked again now and then: access can
		// end while a document stays open (a project taken away, a task
		// moved out of reach), and devices renew their presence often.
		if !joined || now.Sub(checked) >= presenceRecheck {
			if docs == nil || (!joined && full) {
				return
			}
			ok, err := docs.SeesDoc(ctx, c.userID, m.Workspace, m.Doc)
			if err != nil || !ok {
				if joined && err == nil {
					h.mu.Lock()
					h.leave(c, key)
					h.mu.Unlock()
				}
				return
			}
			checked = now
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		r := h.rooms[key]
		if r == nil {
			r = &room{members: map[*client]bool{}, states: map[uint64]presenceEntry{}}
			h.rooms[key] = r
		}
		first := !r.members[c]
		r.members[c] = true
		c.rooms[key] = checked
		mine := 0
		for _, s := range r.states {
			if s.owner == c {
				mine++
			}
		}
		var passed []awarenessEntry
		for _, e := range entries {
			old, known := r.states[e.client]
			if known && old.owner != c {
				continue // someone else's client
			}
			if !known && e.state != "null" && (mine >= maxClientsPerSocket || len(r.states) >= maxStatesPerRoom) {
				continue // one client too many
			}
			if known && e.clock <= old.clock && e.state != "null" {
				continue // older than what is held
			}
			state, ok := stampState(e.state, c.userID)
			if !ok {
				continue
			}
			e.state = state
			if state == "null" {
				if known {
					mine--
				}
				delete(r.states, e.client)
			} else {
				if !known {
					mine++
				}
				r.states[e.client] = presenceEntry{clock: e.clock, state: state, owner: c}
			}
			passed = append(passed, e)
		}
		if len(passed) > 0 {
			h.toRoom(r, c, m.Workspace, m.Doc, encodeAwareness(passed))
		}
		if first {
			// A device that just opened the document hears who else has it.
			var others []awarenessEntry
			for id, s := range r.states {
				if s.owner != c {
					others = append(others, awarenessEntry{client: id, clock: s.clock, state: s.state})
				}
			}
			if len(others) > 0 {
				h.sendPresence(c, m.Workspace, m.Doc, encodeAwareness(others))
			}
		}
	}
}

// leave takes a device out of a document's room and tells the others its
// clients are gone. h.mu is held.
func (h *Hub) leave(c *client, key string) {
	r := h.rooms[key]
	delete(c.rooms, key)
	if r == nil || !r.members[c] {
		return
	}
	delete(r.members, c)
	var gone []awarenessEntry
	for id, s := range r.states {
		if s.owner == c {
			gone = append(gone, awarenessEntry{client: id, clock: s.clock + 1, state: "null"})
			delete(r.states, id)
		}
	}
	if len(r.members) == 0 {
		delete(h.rooms, key)
		return
	}
	if len(gone) > 0 {
		ws, doc, _ := strings.Cut(key, "/")
		h.toRoom(r, c, ws, doc, encodeAwareness(gone))
	}
}

// leaveAll takes a device out of every room, as its socket closes, or of
// one workspace's rooms, as its access changes. h.mu is held.
func (h *Hub) leaveAll(c *client, workspaceID string) {
	for key := range c.rooms {
		if workspaceID == "" || strings.HasPrefix(key, workspaceID+"/") {
			h.leave(c, key)
		}
	}
}

// toRoom passes an update to every device in a room but the sender. h.mu
// is held.
func (h *Hub) toRoom(r *room, from *client, ws, doc string, update []byte) {
	for m := range r.members {
		if m != from {
			h.sendPresence(m, ws, doc, update)
		}
	}
}

// sendPresence queues a presence message, or drops it when the device is
// behind: unlike a poke it is no loss, the next one says where things are.
func (h *Hub) sendPresence(c *client, ws, doc string, update []byte) {
	raw, _ := json.Marshal(presenceMessage{Type: "presence", Workspace: ws, Doc: doc, Update: base64.StdEncoding.EncodeToString(update)})
	select {
	case c.send <- raw:
	default:
	}
}

// validDoc says whether a document's name is one presence knows: a task's
// notes ("task:<id>") or a page's body ("page:<id>").
func validDoc(doc string) bool {
	kind, id, ok := strings.Cut(doc, ":")
	return ok && (kind == "task" || kind == "page") && idPattern.MatchString(id)
}

// stampState makes a state the sender's: its user is their ID alone.
func stampState(state, userID string) (string, bool) {
	if state == "null" {
		return state, true
	}
	if len(state) > maxPresenceState {
		return "", false
	}
	var obj map[string]any
	if json.Unmarshal([]byte(state), &obj) != nil || obj == nil {
		return "", false
	}
	obj["user"] = map[string]any{"id": userID}
	b, err := json.Marshal(obj)
	return string(b), err == nil
}

// The awareness update format (y-protocols): a count, then each client's
// ID, clock and state as JSON, numbers as lib0 varuints.

type awarenessEntry struct {
	client, clock uint64
	state         string
}

var errAwareness = errors.New("not an awareness update")

func decodeAwareness(b []byte) ([]awarenessEntry, error) {
	pos := 0
	uint := func() (uint64, error) {
		var v uint64
		for shift := 0; shift < 64; shift += 7 {
			if pos >= len(b) {
				return 0, errAwareness
			}
			x := b[pos]
			pos++
			v |= uint64(x&0x7f) << shift
			if x < 0x80 {
				return v, nil
			}
		}
		return 0, errAwareness
	}
	n, err := uint()
	if err != nil || n > 64 {
		return nil, errAwareness
	}
	out := make([]awarenessEntry, 0, n)
	for range n {
		id, err := uint()
		if err != nil {
			return nil, err
		}
		clock, err := uint()
		if err != nil {
			return nil, err
		}
		size, err := uint()
		if err != nil || size > uint64(len(b)-pos) {
			return nil, errAwareness
		}
		out = append(out, awarenessEntry{client: id, clock: clock, state: string(b[pos : pos+int(size)])})
		pos += int(size)
	}
	return out, nil
}

func encodeAwareness(entries []awarenessEntry) []byte {
	var b []byte
	put := func(v uint64) {
		for v >= 0x80 {
			b = append(b, byte(v)|0x80)
			v >>= 7
		}
		b = append(b, byte(v))
	}
	put(uint64(len(entries)))
	for _, e := range entries {
		put(e.client)
		put(e.clock)
		put(uint64(len(e.state)))
		b = append(b, e.state...)
	}
	return b
}

// SeesDoc says whether someone may see a live document: a task's notes or
// a page's body, where they can see the task or the page.
func (s *Service) SeesDoc(ctx context.Context, userID, workspaceID, doc string) (bool, error) {
	if !validDoc(doc) {
		return false, nil
	}
	kind, id, _ := strings.Cut(doc, ":")
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return false, nil //nolint:nilerr // not a member: no
	}
	d := s.st.DB()
	e := entities[kind]
	r, err := loadRow(ctx, d, d.Dialect, e, id)
	if err != nil || !r.exists || r.workspaceID != workspaceID {
		return false, err
	}
	project, err := projectOf(ctx, d, d.Dialect, e, r)
	if err != nil {
		return false, err
	}
	return v.sees(project), nil
}
