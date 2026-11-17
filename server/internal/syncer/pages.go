package syncer

// The wiki's pages. Each project has a wiki, and
// the workspace has one, with project_id null, that guests never see. A
// wiki's main page exists from the start, as far as anyone can tell: its
// ID is made from its project's or workspace's, so whichever device first
// writes in it makes the same row any other would. Every other page has a
// parent in the same wiki, and no page is inside its own subpages.

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// mainPageSpace is mixed into a project's or workspace's ID to make its
// wiki's main page's (web/src/lib/sync/pages.ts has the same).
const mainPageSpace = "2f5c6c0e-8a9b-4c7e-9d3a-6b1e4f2a7c58"

// MainPageID is the ID of a wiki's main page: the project's, or for the
// workspace's wiki the workspace's, mixed with mainPageSpace.
func MainPageID(projectOrWorkspace string) string {
	return ids.XOR(projectOrWorkspace, mainPageSpace)
}

// maxPageDepth bounds how deep subpages go, and how far a check walks up.
const maxPageDepth = 64

// checkPage holds a page to the wiki's shape.
func (s *Service) checkPage(ctx context.Context, tx *sql.Tx, r *row, accepted []string, workspaceID string) error {
	d := s.st.DB()
	scope := workspaceID
	if p, ok := r.values["project_id"].(string); ok {
		scope = p
	}
	main := r.id == MainPageID(scope)
	parent, _ := r.values["parent_id"].(string)
	switch {
	case main && parent != "":
		return refuse(apperr.Invalid, "parent_id", "A wiki's main page has no parent.")
	case main && r.values["deleted_at"] != nil:
		return refuse(apperr.Invalid, "deleted_at", "A wiki's main page cannot be deleted; it goes with its project.")
	case !main && parent == "":
		return refuse(apperr.Invalid, "parent_id", "A page needs a parent page in its wiki; only the main page has none.")
	}
	if parent == "" || (!slices.Contains(accepted, "parent_id") && r.exists) {
		return nil
	}
	// The parent is in the same wiki, and is not this page or below it.
	at := parent
	for range maxPageDepth {
		if at == r.id {
			return refuse(apperr.Invalid, "parent_id", "A page cannot go inside its own subpages.")
		}
		p, err := loadRow(ctx, tx, d.Dialect, entities["page"], at)
		if err != nil {
			return err
		}
		if !p.exists || p.workspaceID != workspaceID {
			return refuse(apperr.Invalid, "parent_id", "parent_id points at a page that does not exist.")
		}
		if at == parent && p.values["project_id"] != r.values["project_id"] {
			return refuse(apperr.Invalid, "parent_id", "A page's parent must be in the same wiki.")
		}
		next, _ := p.values["parent_id"].(string)
		if next == "" {
			return nil
		}
		at = next
	}
	return refuse(apperr.Invalid, "parent_id", "Pages go at most %d deep.", maxPageDepth)
}

// A page's history: its body after each editing session, its newest
// maxVersions, and of those no more than maxHistory bytes (the newest is
// always kept). Typing in the page (through its document) joins the
// person's current session, while they keep at it with gaps of no more
// than sessionGap; a body written whole (a restore, a REST write) is a
// version of its own, so the page as it was before a restore stays.

// recordVersion keeps a page's new body in its history. The first time,
// the body it had before (made before its history began, or imported)
// goes in first.
func recordVersion(ctx context.Context, q store.Queries, workspaceID, pageID, actor, before, body string, typed bool, now time.Time) error {
	last, err := q.LatestPageVersion(ctx, pageID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if before != "" {
			if err := q.InsertPageVersion(ctx, sqlitedb.InsertPageVersionParams{
				ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: workspaceID, PageID: pageID,
				Body: before, Now: now.Add(-time.Microsecond),
			}); err != nil {
				return err
			}
		}
	case err != nil:
		return err
	case typed && last.ActorID.String == actor && now.Sub(last.UpdatedAt) <= sessionGap:
		return q.UpdatePageVersion(ctx, sqlitedb.UpdatePageVersionParams{ID: last.ID, Body: body, Now: now})
	}
	if err := q.InsertPageVersion(ctx, sqlitedb.InsertPageVersionParams{
		ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: workspaceID, PageID: pageID,
		ActorID: sql.NullString{String: actor, Valid: actor != ""}, Body: body, Now: now,
	}); err != nil {
		return err
	}
	return pruneVersions(ctx, q, pageID)
}

// maxHistory is how many bytes of a page's history are kept: fifty
// versions of a page of the largest size, thousands of an ordinary one.
var maxHistory int64 = 50_000_000

// pruneVersions deletes a page's versions past its newest maxVersions,
// and past maxHistory bytes of them, oldest first.
func pruneVersions(ctx context.Context, q store.Queries, pageID string) error {
	rows, err := q.ListPageVersionSizes(ctx, pageID)
	if err != nil {
		return err
	}
	var total int64
	for i, r := range rows {
		total += r.Size
		if i == 0 || (i < maxVersions && total <= maxHistory) {
			continue
		}
		if err := q.DeletePageVersion(ctx, r.ID); err != nil {
			return err
		}
	}
	return nil
}

// PageVersion is one version of a page's body: who left it so, and when
// their session ended.
type PageVersion struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actorId"`
	ActorName string    `json:"actorName"`
	Body      string    `json:"body"`
	At        time.Time `json:"at"`
}

// maxVersions is how many versions of a page are kept, and so all one
// answer of its history holds.
const maxVersions = 500

// PageVersions is a page's history, newest first, for someone who sees it.
func (s *Service) PageVersions(ctx context.Context, userID, workspaceID, pageID string) ([]PageVersion, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	page, err := loadRow(ctx, d, d.Dialect, entities["page"], pageID)
	if err != nil {
		return nil, err
	}
	if !page.exists || page.workspaceID != workspaceID || !v.sees(page.values["project_id"]) {
		return nil, apperr.New(apperr.NotFound, "No such page.")
	}
	rows, err := s.st.ListPageVersions(ctx, sqlitedb.ListPageVersionsParams{WorkspaceID: workspaceID, PageID: pageID, MaxRows: maxVersions})
	if err != nil {
		return nil, err
	}
	// Named as task history names them: a guest sees "Someone" for another
	// guest they share no project with.
	hidden, err := s.hiddenFrom(ctx, v, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]PageVersion, 0, len(rows))
	for _, r := range rows {
		actor := r.ActorID.String
		name := nameFor(hidden, actor, r.ActorName)
		if hidden[actor] {
			actor = ""
		}
		out = append(out, PageVersion{ID: r.ID, ActorID: actor, ActorName: name, Body: r.Body, At: r.UpdatedAt})
	}
	return out, nil
}
