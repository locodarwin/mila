package syncer

// Links between pages and tasks. A link is an
// ordinary Markdown link to its address in the app: a page's
// (/wiki?page=<id>, or /projects/<id>?tab=wiki&page=<id>) or a task's
// (?task=<id> on any page). When a page or task is renamed, the links to
// it that still show its old name take the new one, in every document
// that holds one, so a link always shows the current title. doc_links
// lists which documents link to what, so a rename reads only those.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/ydoc"
)

// appHost is the host of the app's own address (MILA_BASE_URL), whose full
// addresses are the app's too: SetAppAddress, at start.
var appHost string

// SetAppAddress names the app's own address, so a link given in full (one
// copied from the browser) is read as a link in the app.
func SetAppAddress(base string) {
	appHost = ""
	if u, err := url.Parse(base); err == nil {
		appHost = strings.ToLower(u.Host)
	}
}

// LinkTarget is the ID of the page or task a link's address points at,
// or "" for an address that is not one of the app's: a path in it, or a
// full address of the app's own host. A browser follows "//host", a
// backslash for a slash and a tab inside to another site, so none of
// those is the app's, though shown with a page's or task's title (and
// renamed with it) they would look it.
func LinkTarget(href string) string {
	if strings.ContainsAny(href, "\\ \t\n\r") {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	switch {
	case u.Scheme == "" && strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//"):
	case (u.Scheme == "http" || u.Scheme == "https") && appHost != "" && strings.ToLower(u.Host) == appHost:
	default:
		return ""
	}
	q := u.Query()
	isID := func(s string) bool { _, err := uuid.Parse(s); return err == nil && len(s) == 36 }
	if p := q.Get("page"); isID(p) && (u.Path == "/wiki" || strings.HasPrefix(u.Path, "/projects/")) {
		return p
	}
	if t := q.Get("task"); isID(t) && strings.HasPrefix(u.Path, "/") {
		return t
	}
	return ""
}

// renameLinks gives the links to a renamed page or task in the
// workspace's documents its new name, as the server's own edit of each,
// whoever renamed it: a guest's rename reaches documents they cannot see,
// and is never refused for them. The documents are those doc_links says
// link to it, and of those only the ones whose every reader sees the
// target (its scope, as projectOf gives it): the new name of a task in a
// project a guest is not given never shows in a page they read.
func (s *Service) renameLinks(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID, targetID string, scope any, oldName, newName string, now time.Time) error {
	if strings.TrimSpace(oldName) == "" || oldName == newName {
		return nil
	}
	d := s.st.DB()
	v = &viewer{userID: v.userID, role: authz.Owner, server: true}
	points := func(href string) bool { return LinkTarget(href) == targetID }
	for _, k := range docKinds {
		ids, err := queryIDs(ctx, tx, rebind(d.Dialect, "SELECT "+k.parentField+" FROM doc_links"+
			" WHERE workspace_id = ? AND target_id = ? AND "+k.parentField+" IS NOT NULL"), workspaceID, targetID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			holder, err := loadRow(ctx, tx, d.Dialect, entities[k.target], id)
			if err != nil {
				return err
			}
			if !holder.exists {
				continue
			}
			docScope, err := projectOf(ctx, tx, d.Dialect, entities[k.target], holder)
			if err != nil {
				return err
			}
			if seen, err := seenByReaders(ctx, tx, d.Dialect, workspaceID, docScope, scope); err != nil || !seen {
				return err
			}
			rows, err := loadDoc(ctx, tx, d.Dialect, k, id)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				// Text from before live documents, which the server has yet
				// to make one of: it makes it now.
				if rows, err = s.seedDoc(ctx, q, tx, v, workspaceID, k, id, now); err != nil || len(rows) == 0 {
					return err
				}
			}
			doc, err := ydoc.Read(updatesOf(rows)...)
			if err != nil {
				return err
			}
			next, changed := ydoc.RenameLinks(doc, points, oldName, newName)
			if !changed {
				continue
			}
			update, err := ydoc.Replace(updatesOf(rows), next, freshClient())
			if err != nil || update == nil {
				return err
			}
			if err := s.derivedOp(ctx, q, tx, v, workspaceID, k.entity, uuid.Must(uuid.NewV7()).String(), serverHLC(now), map[string]any{
				k.parentField: id, "data": base64.StdEncoding.EncodeToString(update), "deleted_at": nil,
			}, now); err != nil {
				return err
			}
			if err := s.renderDoc(ctx, q, tx, v, workspaceID, k, id, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// seenByReaders is whether everyone who reads what is in doc's scope sees
// what is in target's. Members see every project, so only guests can
// miss it: those given doc's project (all of them, for a row everyone
// sees; none, for one outside any project) who are not given target's.
func seenByReaders(ctx context.Context, q querier, dialect db.Dialect, workspaceID string, doc, target any) (bool, error) {
	if target == everyone || doc == nil || doc == target {
		return true, nil
	}
	readers := ""
	args := []any{workspaceID}
	if doc != everyone {
		readers = " AND EXISTS (SELECT 1 FROM project_access a WHERE a.user_id = m.user_id AND a.project_id = ?)"
		args = append(args, doc)
	}
	// A target outside any project is no guest's to see.
	t, _ := target.(string)
	args = append(args, t)
	var blind int
	err := q.QueryRowContext(ctx, rebind(dialect, "SELECT COUNT(*) FROM memberships m WHERE m.workspace_id = ? AND m.role = '"+string(authz.Guest)+"'"+readers+
		" AND NOT EXISTS (SELECT 1 FROM project_access b WHERE b.user_id = m.user_id AND b.project_id = ?)"), args...).Scan(&blind)
	return blind == 0, err
}

// seedDoc makes the first document of a task's notes or a page's body
// from its Markdown, as SeedDocs would, and returns its rows: none when
// the row is gone or its text is empty.
func (s *Service) seedDoc(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, k docKind, id string, now time.Time) ([]docRow, error) {
	d := s.st.DB()
	target, err := loadRow(ctx, tx, d.Dialect, entities[k.target], id)
	if err != nil || !target.exists {
		return nil, err
	}
	md, _ := target.values[k.field].(string)
	if md == "" {
		return nil, nil
	}
	if err := s.markdownWritten(ctx, q, tx, v, workspaceID, k, id, md, now); err != nil {
		return nil, err
	}
	return loadDoc(ctx, tx, d.Dialect, k, id)
}

// mayLink is whether Markdown could hold a link to a page or task: every
// such address has one of these in its query.
func mayLink(md string) bool {
	return strings.Contains(md, "page=") || strings.Contains(md, "task=")
}

// linkTargets are the pages and tasks Markdown links to, each once.
func linkTargets(md string) []string {
	if !mayLink(md) {
		return nil
	}
	var out []string
	for _, href := range ydoc.Hrefs(ydoc.Parse(md)) {
		if t := LinkTarget(href); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// indexLinks keeps doc_links in step with a task's notes or a page's body
// as written: fresh is a row just made, which has none yet.
func indexLinks(ctx context.Context, q querier, dialect db.Dialect, k docKind, workspaceID, id, md string, fresh bool) error {
	if !fresh {
		if _, err := q.ExecContext(ctx, rebind(dialect, "DELETE FROM doc_links WHERE "+k.parentField+" = ?"), id); err != nil {
			return err
		}
	}
	for _, target := range linkTargets(md) {
		if _, err := q.ExecContext(ctx, rebind(dialect, "INSERT INTO doc_links (workspace_id, "+k.parentField+", target_id)"+
			" VALUES (?, ?, ?) ON CONFLICT DO NOTHING"), workspaceID, id, target); err != nil {
			return err
		}
	}
	return nil
}

// IndexLinks fills doc_links for the notes and bodies that may hold links
// and have none listed: those written before it existed. It runs at
// start, before the server takes requests; after that every write keeps
// the table itself. It returns how many documents it read.
func (s *Service) IndexLinks(ctx context.Context) (int, error) {
	d := s.st.DB()
	read := 0
	for _, k := range docKinds {
		type pending struct{ id, ws, text string }
		var todo []pending
		rows, err := d.QueryContext(ctx, `SELECT t.id, t.workspace_id, t.`+k.field+` FROM `+entities[k.target].table+` t
			WHERE (t.`+k.field+` LIKE '%page=%' OR t.`+k.field+` LIKE '%task=%')
			AND NOT EXISTS (SELECT 1 FROM doc_links l WHERE l.`+k.parentField+` = t.id) ORDER BY t.id`)
		if err != nil {
			return read, err
		}
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.ws, &p.text); err != nil {
				_ = rows.Close()
				return read, err
			}
			todo = append(todo, p)
		}
		if err := rows.Close(); err != nil {
			return read, err
		}
		if err := rows.Err(); err != nil {
			return read, err
		}
		for start := 0; start < len(todo); start += restoreChunk {
			batch := todo[start:min(start+restoreChunk, len(todo))]
			err := s.st.InTxConn(ctx, func(_ store.Queries, tx *sql.Tx) error {
				for _, p := range batch {
					if err := indexLinks(ctx, tx, d.Dialect, k, p.ws, p.id, p.text, false); err != nil {
						return fmt.Errorf("%s %s: %w", k.target, p.id, err)
					}
				}
				return nil
			})
			if err != nil {
				return read, err
			}
			read += len(batch)
		}
	}
	return read, nil
}
