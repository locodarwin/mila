package syncer_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
)

// A rename reaches only the documents whose readers all see what was
// renamed, so a hidden task's new title never shows in a guest's page; and
// it is the server's own edit, so a guest's rename never fails on, or
// tells them of, a document they cannot see.
func TestRenamingTellsNoOneWhatTheyCannotSee(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		now := time.Now()
		shared, _ := w.project("Shared")
		hidden, _ := w.project("Hidden")
		w.share(shared, true)
		secret, open := uuid.NewString(), uuid.NewString()
		inShared, inHidden := uuid.NewString(), uuid.NewString()
		w.push("owner",
			op("task", secret, hlcAt(now, "a"), newTask(hidden, nil, "Layoffs")),
			op("task", open, hlcAt(now, "a"), newTask(shared, nil, "Paint")),
			op("task", inShared, hlcAt(now, "a"), newTask(shared, nil, "Notes the guest reads")),
			op("task", inHidden, hlcAt(now, "a"), newTask(hidden, nil, "Notes the guest cannot read")),
		)
		w.push("owner",
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(inShared, edit(t, 80, "See [Layoffs](/?task="+secret+") and [Paint](/?task="+open+")."))),
			op("doc_update", uuid.NewString(), hlcAt(now, "a"), docRow(inHidden, edit(t, 81, "After [Paint](/?task="+open+")."))),
		)
		later := now.Add(time.Second)
		wantStatus(t, w.push("owner", op("task", secret, hlcAt(later, "a"), map[string]any{"title": "Layoffs in March"}))[0], "applied")
		if got := w.taskFields(inShared)["notes"]; got != "See [Layoffs](/?task="+secret+") and [Paint](/?task="+open+")." {
			t.Errorf("the guest's notes read %q", got)
		}

		// The guest renames a task a hidden document links to.
		wantStatus(t, w.push("guest", op("task", open, hlcAt(later.Add(time.Second), "g"), map[string]any{"title": "Paint the hall"}))[0], "applied")
		if got := w.taskFields(inHidden)["notes"]; got != "After [Paint the hall](/?task="+open+")." {
			t.Errorf("the hidden notes read %q", got)
		}
		if got := w.taskFields(inShared)["notes"]; got != "See [Layoffs](/?task="+secret+") and [Paint the hall](/?task="+open+")." {
			t.Errorf("the guest's notes read %q", got)
		}
	})
}
