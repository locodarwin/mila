package inbound_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/inbound"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/ydoc"
)

func crlf(s string) []byte { return []byte(strings.ReplaceAll(s, "\n", "\r\n")) }

// nested is an email with parts inside parts, each with a boundary of its own.
func nested(from, to string, levels int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\nTo: %s\nSubject: Deep\nMessage-ID: <%s@example.org>\nContent-Type: multipart/mixed; boundary=b0\n\n", from, to, uuid.NewString())
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&b, "--b%d\nContent-Type: multipart/mixed; boundary=b%d\n\n", i-1, i)
	}
	b.WriteString("--b" + fmt.Sprint(levels) + "\nContent-Type: text/plain\n\nThe bottom.\n")
	return crlf(b.String())
}

func TestAStrangersMailIsNeverRead(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		r := w.readerOf(t, "file:///nowhere", false)
		ours := "mila+" + token + "@example.com"

		// However it is built: fast, because only its headers are looked at.
		for name, raw := range map[string][]byte{
			"addressed to nobody here":  nested("stranger@example.net", "friend@example.org", 200_000),
			"addressed to someone here": nested("stranger@example.net", ours, 200_000),
		} {
			started := time.Now()
			if _, err := r.DeliverRaw(w.ctx, raw); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if took := time.Since(started); took > 2*time.Second {
				t.Fatalf("%s took %v", name, took)
			}
		}
		wantOutcomes(t, w.outcomes(), inbound.RefusedSender)
		if len(w.tasks()) != 0 {
			t.Fatal("made a task")
		}
	})
}

func TestMailBuiltToBeCostlyIsRefusedEvenFromAListedSender(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		r := w.readerOf(t, "file:///nowhere", false)
		ours := "mila+" + token + "@example.com"
		many := func(line string, n int) []byte {
			head := fmt.Sprintf("From: %s\nTo: %s\nSubject: Costly\nMessage-ID: <%s@example.org>\nContent-Type: multipart/mixed; boundary=x\n\n", w.email, ours, uuid.NewString())
			return crlf(head + strings.Repeat(line, n))
		}
		for name, raw := range map[string][]byte{
			"nesting":       nested(w.email, ours, inbound.MaxBoundaries+1),
			"parts":         many("--x\nContent-Type: text/plain\n\na\n", 6000),
			"header fields": many("a:\n", 3<<20),
		} {
			started := time.Now()
			if _, err := r.DeliverRaw(w.ctx, raw); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if took := time.Since(started); took > 5*time.Second {
				t.Fatalf("%s took %v", name, took)
			}
			if got := w.outcomes()[0]; got != inbound.RefusedTooLarge {
				t.Fatalf("%s: %s", name, got)
			}
		}
		if len(w.tasks()) != 0 {
			t.Fatal("made a task")
		}
		// An ordinary message nested a little is fine.
		if _, err := r.DeliverRaw(w.ctx, nested(w.email, ours, 3)); err != nil {
			t.Fatal(err)
		}
		if len(w.tasks()) != 1 {
			t.Fatal("no task from ordinary mail")
		}
	})
}

func TestAMessageTooLargeIsListedFromItsHeaders(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		r := w.readerOf(t, "file:///nowhere", false)
		raw := fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nSubject: Huge\nMessage-ID: <h@example.org>\n\n", w.email, token)
		raw += strings.Repeat("x", inbound.MaxMessage+10)
		_, err := r.DeliverRaw(w.ctx, []byte(raw))
		must(t, err)
		wantOutcomes(t, w.outcomes(), inbound.RefusedTooLarge)
	})
}

func TestAtMostSoManyFilesOfOneMessageAreKept(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		var b strings.Builder
		fmt.Fprintf(&b, "From: %s\nTo: mila+%s@example.com\nSubject: Files\nMessage-ID: <f@example.org>\nContent-Type: multipart/mixed; boundary=x\n\n--x\nContent-Type: text/plain\n\nHello.\n", w.email, token)
		for i := range inbound.MaxFiles + 3 {
			fmt.Fprintf(&b, "--x\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=f%d.pdf\nContent-Transfer-Encoding: base64\n\nJVBERi0=\n", i)
		}
		b.WriteString("--x--\n")
		_, err := w.readerOf(t, "file:///nowhere", false).DeliverRaw(w.ctx, crlf(b.String()))
		must(t, err)
		if n := len(w.rows("attachment")); n != inbound.MaxFiles {
			t.Fatalf("%d files", n)
		}
		notes, _ := w.tasks()[0].Fields["notes"].(string)
		if !strings.Contains(notes, "3 more") {
			t.Fatalf("notes %q", notes)
		}
	})
}

func TestADailyLimitOnSize(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		id, token := w.address("", false)
		must(t, w.st.InsertInboundMail(w.ctx, sqlitedb.InsertInboundMailParams{
			ID: uuid.NewString(), UserID: w.user, WorkspaceID: w.ws, AddressID: id, FromAddress: w.email,
			Subject: "Large", Outcome: inbound.Made, Size: inbound.MaxBytesPerDay, Now: store.Now(),
		}))
		tick()
		raw := fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nSubject: One more\nMessage-ID: <s@example.org>\n\nHello\n", w.email, token)
		_, err := w.readerOf(t, "file:///nowhere", false).DeliverRaw(w.ctx, crlf(raw))
		must(t, err)
		if len(w.tasks()) != 0 {
			t.Fatal("made a task past the limit")
		}
		if got := w.outcomes(); got[0] != inbound.RefusedLimit {
			t.Fatalf("%v", got)
		}
	})
}

// Cut off after the task was made, a delivery is finished by reading it again:
// its files kept once each and its line in the list written.
func TestADeliveryCutShortIsFinished(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		raw := fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nSubject: With a file\nMessage-ID: <cut@example.org>\nContent-Type: multipart/mixed; boundary=x\n\n--x\nContent-Type: text/plain\n\nHello.\n--x\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=a.pdf\nContent-Transfer-Encoding: base64\n\nJVBERi0=\n--x--\n", w.email, token)
		parse := func() *inbound.Message {
			m, err := inbound.Parse(strings.NewReader(string(crlf(raw))))
			must(t, err)
			return m
		}
		_, err := w.svc.Deliver(w.ctx, parse())
		must(t, err)
		// As if it stopped before its line in the list.
		must(t, w.st.DeleteUserInboundMail(w.ctx, w.user))

		_, err = w.svc.Deliver(w.ctx, parse())
		must(t, err)
		if n := len(w.tasks()); n != 1 {
			t.Fatalf("%d tasks", n)
		}
		if n := len(w.rows("attachment")); n != 1 {
			t.Fatalf("%d files", n)
		}
		wantOutcomes(t, w.outcomes(), inbound.Made)
		// A third time, finished, it does nothing.
		_, err = w.svc.Deliver(w.ctx, parse())
		must(t, err)
		wantOutcomes(t, w.outcomes(), inbound.Made)
	})
}

// A stopped server or a busy database must mean trying again, not losing the mail.
func TestAnErrorIsNotTakenForARefusal(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", true)
		m, err := inbound.Parse(strings.NewReader(string(crlf(fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nSubject: Hi\nMessage-ID: <e@example.org>\n\nHello\n", w.email, token)))))
		must(t, err)
		ctx, cancel := contextCanceled(w.ctx)
		defer cancel()
		named, err := w.svc.Deliver(ctx, m)
		if err == nil {
			t.Fatalf("no error, named %v", named)
		}
		if len(w.outcomes()) != 0 || len(w.tasks()) != 0 {
			t.Fatal("recorded something")
		}
	})
}

func TestRefusalsDoNotPushTasksOffTheList(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		_, err := w.send(token, w.email, "Subject: A task\n", "Hello.\n")
		must(t, err)
		for i := range 60 {
			_, err := w.send(token, "stranger@example.net", fmt.Sprintf("Subject: Junk %d\n", i), "Hello.\n")
			must(t, err)
		}
		// Refusals stop being listed at the day's number, and the task stays.
		got := w.outcomes()
		if len(got) != inbound.MaxRefusedDay+1 {
			t.Fatalf("%d listed", len(got))
		}
	})
}

func TestAMessagesWordsNeverBecomeFormatting(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		body := "  # Not a heading\n\n    not code\n\n   ```\nnot a fence\n\n- not a list\n\n> not a quote\n\n1. not numbered\n\n<script>alert(1)</script>\n\n![x](https://example.net/track.png)\n\n[click](https://example.net)\n\n[[Secret page]]\n\na | b\n--- | ---\n1 | 2\n\n---\n\n***\n"
		_, err := w.send(token, w.email, "Subject: Marks\n", body)
		must(t, err)
		notes, _ := w.tasks()[0].Fields["notes"].(string)
		doc := ydoc.Parse(notes)
		for _, n := range doc.Content {
			if n.Type != "paragraph" {
				t.Errorf("a %s from %q", n.Type, notes)
			}
			for _, c := range n.Content {
				if c.Type != "text" && c.Type != "hardBreak" {
					t.Errorf("a %s from %q", c.Type, notes)
				}
				if len(c.Marks) > 0 {
					t.Errorf("marks %v on %q", c.Marks, c.Text)
				}
			}
		}
		// Files are named after what they were, with their contents untouched.
		if strings.Contains(notes, "<script>") {
			t.Errorf("%q", notes)
		}
	})
}

func TestHeadersAreCutOffSafely(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, token := w.address("", false)
		huge := "X-Pad: " + strings.Repeat("a", 900) + "\n"
		raw := fmt.Sprintf("From: %s\nTo: mila+%s@example.com\nSubject: Long headers\nMessage-ID: <l@example.org>\n", w.email, token) + strings.Repeat(huge, 400) + "\nHello\n"
		_, err := w.readerOf(t, "file:///nowhere", false).DeliverRaw(w.ctx, crlf(raw))
		must(t, err)
		// The fields kept are read as usual, whatever was cut off after them.
		if got := w.outcomes(); len(got) != 1 || got[0] != inbound.Made {
			t.Fatalf("%v", got)
		}
	})
}
