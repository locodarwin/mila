package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// TrashDays is how long deleted projects and tasks can be restored.
const TrashDays = 30

// viaField holds, in a history entry's data, the API token it came through.
const viaField = "_via"

// scrub takes out of a history entry what must not outlive its row: a
// comment's text, which its author or an admin may delete or change, and
// which history would otherwise keep for everyone who sees the task.
// Entries written before this still hold it, so it is scrubbed when they
// are read, exported and imported too. It also takes out what is no news:
// a running timer noting that its person is about.
func scrub(entity string, fields map[string]any) {
	switch entity {
	case "comment", "page_comment":
		delete(fields, "body")
	case "time_entry", "project_time_entry":
		// A running timer's sign that its person is about is no news.
		delete(fields, "active_at")
	}
}

// Someone is what a guest sees for another guest they share no project
// with.
const Someone = "Someone"

// hiddenFrom is who a viewer may not name: for a guest, the other guests
// they share no project with, as accounts.People leaves out; nobody for
// anyone else. Members and people who have left are named to everyone.
func (s *Service) hiddenFrom(ctx context.Context, v *viewer, workspaceID string) (map[string]bool, error) {
	if !v.guest() {
		return nil, nil
	}
	rows, err := s.st.ListGuestAccess(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	hidden, shared := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		if row.UserID == v.userID {
			continue
		}
		hidden[row.UserID] = true
		if row.ProjectID != "" && v.projects[row.ProjectID] {
			shared[row.UserID] = true
		}
	}
	for id := range shared {
		delete(hidden, id)
	}
	return hidden, nil
}

// nameFor is a person's name, unless the viewer may not see it.
func nameFor(hidden map[string]bool, actorID, name string) string {
	if hidden[actorID] {
		return Someone
	}
	return name
}

// recordActivity writes what an accepted op did to a task's history, in a
// form the app can turn into a sentence. Reordering alone is not news. A
// change to the notes rendered from their document (fromDoc) joins the
// same person's entry from the same editing session (docs.go).
func (s *Service) recordActivity(ctx context.Context, q store.Queries, e *entity, r *row, accepted []string, wasDeleted bool, actor, workspaceID string, now time.Time, fromDoc bool) error {
	taskID := taskOf(e, r)
	// A private row (a reminder, a My Day entry) is no news to the others
	// who read the task's history, and a quiet one is no news at all.
	if taskID == "" || e.private != "" || e.quiet {
		return nil
	}
	fields := map[string]any{}
	for _, name := range accepted {
		if name != "sort_key" {
			fields[name] = jsonValue(r.values[name])
		}
	}
	scrub(e.name, fields)
	if len(fields) == 0 {
		return nil
	}
	kind := "changed"
	switch {
	case !r.exists:
		kind = "created"
	case slices.Contains(accepted, "deleted_at") && r.values["deleted_at"] != nil:
		kind = "deleted"
	case slices.Contains(accepted, "deleted_at") && wasDeleted:
		kind = "restored"
	}
	if via := viaOf(ctx); via != "" {
		fields[viaField] = via
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if fromDoc && e.name == "task" && kind == "changed" && len(fields) == 1 && fields["notes"] != nil {
		if joined, err := sessionActivity(ctx, q, workspaceID, taskID, actor, data, now); err != nil || joined {
			return err
		}
	}
	// A time-ordered ID keeps events in the order they happened when two
	// share a clock tick, as they can on Windows: the list breaks ties on it.
	return q.InsertActivity(ctx, sqlitedb.InsertActivityParams{
		ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: workspaceID, TaskID: taskID, ActorID: actor,
		Entity: e.name, EntityID: r.id, Kind: kind, Data: string(data), Now: now,
	})
}

// Event is one entry in a task's history.
type Event struct {
	ID        string         `json:"id"`
	ActorID   string         `json:"actorId"`
	ActorName string         `json:"actorName"`
	Via       string         `json:"via,omitempty"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entityId"`
	Kind      string         `json:"kind"`
	Fields    map[string]any `json:"fields"`
	At        time.Time      `json:"at"`
}

// Activity returns a task's history, newest first.
func (s *Service) Activity(ctx context.Context, userID, workspaceID, taskID string) ([]Event, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	task, err := loadRow(ctx, d, d.Dialect, entities["task"], taskID)
	if err != nil {
		return nil, err
	}
	if !task.exists || task.workspaceID != workspaceID || !v.sees(task.values["project_id"]) {
		return nil, apperr.New(apperr.NotFound, "No such task.")
	}
	rows, err := s.st.ListActivity(ctx, sqlitedb.ListActivityParams{WorkspaceID: workspaceID, TaskID: taskID, MaxRows: 200})
	if err != nil {
		return nil, err
	}
	hidden, err := s.hiddenFrom(ctx, v, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Event, len(rows))
	for i, r := range rows {
		// Someone the viewer may not name goes without their ID too, as a
		// page's history has it.
		actor := r.ActorID
		if hidden[actor] {
			actor = ""
		}
		out[i] = Event{ID: r.ID, ActorID: actor, ActorName: nameFor(hidden, r.ActorID, r.ActorName), Entity: r.Entity, EntityID: r.EntityID, Kind: r.Kind, At: r.CreatedAt}
		if err := json.Unmarshal([]byte(r.Data), &out[i].Fields); err != nil {
			return nil, err
		}
		scrub(r.Entity, out[i].Fields)
		if via, ok := out[i].Fields[viaField].(string); ok {
			delete(out[i].Fields, viaField)
			if !hidden[r.ActorID] {
				out[i].Via = viaFor(v, via)
			}
		}
	}
	return out, nil
}

// Trash returns the projects, tasks, pages and templates deleted in the last TrashDays days
// that the user may see, newest deletion first. Restoring one is an
// ordinary op setting deleted_at back to null.
func (s *Service) Trash(ctx context.Context, userID, workspaceID string) ([]Row, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	since := s.now().Add(-TrashDays * 24 * time.Hour)
	out := []Row{}
	for _, name := range []string{"project", "task", "page", "template"} {
		e := entities[name]
		rows, err := listDeleted(ctx, d, d.Dialect, e, workspaceID, since)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			project, err := projectOf(ctx, d, d.Dialect, e, r)
			if err != nil {
				return nil, err
			}
			if v.sees(project) {
				out = append(out, r.snapshot(e))
			}
		}
	}
	return out, nil
}

// Purge permanently removes what was deleted more than TrashDays days ago.
// Rows hanging off a purged task go with it.
func (s *Service) Purge(ctx context.Context) (int64, error) {
	d := s.st.DB()
	before := s.now().Add(-TrashDays * 24 * time.Hour)
	var total int64
	for i := len(syncOrder) - 1; i >= 0; i-- {
		e := entities[syncOrder[i]]
		res, err := d.ExecContext(ctx, rebind(d.Dialect, "DELETE FROM "+e.table+" WHERE deleted_at IS NOT NULL AND deleted_at < ?"), before)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// StorageKeys lists every file in the store that the database still
// names: attachments and pictures.
func (s *Service) StorageKeys(ctx context.Context) (map[string]bool, error) {
	keys, err := s.st.ListStorageKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out, nil
}

// serverHLC is an HLC for a change the server makes itself.
func serverHLC(now time.Time) string {
	return fmt.Sprintf("%013d-0000-server", now.UnixMilli())
}

// NewAttachment describes an uploaded file whose row the server creates:
// a task's attachment, or with PageID a wiki page's.
type NewAttachment struct {
	ID, TaskID, PageID, Filename, Mime, StorageKey string
	Size                                           int64
}

// CreateAttachment adds the row for a file the user just uploaded, through
// the same merge, checks and log as any op, and returns it.
func (s *Service) CreateAttachment(ctx context.Context, userID, workspaceID string, a NewAttachment) (*Row, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	e, fields := entities["attachment"], map[string]any{"task_id": a.TaskID}
	if a.PageID != "" {
		e, fields = entities["page_attachment"], map[string]any{"page_id": a.PageID}
	}
	for k, val := range map[string]any{
		"filename": a.Filename, "mime": a.Mime, "size": a.Size,
		"storage_key": a.StorageKey, "uploaded_by": userID, "deleted_at": nil,
	} {
		fields[k] = val
	}
	raw := map[string]json.RawMessage{}
	for k, val := range fields {
		b, _ := json.Marshal(val)
		raw[k] = b
	}
	op := Op{OpID: uuid.NewString(), Entity: e.name, ID: a.ID, HLC: serverHLC(s.now()), Fields: raw, trusted: true}
	res, changed, err := s.apply(ctx, v, workspaceID, op)
	if err != nil {
		return nil, err
	}
	if res.Status == "rejected" {
		return nil, apperr.New(apperr.Invalid, "%s", res.Message)
	}
	if changed && s.notify != nil {
		if seq, err := s.st.GetChangeSeq(ctx, workspaceID); err == nil {
			s.notify.Changed(workspaceID, seq)
		}
	}
	if changed && s.watch != nil {
		s.watch.Committed()
	}
	d := s.st.DB()
	r, err := loadRow(ctx, d, d.Dialect, e, a.ID)
	if err != nil {
		return nil, err
	}
	snap := r.snapshot(e)
	return &snap, nil
}

// Attachment returns an attachment's row, a task's or a page's, if the
// user may see it.
func (s *Service) Attachment(ctx context.Context, userID, workspaceID, id string) (*Row, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	d := s.st.DB()
	e := entities["attachment"]
	r, err := loadRow(ctx, d, d.Dialect, e, id)
	if err != nil {
		return nil, err
	}
	if !r.exists {
		e = entities["page_attachment"]
		if r, err = loadRow(ctx, d, d.Dialect, e, id); err != nil {
			return nil, err
		}
	}
	project, err := projectOf(ctx, d, d.Dialect, e, r)
	if err != nil {
		return nil, err
	}
	if !r.exists || r.workspaceID != workspaceID || r.values["deleted_at"] != nil || !v.sees(project) {
		return nil, apperr.New(apperr.NotFound, "No such attachment.")
	}
	snap := r.snapshot(e)
	return &snap, nil
}

// CanEditTask reports whether the user may add to a task (upload a file).
func (s *Service) CanEditTask(ctx context.Context, userID, workspaceID, taskID string) error {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return err
	}
	d := s.st.DB()
	task, err := loadRow(ctx, d, d.Dialect, entities["task"], taskID)
	if err != nil {
		return err
	}
	if !task.exists || task.workspaceID != workspaceID || !v.sees(task.values["project_id"]) {
		return apperr.New(apperr.NotFound, "No such task.")
	}
	if v.guest() && !entities["attachment"].guestWritable {
		return apperr.New(apperr.Forbidden, "Guests cannot add files here.")
	}
	return nil
}

// CanEditPage reports whether the user may add to a wiki page (upload a file).
func (s *Service) CanEditPage(ctx context.Context, userID, workspaceID, pageID string) error {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return err
	}
	d := s.st.DB()
	page, err := loadRow(ctx, d, d.Dialect, entities["page"], pageID)
	if err != nil {
		return err
	}
	if !page.exists || page.workspaceID != workspaceID || page.values["deleted_at"] != nil || !v.sees(page.values["project_id"]) {
		return apperr.New(apperr.NotFound, "No such page.")
	}
	if v.guest() && !entities["page_attachment"].guestWritable {
		return apperr.New(apperr.Forbidden, "Guests cannot add files here.")
	}
	return nil
}

// listDeleted reads rows deleted since a time, newest deletion first.
func listDeleted(ctx context.Context, q querier, dialect db.Dialect, e *entity, workspaceID string, since time.Time) ([]*row, error) {
	query := rebind(dialect, "SELECT "+strings.Join(columns(e), ", ")+" FROM "+e.table+
		" WHERE workspace_id = ? AND deleted_at IS NOT NULL AND deleted_at >= ? ORDER BY deleted_at DESC")
	rows, err := q.QueryContext(ctx, query, workspaceID, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*row
	for rows.Next() {
		r, err := scanRow(e, rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
