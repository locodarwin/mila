package syncer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

type allow map[string]bool

func (a allow) SeesDoc(_ context.Context, userID, _, _ string) (bool, error) { return a[userID], nil }

const doc = "task:0193a000-0000-7000-8000-000000000001"

func say(t *testing.T, h *Hub, c *client, entries ...awarenessEntry) {
	t.Helper()
	raw, _ := json.Marshal(presenceMessage{Type: "presence", Workspace: "w", Doc: doc, Update: base64.StdEncoding.EncodeToString(encodeAwareness(entries))})
	h.receive(context.Background(), c, raw)
}

// heard is what a device has been sent, as awareness entries.
func heard(t *testing.T, c *client) []awarenessEntry {
	t.Helper()
	var out []awarenessEntry
	for {
		select {
		case raw := <-c.send:
			var m presenceMessage
			if err := json.Unmarshal(raw, &m); err != nil || m.Type != "presence" {
				t.Fatalf("not presence: %s", raw)
			}
			b, _ := base64.StdEncoding.DecodeString(m.Update)
			entries, err := decodeAwareness(b)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, entries...)
		default:
			return out
		}
	}
}

func device(userID string) *client {
	return &client{userID: userID, send: make(chan []byte, 64), cancel: func() {}, rooms: map[string]time.Time{}}
}

func TestPresenceIsPassedOnStampedToThoseWhoMaySee(t *testing.T) {
	h := NewHub(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetDocs(allow{"ann": true, "bo": true})
	ann, bo, cy := device("ann"), device("bo"), device("cy")

	// Ann opens the notes, claiming to be someone else: the server says who.
	say(t, h, ann, awarenessEntry{client: 1, clock: 1, state: `{"cursor":{"anchor":3},"user":{"name":"The Boss"}}`})
	if got := heard(t, ann); len(got) != 0 {
		t.Fatalf("ann heard %v alone", got)
	}
	// Bo opens them and hears Ann; Ann hears Bo.
	say(t, h, bo, awarenessEntry{client: 2, clock: 1, state: `{"cursor":null}`})
	got := heard(t, bo)
	if len(got) != 1 || got[0].client != 1 || got[0].state != `{"cursor":{"anchor":3},"user":{"id":"ann"}}` {
		t.Fatalf("bo heard %+v", got)
	}
	if got := heard(t, ann); len(got) != 1 || got[0].state != `{"cursor":null,"user":{"id":"bo"}}` {
		t.Fatalf("ann heard %+v", got)
	}

	// Cy may not see the task: Cy is not let in, and nobody hears Cy.
	say(t, h, cy, awarenessEntry{client: 3, clock: 1, state: `{}`})
	if len(heard(t, cy)) != 0 || len(heard(t, ann)) != 0 || len(heard(t, bo)) != 0 {
		t.Fatal("cy got through")
	}

	// Bo cannot move Ann's cursor, or take her away.
	say(t, h, bo, awarenessEntry{client: 1, clock: 9, state: `{"cursor":{"anchor":0}}`})
	say(t, h, bo, awarenessEntry{client: 1, clock: 10, state: "null"})
	if got := heard(t, ann); len(got) != 0 {
		t.Fatalf("bo's claim on ann's client reached her: %+v", got)
	}

	// Ann's cursor moves; an older state is ignored.
	say(t, h, ann, awarenessEntry{client: 1, clock: 2, state: `{"cursor":{"anchor":5}}`})
	say(t, h, ann, awarenessEntry{client: 1, clock: 1, state: `{"cursor":{"anchor":1}}`})
	if got := heard(t, bo); len(got) != 1 || got[0].clock != 2 {
		t.Fatalf("bo heard %+v", got)
	}

	// Ann closes the notes: Bo hears she is gone.
	raw, _ := json.Marshal(presenceMessage{Type: "leave", Workspace: "w", Doc: doc})
	h.receive(context.Background(), ann, raw)
	if got := heard(t, bo); len(got) != 1 || got[0].client != 1 || got[0].state != "null" || got[0].clock != 3 {
		t.Fatalf("bo heard %+v", got)
	}

	// Bo's socket closes: the room goes.
	h.mu.Lock()
	h.leaveAll(bo, "")
	rooms := len(h.rooms)
	h.mu.Unlock()
	if rooms != 0 {
		t.Fatalf("%d rooms left", rooms)
	}
}

func TestAwarenessUpdatesReadBackAndBadOnesAreRefused(t *testing.T) {
	in := []awarenessEntry{{client: 1 << 40, clock: 300, state: `{"a":"é"}`}, {client: 7, clock: 1, state: "null"}}
	out, err := decodeAwareness(encodeAwareness(in))
	if err != nil || len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("read back %+v, %v", out, err)
	}
	for _, bad := range [][]byte{nil, {0x01}, {0x01, 0x01, 0x01, 0x05, 'a'}, {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}} {
		if _, err := decodeAwareness(bad); err == nil {
			t.Errorf("%v was read", bad)
		}
	}
	if _, ok := stampState(`[1]`, "u"); ok {
		t.Error("a list was taken as a state")
	}
}

// One window has a cursor or two in a document, not thousands: a device
// that claims ever more Yjs clients in a room keeps a few, and a room holds
// a bounded number of states, so neither memory nor what each newcomer is
// sent grows without end.
func TestPresenceStaysBoundedPerRoom(t *testing.T) {
	h := NewHub(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetDocs(allow{"ann": true, "bo": true})
	ann := device("ann")
	var claims []awarenessEntry
	for i := range 64 {
		claims = append(claims, awarenessEntry{client: uint64(100 + i), clock: 1, state: `{}`})
	}
	say(t, h, ann, claims...)
	r := h.rooms["w/"+doc]
	if n := len(r.states); n != maxClientsPerSocket {
		t.Fatalf("one device holds %d states in a room, want %d", n, maxClientsPerSocket)
	}
	// Its own clients still move.
	say(t, h, ann, awarenessEntry{client: 100, clock: 2, state: `{"cursor":{"anchor":1}}`})
	if got := r.states[100].clock; got != 2 {
		t.Errorf("a device's own client did not move: clock %d", got)
	}

	// However many devices, the room stops at its limit.
	for i := range maxStatesPerRoom {
		say(t, h, device("bo"), awarenessEntry{client: uint64(1000 + i), clock: 1, state: `{}`})
	}
	if n := len(r.states); n > maxStatesPerRoom {
		t.Errorf("a room holds %d states, over %d", n, maxStatesPerRoom)
	}
}

// Whether a device may see the document is asked again as it goes on
// speaking: one that lost access leaves the room, and hears nobody after.
func TestPresenceAsksAgainWhoMaySee(t *testing.T) {
	h := NewHub(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	may := allow{"ann": true, "bo": true}
	h.SetDocs(may)
	ann, bo := device("ann"), device("bo")
	say(t, h, ann, awarenessEntry{client: 1, clock: 1, state: `{}`})
	say(t, h, bo, awarenessEntry{client: 2, clock: 1, state: `{}`})
	heard(t, ann)
	heard(t, bo)

	may["bo"] = false
	// Within the recheck nothing is asked; after it, Bo is let go.
	say(t, h, bo, awarenessEntry{client: 2, clock: 2, state: `{"cursor":null}`})
	if got := heard(t, ann); len(got) != 1 {
		t.Fatalf("ann heard %+v before the recheck", got)
	}
	h.mu.Lock()
	bo.rooms["w/"+doc] = time.Now().Add(-presenceRecheck)
	h.mu.Unlock()
	say(t, h, bo, awarenessEntry{client: 2, clock: 3, state: `{"cursor":{"anchor":1}}`})
	if got := heard(t, ann); len(got) != 1 || got[0].state != "null" {
		t.Fatalf("ann heard %+v as bo lost the document", got)
	}
	say(t, h, ann, awarenessEntry{client: 1, clock: 2, state: `{"cursor":null}`})
	if got := heard(t, bo); len(got) != 0 {
		t.Fatalf("bo still hears: %+v", got)
	}
}
