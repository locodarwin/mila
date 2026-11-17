// Package syncer keeps every device's copy of a workspace in step with the
// server. Clients send operations (ops) that set fields on rows; the server
// merges them field by field, last writer wins by HLC, records each
// accepted change in a numbered log, and tells connected clients to pull.
package syncer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Row is a synced row as clients see it.
type Row struct {
	Entity string            `json:"entity"`
	ID     string            `json:"id"`
	Fields map[string]any    `json:"fields"`
	Clocks map[string]string `json:"clocks"`
}

// Change is one accepted op from the log: the fields it changed, their
// new values and HLCs.
type Change struct {
	Seq    int64             `json:"seq"`
	Entity string            `json:"entity"`
	ID     string            `json:"id"`
	Fields map[string]any    `json:"fields"`
	Clocks map[string]string `json:"clocks"`
}

// Op sets fields on one row. Creating a row is an op on an ID the server
// has not seen; deleting is setting deleted_at.
type Op struct {
	OpID   string                     `json:"opId"`
	Entity string                     `json:"entity"`
	ID     string                     `json:"id"`
	HLC    string                     `json:"hlc"`
	Fields map[string]json.RawMessage `json:"fields"`

	// trusted ops come from the server itself (an upload creating its
	// attachment row) and may set server-only fields.
	trusted bool
	// rest is set on a write through the REST endpoints (rest.go): the
	// server stamps its HLC, and it may need its row to exist or not.
	rest *restCond
	// derived ops are the server's own, following a change to a live
	// document (docs.go): its rendered notes, its merged rows.
	derived bool
	// serverID says the server chose the row's ID (a REST create that
	// names none), rather than whoever sent the op.
	serverID bool
	// Bulk marks one of many rows made at once: a template's or an import's.
	Bulk bool `json:"bulk,omitempty"`
	// Batch names one change to many tasks (selecting several in the app):
	// each of its ops carries the same ID. Rules count it once, and the
	// assignments it makes are pushed as one. See batchOf.
	Batch string `json:"batch,omitempty"`
}

// batchOf is an op's batch, if it is one an app could have made: up to 64
// letters, digits and dashes. Anything else is no batch. The app chooses
// it, so it is held to whoever sent it, in one workspace, for one stretch
// of batchSpan: someone else's batch is never theirs to join, and a batch
// sent again and again counts again each stretch, not once for good.
func batchOf(op Op, userID, workspaceID string, now time.Time) string {
	if !ValidBatch(op.Batch) {
		return ""
	}
	sum := sha256.Sum256([]byte(userID + "/" + workspaceID + "/" + op.Batch + "/" + strconv.FormatInt(now.Unix()/int64(batchSpan/time.Second), 10)))
	return hex.EncodeToString(sum[:16])
}

// ValidBatch says whether a batch's name is one Mila takes: 1 to 64
// letters, digits and dashes.
func ValidBatch(batch string) bool {
	if len(batch) == 0 || len(batch) > 64 {
		return false
	}
	for _, c := range batch {
		if c != '-' && (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// batchSpan is the stretch a batch counts once in: far longer than any one
// change to many tasks takes to send.
const batchSpan = 10 * time.Minute

// Result says what became of one op.
type Result struct {
	OpID   string `json:"opId"`
	Status string `json:"status"` // applied, duplicate or rejected
	// Message explains a rejection, and Row is the server's copy of the
	// row so the client can undo its optimistic change. Row is nil when
	// the row does not exist on the server.
	Message string `json:"message,omitempty"`
	Row     *Row   `json:"row,omitempty"`

	// reason is the rejection, with its kind, for REST; created says an
	// applied op made its row.
	reason  *rejection
	created bool
}

// Snapshot is a workspace's live rows and the change number they reflect.
type Snapshot struct {
	Seq int64 `json:"seq"`
	// Epoch is sent back with every pull; see Service.Epoch.
	Epoch string `json:"epoch"`
	Rows  []Row  `json:"rows"`
}

// Changes is a page of the log after a given change number.
type Changes struct {
	Seq     int64    `json:"seq"` // pull after this next time
	More    bool     `json:"more"`
	Changes []Change `json:"changes"`
}

// Notifier hears about committed changes, to poke connected clients.
type Notifier interface {
	Changed(workspaceID string, seq int64)
}

const (
	maxOpsPerPush  = 500
	maxChangesPage = 500
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Service is the sync API used by the HTTP handlers.
type Service struct {
	st     *store.Store
	log    *slog.Logger
	notify Notifier
	watch  Watcher
	now    func() time.Time
	// epoch caches the sync epoch, which changes only while the server is
	// stopped (a restore).
	epochMu sync.Mutex
	epoch   string
}

// New builds the service. notify may be nil.
func New(st *store.Store, log *slog.Logger, notify Notifier) *Service {
	return &Service{st: st, log: log, notify: notify, now: store.Now}
}

// viewer is one member's view of a workspace.
type viewer struct {
	userID   string
	role     authz.Role
	projects map[string]bool // guests only: the projects they can see
	// resetSeq is the change number at which the member's access last
	// changed: a copy pulled from before it is wrong for them now.
	resetSeq int64
	// server marks a viewer the server makes to act for itself (Forget,
	// as an account is deleted), not someone's access read from their
	// membership.
	server bool
}

func (v *viewer) guest() bool { return v.role == authz.Guest }

// everyone marks rows every member sees, guests too (labels). It is
// stored as the change log's project_id for such rows.
const everyone = "*"

// privateScope is the scope of a row only one user sees: "@" and their
// ID, stored as the change log's project_id.
func privateScope(userID string) string { return "@" + userID }

// maxTemplates bounds a workspace's live templates.
const maxTemplates = 200

// maxWorkspaceThemes bounds the themes a workspace offers.
const maxWorkspaceThemes = 10

// bothScope is the scope of a row in two projects (a dependency between
// tasks in different ones): "&", one project, "&", the other, with the
// inbox as "". It is the scope itself when both are the same.
func bothScope(a, b any) any {
	if a == b {
		return a
	}
	part := func(p any) string { s, _ := p.(string); return s }
	return "&" + part(a) + "&" + part(b)
}

func scopePart(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// guestOnly names, for a refusal, the rows a guest can never write.
var guestOnly = map[string]string{
	"project": "projects", "status": "statuses", "label": "labels",
	"custom_field": "custom fields", "template": "templates", "automation": "automations",
	"workspace_theme": "workspace themes", "saved_view": "saved views",
}

// guestRefusal says which rule refused a guest's write where the guest's
// own request decides it: the kind of row, or a task or page they make or
// move outside every project (seen is whether they see the row as it was,
// or it is new). Anything else gets the general rule, which says nothing
// of rows they cannot see.
func guestRefusal(e *entity, seen bool, after any) string {
	if !e.guestWritable {
		what, ok := guestOnly[e.name]
		if !ok {
			what = e.label() + " rows"
		}
		return fmt.Sprintf("Guests can't make or change %s; they work on the tasks and pages of the projects shared with them.", what)
	}
	if seen && after == nil {
		switch e.name {
		case "task":
			return "Guests can't put tasks in the Inbox; only in the projects shared with them."
		case "page":
			return "Guests can't change the workspace's wiki; only the wikis of the projects shared with them."
		}
	}
	return "Guests can only change tasks in the projects shared with them."
}

// sees reports whether the viewer may read a row in project (nil for the
// workspace's inbox).
func (v *viewer) sees(project any) bool {
	if p, ok := project.(string); ok && strings.HasPrefix(p, "@") {
		return p == privateScope(v.userID)
	}
	if p, ok := project.(string); ok && strings.HasPrefix(p, "&") {
		a, b, _ := strings.Cut(p[1:], "&")
		return v.sees(scopePart(a)) && v.sees(scopePart(b))
	}
	if !v.guest() || project == everyone {
		return true
	}
	p, ok := project.(string)
	return ok && v.projects[p]
}

func (s *Service) viewer(ctx context.Context, q store.Queries, userID, workspaceID string) (*viewer, error) {
	if _, err := q.GetWorkspace(ctx, workspaceID); errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "No such workspace.")
	} else if err != nil {
		return nil, err
	}
	m, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return nil, err
	}
	v := &viewer{userID: userID, role: authz.Role(m.Role), resetSeq: m.ResetSeq}
	if v.guest() {
		ids, err := q.ListAccessibleProjects(ctx, sqlitedb.ListAccessibleProjectsParams{UserID: userID, WorkspaceID: workspaceID})
		if err != nil {
			return nil, err
		}
		v.projects = make(map[string]bool, len(ids))
		for _, id := range ids {
			v.projects[id] = true
		}
	}
	return v, nil
}

// projectOf is the project a row belongs to: its own, its task's for rows
// that hang off a task, everyone for labels, or nil for the inbox.
func projectOf(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row) (any, error) {
	switch {
	case e.workspaceWide, e.wideWhenNull && r.values[e.projectOf] == nil:
		return everyone, nil
	case e.projectOf == "id":
		return r.id, nil
	case e.parent != "":
		parentID, _ := r.values[e.parent].(string)
		if parentID == "" {
			return nil, nil
		}
		parent, err := loadRow(ctx, q, dialect, entities[e.parentOf()], parentID)
		if err != nil {
			return nil, err
		}
		if e.alsoIn == "" {
			return parent.values["project_id"], nil
		}
		otherID, _ := r.values[e.alsoIn].(string)
		other, err := loadRow(ctx, q, dialect, entities["task"], otherID)
		if err != nil {
			return nil, err
		}
		return bothScope(parent.values["project_id"], other.values["project_id"]), nil
	}
	return r.values[e.projectOf], nil
}

// parentOf is the entity a row hangs off: a task, or what parentEntity names.
func (e *entity) parentOf() string {
	if e.parentEntity != "" {
		return e.parentEntity
	}
	return "task"
}

// taskOf is the task whose history a row belongs to, or "" for rows that
// are not about a task.
func taskOf(e *entity, r *row) string {
	if e.name == "task" {
		return r.id
	}
	if e.parent != "" && e.parentOf() == "task" {
		id, _ := r.values[e.parent].(string)
		return id
	}
	return ""
}

// Bootstrap returns every row of the workspace the user may see, the trash
// included: rows deleted but not yet purged, and the rows hanging off a
// deleted task. A device that bootstrapped without them would turn a later
// change to one (a restore from the trash, an edit made offline) into a
// row holding only the fields changed, with no title and no deleted_at.
func (s *Service) Bootstrap(ctx context.Context, userID, workspaceID string) (*Snapshot, error) {
	var snap *Snapshot
	err := s.StreamBootstrap(ctx, userID, workspaceID, func(seq int64, epoch string) error {
		snap = &Snapshot{Seq: seq, Epoch: epoch, Rows: []Row{}}
		return nil
	}, func(r Row) error {
		snap.Rows = append(snap.Rows, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// StreamBootstrap is Bootstrap a row at a time, so a large workspace is
// sent without ever being held whole: start gets the change number and
// epoch before the first row, and the first error from either stops it.
func (s *Service) StreamBootstrap(ctx context.Context, userID, workspaceID string, start func(seq int64, epoch string) error, emit func(Row) error) error {
	// The change number is read before the rows, so a change committed in
	// between is in the rows and also pulled again later. Applying it twice
	// is harmless; missing it would not be. It is read before the viewer's
	// access too: a reset committed after it is past the snapshot's number,
	// so the next pull answers 410 even though the rows were filtered by
	// the access from before.
	seq, err := s.st.GetChangeSeq(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return err
	}
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return err
	}
	epoch, err := s.Epoch(ctx)
	if err != nil {
		return err
	}
	if err := start(seq, epoch); err != nil {
		return err
	}
	var failed error
	err = s.visible(ctx, v, workspaceID, syncOrder, true, func(e *entity, r *row) {
		if failed == nil {
			failed = emit(r.snapshot(e))
		}
	})
	if err != nil {
		return err
	}
	return failed
}

// Rows returns the live rows of the named entities that a member sees in
// a workspace now, as Bootstrap would send them, in sync order. It fails
// with NotFound when the user is not a member.
func (s *Service) Rows(ctx context.Context, userID, workspaceID string, names ...string) ([]Row, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	var read []string
	for _, name := range syncOrder {
		// Rows that hang off a task or page need those read first.
		if slices.Contains(names, name) || slices.ContainsFunc(names, func(n string) bool {
			e := entities[n]
			return e != nil && e.parent != "" && e.parentOf() == name
		}) {
			read = append(read, name)
		}
	}
	out := []Row{}
	err = s.visible(ctx, v, workspaceID, read, false, func(e *entity, r *row) {
		if slices.Contains(names, e.name) {
			out = append(out, r.snapshot(e))
		}
	})
	return out, err
}

// visible calls emit for every live row of the named entities, read in
// that order, that the viewer may see; with deleted, for the rows still
// kept after their deletion (the trash) too.
func (s *Service) visible(ctx context.Context, v *viewer, workspaceID string, names []string, deleted bool, emit func(*entity, *row)) error {
	d := s.st.DB()
	// The projects of tasks and pages, filled as they are read, before
	// the rows that hang off them.
	parentProject := map[string]map[string]any{"task": {}, "page": {}}
	for _, name := range names {
		e := entities[name]
		list := listRows
		if deleted {
			list = listAll
		}
		rows, err := list(ctx, d, d.Dialect, e, workspaceID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			var project any
			switch {
			case e.workspaceWide, e.wideWhenNull && r.values[e.projectOf] == nil:
				project = everyone
			case e.parent != "":
				id, _ := r.values[e.parent].(string)
				p, ok := parentProject[e.parentOf()][id]
				if !ok {
					continue // its task or page is deleted
				}
				project = p
				if e.alsoIn != "" {
					otherID, _ := r.values[e.alsoIn].(string)
					other, ok := parentProject["task"][otherID]
					if !ok {
						continue
					}
					project = bothScope(p, other)
				}
			case e.projectOf == "id":
				project = r.id
			default:
				project = r.values[e.projectOf]
			}
			if byID, ok := parentProject[e.name]; ok {
				byID[r.id] = project
			}
			if owner, ok := r.values[e.private].(string); e.private != "" && ok {
				project = privateScope(owner)
			}
			if v.sees(project) {
				emit(e, r)
			}
		}
	}
	return nil
}

// changeData is a change_log row's data column.
type changeData struct {
	Fields map[string]any    `json:"fields"`
	Clocks map[string]string `json:"clocks"`
}

// Pull returns the changes after change number `after`. A client that is
// ahead of the server (the database was restored from a backup), behind
// the compacted part of the log, or behind the moment the user's access
// changed (store.ResetMember), gets a Gone error and must bootstrap again.
func (s *Service) Pull(ctx context.Context, userID, workspaceID string, after int64) (*Changes, error) {
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	current, err := s.st.GetChangeSeq(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if after > current || after < 0 {
		return nil, apperr.New(apperr.Gone, "This device's copy is out of step with the server and will be reloaded.")
	}
	if after < v.resetSeq {
		return nil, accessChanged()
	}
	logged, err := s.st.ListChanges(ctx, sqlitedb.ListChangesParams{WorkspaceID: workspaceID, After: after, MaxRows: maxChangesPage})
	if err != nil {
		return nil, err
	}
	// The floor is read after the changes: compaction raises it in the
	// same transaction as it deletes, so if any change after `after` was
	// gone when they were read, the floor read now says so.
	floor, err := s.st.GetLogFloor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if after < floor {
		return nil, apperr.New(apperr.Gone, "This device has been away longer than the server keeps changes, so its copy will be reloaded.")
	}
	// The same goes for a reset: one committed since the viewer was read
	// would leave these changes filtered by the access from before it, and
	// the page's number past it, so no later pull would answer 410.
	if err := s.checkReset(ctx, userID, workspaceID, after); err != nil {
		return nil, err
	}
	out := &Changes{Seq: after, More: len(logged) == maxChangesPage, Changes: []Change{}}
	for _, c := range logged {
		out.Seq = c.Seq
		var project any
		if c.ProjectID.Valid {
			project = c.ProjectID.String
		}
		if !v.sees(project) {
			continue
		}
		var data changeData
		if err := json.Unmarshal([]byte(c.Data), &data); err != nil {
			return nil, err
		}
		out.Changes = append(out.Changes, Change{Seq: c.Seq, Entity: c.Entity, ID: c.EntityID, Fields: data.Fields, Clocks: data.Clocks})
	}
	return out, nil
}

func accessChanged() error {
	return apperr.New(apperr.Gone, "What you can see in this workspace changed, so this device's copy will be reloaded.")
}

// checkReset fails with Gone when the user's access to the workspace was
// reset after change number `after` (store.ResetMember), and with NotFound
// when they are no longer a member.
func (s *Service) checkReset(ctx context.Context, userID, workspaceID string, after int64) error {
	m, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return err
	}
	if after < m.ResetSeq {
		return accessChanged()
	}
	return nil
}

// rejection is an op the server refuses; it rolls back the op's
// transaction and is reported in the op's Result. Its kind and field are
// for REST, which answers with them; sync sends only the message.
type rejection struct {
	msg   string
	kind  apperr.Kind
	field string
}

func (r *rejection) Error() string { return r.msg }

// madeTogether says whether an op that would take a firstWins field back
// to an older clock is another making of the row: a whole new copy of a
// repeating task under the ID derived from the task it follows, naming
// its maker, and made after that task was. Anything else (an old clock
// sent on purpose, say) would let anyone who can edit a task rewrite or
// clear who made it.
func madeTogether(ctx context.Context, q querier, dialect db.Dialect, e *entity, id, workspaceID string, stored map[string]any, op Op, val any) (bool, error) {
	if val == nil || e.name != "task" {
		return false, nil
	}
	for _, f := range e.fields {
		if _, given := op.Fields[f.name]; f.required && !given {
			return false, nil
		}
	}
	series, _ := stored["series_id"].(string)
	if raw, given := op.Fields["series_id"]; series == "" || !given {
		return false, nil
	} else if v, err := decode(e.field("series_id"), raw); err != nil || v != series {
		return false, nil
	}
	ids, err := queryIDs(ctx, q, rebind(dialect,
		"SELECT id FROM tasks WHERE workspace_id = ? AND (series_id = ? OR id = ?) AND id <> ?"), workspaceID, series, series, id)
	if err != nil {
		return false, err
	}
	for _, prevID := range ids {
		if recur.NextTaskID(prevID) != id {
			continue
		}
		prev, err := loadRow(ctx, q, dialect, e, prevID)
		if err != nil {
			return false, err
		}
		// When the task it follows was made: its creator's clock, or for a
		// task older than that field, its oldest write.
		made := prev.clocks["created_by"]
		if made == "" {
			for _, c := range prev.clocks {
				if made == "" || c < made {
					made = c
				}
			}
		}
		return op.HLC > made, nil
	}
	return false, nil
}

// resetGuestsAcross resets the copy of every guest who sees one of two
// projects and not the other, when a row moves between them. The change
// log holds only the fields an op changed, and logs a task's comments
// and checklist under the project they were in then: a guest who now
// sees the row would get a few fields of it and none of what hangs off
// it, and one who no longer sees it would keep a stale copy. Moves
// between projects are rare, and guests few, so they load afresh.
func resetGuestsAcross(ctx context.Context, q store.Queries, workspaceID string, before, after any) error {
	rows, err := q.ListGuestAccess(ctx, workspaceID)
	if err != nil {
		return err
	}
	guests := map[string]*viewer{}
	var order []string
	for _, row := range rows {
		v := guests[row.UserID]
		if v == nil {
			v = &viewer{userID: row.UserID, role: authz.Guest, projects: map[string]bool{}}
			guests[row.UserID] = v
			order = append(order, row.UserID)
		}
		if row.ProjectID != "" {
			v.projects[row.ProjectID] = true
		}
	}
	for _, id := range order {
		if v := guests[id]; v.sees(before) != v.sees(after) {
			if err := store.ResetMember(ctx, q, workspaceID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func reject(format string, args ...any) error {
	return &rejection{msg: fmt.Sprintf(format, args...), kind: apperr.Invalid}
}

// refuse is reject with a kind other than Invalid, or naming the field at
// fault.
func refuse(kind apperr.Kind, field, format string, args ...any) error {
	return &rejection{msg: fmt.Sprintf(format, args...), kind: kind, field: field}
}

// Push applies ops in order and reports on each. It returns the
// workspace's change number afterward.
func (s *Service) Push(ctx context.Context, userID, workspaceID string, ops []Op) ([]Result, int64, error) {
	if len(ops) > maxOpsPerPush {
		return nil, 0, apperr.New(apperr.Invalid, "At most %d operations per push.", maxOpsPerPush)
	}
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	results := make([]Result, len(ops))
	applied := false
	for i, op := range ops {
		res, changed, err := s.apply(ctx, v, workspaceID, op)
		if err != nil {
			return nil, 0, err
		}
		results[i] = res
		applied = applied || changed
	}
	seq, err := s.st.GetChangeSeq(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	if applied && s.notify != nil {
		s.notify.Changed(workspaceID, seq)
	}
	if applied && s.watch != nil {
		s.watch.Committed()
	}
	return results, seq, nil
}

// apply merges one op in its own transaction. It reports whether the log
// grew.
func (s *Service) apply(ctx context.Context, v *viewer, workspaceID string, op Op) (Result, bool, error) {
	res := Result{OpID: op.OpID, Status: "applied"}
	e := entities[op.Entity]
	now := s.now()
	switch {
	case !idPattern.MatchString(op.OpID):
		return Result{OpID: op.OpID, Status: "rejected", Message: "The operation has no valid ID."}, false, nil
	case e == nil:
		res.Status, res.Message = "rejected", fmt.Sprintf("%q is not something Mila syncs.", op.Entity)
		return res, false, nil
	case !idPattern.MatchString(op.ID):
		res.Status, res.Message = "rejected", "The row has no valid ID."
		return res, false, nil
	}
	var err error
	if op.ID, err = s.currentID(ctx, e, op.ID); err != nil {
		return res, false, err
	}
	if op.rest == nil {
		if err := checkHLC(op.HLC, now); err != nil {
			res.Status, res.Message = "rejected", err.Error()
			return res, false, nil
		}
	}

	changed, created := false, false
	d := s.st.DB()
	err = s.st.InTxConn(ctx, func(q store.Queries, tx *sql.Tx) error {
		return s.applyIn(ctx, q, tx, v, workspaceID, op, now, &res, &changed, &created)
	})
	var rej *rejection
	if errors.As(err, &rej) {
		res.Status, res.Message, res.reason = "rejected", rej.msg, rej
		current, err := loadRow(ctx, d, d.Dialect, e, op.ID)
		if err != nil {
			return res, false, err
		}
		project, err := projectOf(ctx, d, d.Dialect, e, current)
		if err != nil {
			return res, false, err
		}
		if owner, ok := current.values[e.private].(string); e.private != "" && current.exists && ok {
			project = privateScope(owner)
		}
		if current.exists && current.workspaceID == workspaceID && v.sees(project) {
			snap := current.snapshot(e)
			res.Row = &snap
		}
		return res, false, nil
	}
	res.created = created
	return res, changed, err
}

// applyIn is apply's work inside its transaction, so the server can apply
// ops of its own in the same one (docs.go): the notes a live document
// renders to, and a document's rows made from a write to notes.
func (s *Service) applyIn(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, workspaceID string, op Op, now time.Time, res *Result, changed, created *bool) error {
	e := entities[op.Entity]
	d := s.st.DB()
	// Access as it is now, not as it was when the push began: someone
	// removed, demoted or given fewer projects meanwhile goes no further.
	if !v.server {
		m, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: v.userID})
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.New(apperr.NotFound, "No such workspace.")
		}
		if err != nil {
			return err
		}
		if m.ResetSeq != v.resetSeq || authz.Role(m.Role) != v.role {
			return accessChanged()
		}
	}
	if n, err := q.RecordAppliedOp(ctx, sqlitedb.RecordAppliedOpParams{OpID: op.OpID, WorkspaceID: workspaceID, Now: now}); err != nil {
		return err
	} else if n == 0 {
		res.Status = "duplicate"
		return nil
	}
	load := loadRow
	if op.rest != nil {
		load = loadRowLocked
	}
	r, err := load(ctx, tx, d.Dialect, e, op.ID)
	if err != nil {
		return err
	}
	if op.rest != nil {
		// Its condition, the fields a new row is given, and its HLC.
		if err := s.prepareREST(ctx, tx, d.Dialect, v, e, r, &op, workspaceID, now); err != nil {
			return err
		}
	}
	if r.exists && r.workspaceID != workspaceID {
		return reject("That ID belongs to something else.")
	}
	if !r.exists && e.serverCreated && !op.trusted {
		return refuse(apperr.Forbidden, "", "A new %s can only be made by the server.", e.name)
	}
	before, err := projectOf(ctx, tx, d.Dialect, e, r)
	if err != nil {
		return err
	}
	wasDeleted := r.exists && r.values["deleted_at"] != nil
	previous := maps.Clone(r.values)

	// Merge: a field is taken only if this op is newer than the write
	// that set it. Older fields are dropped quietly; the client gets
	// the newer value from its next pull.
	// news are the accepted fields whose value changed: what the history
	// and the watcher hear of.
	var accepted, news []string
	for name, raw := range op.Fields {
		f := e.field(name)
		if f == nil {
			return refuse(apperr.Invalid, name, "%s has no field %q.", e.name, name)
		}
		val, err := decode(f, raw)
		if err != nil {
			return refuse(apperr.Invalid, name, "%s: %v", name, err)
		}
		if f.serverSet && !op.trusted {
			return refuse(apperr.Forbidden, name, "%s is set by the server.", name)
		}
		if f.kind == kSelf && val != nil && val != v.userID {
			return refuse(apperr.Forbidden, name, "%s must be you.", name)
		}
		// A REST write that sets a field to what it holds already is still
		// the newest word on it, so a device's older edit made offline
		// cannot undo it later: the field is stamped and logged, though
		// it is no news for the history. A field fixed at creation has
		// nothing to race, and is left alone.
		same := op.rest != nil && r.exists && sameValue(r.values[name], val)
		if same && (f.immutable || f.firstWins) {
			continue
		}
		switch {
		case r.exists && f.firstWins:
			if c := r.clocks[name]; c == "" || op.HLC >= c {
				continue
			}
			if ok, err := madeTogether(ctx, tx, d.Dialect, e, r.id, r.workspaceID, previous, op, val); err != nil {
				return err
			} else if !ok {
				return refuse(apperr.Invalid, name, "%s cannot be changed after it is set.", name)
			}
		case r.exists && f.immutable && r.values[name] != val:
			return refuse(apperr.Invalid, name, "%s cannot be changed after it is set.", name)
		case r.clocks[name] >= op.HLC:
			continue
		}
		r.values[name] = val
		r.clocks[name] = op.HLC
		accepted = append(accepted, name)
		if !same {
			news = append(news, name)
		}
	}
	if e.name == "task" {
		if err := fillTaskDates(ctx, tx, d.Dialect, r, op.HLC, &accepted); err != nil {
			return err
		}
	}
	if !r.exists {
		for _, f := range e.fields {
			if _, given := op.Fields[f.name]; f.required && !given {
				return refuse(apperr.Invalid, f.name, "A new %s needs %s.", e.name, f.name)
			}
		}
		r.workspaceID = workspaceID
	}
	if len(accepted) == 0 {
		// Nothing newer, so nothing changes. A guest is still told no
		// about a row they cannot see: their device would otherwise keep
		// its copy of the edit as if it had been taken, a row that no
		// pull or bootstrap ever corrects (one they edited offline before
		// losing the project).
		if v.guest() && r.exists && !v.sees(before) {
			return refuse(apperr.Forbidden, "", "Guests can only change tasks in the projects shared with them.")
		}
		return nil
	}
	// Who may write it comes first: a refusal after looking at what the
	// row names (does that task exist, what is it called) would tell a
	// guest about rows they cannot see.
	after, err := projectOf(ctx, tx, d.Dialect, e, r)
	if err != nil {
		return err
	}
	if e.adminsOnly && !authz.Can(v.role, authz.ManageMembers) {
		return refuse(apperr.Forbidden, "", "Only owners and admins can change the workspace's themes.")
	}
	if v.guest() {
		// A guest edits tasks only, and only where they can see both the
		// row's old project and its new one.
		allowed := e.guestWritable && v.sees(after) && (!r.exists || v.sees(before))
		if !allowed {
			return refuse(apperr.Forbidden, "", "%s", guestRefusal(e, !r.exists || v.sees(before), after))
		}
	}
	if !v.guest() && !authz.Can(v.role, authz.EditContent) {
		return refuse(apperr.Forbidden, "", "Your role does not allow changes here.")
	}
	if !r.exists && !op.trusted && !op.derived && !op.serverID && !isAutomated(ctx) && !isEmailed(ctx) {
		if err := checkChosenID(ctx, tx, d.Dialect, e, r, workspaceID); err != nil {
			return err
		}
	}
	if err := s.checkRefs(ctx, q, tx, v, e, r, accepted, workspaceID); err != nil {
		return err
	}
	if e.owner != "" && r.exists && r.values[e.owner] != nil && r.values[e.owner] != v.userID {
		onlyDeleting := len(accepted) == 1 && accepted[0] == "deleted_at"
		if (!onlyDeleting && !e.adminsEdit) || !authz.Can(v.role, authz.ManageMembers) {
			if e.adminsEdit {
				return refuse(apperr.Forbidden, "", "Only the person who logged it, or an admin, can change this %s.", strings.ReplaceAll(e.name, "_", " "))
			}
			return refuse(apperr.Forbidden, "", "Only its author can change this %s.", e.name)
		}
	}
	if e.name == "page_thread" && r.exists && r.values["deleted_at"] != nil && slices.Contains(accepted, "deleted_at") && !authz.Can(v.role, authz.ManageMembers) {
		// Deleting a thread takes its comments out of sight, and comments are
		// their authors' to delete.
		var others int
		if err := tx.QueryRowContext(ctx, rebind(d.Dialect, "SELECT COUNT(*) FROM page_comments WHERE thread_id = ? AND author_id <> ? AND deleted_at IS NULL"), r.id, v.userID).Scan(&others); err != nil {
			return err
		}
		if others > 0 {
			return refuse(apperr.Forbidden, "", "This thread holds other people's comments, so only an owner or admin can delete it.")
		}
	}
	if !r.exists {
		if err := checkNew(ctx, tx, d.Dialect, e, r, after); err != nil {
			return err
		}
	}
	if e.name == "automation" {
		if err := checkAutomation(ctx, q, v, workspaceID, r, accepted); err != nil {
			return err
		}
	}

	if err := saveRow(ctx, tx, d.Dialect, e, r, accepted, now); err != nil {
		return err
	}
	if r.exists && before != after {
		if err := resetGuestsAcross(ctx, q, workspaceID, before, after); err != nil {
			return err
		}
	}
	seq, err := q.BumpChangeSeq(ctx, workspaceID)
	if err != nil {
		return err
	}
	data := changeData{Fields: map[string]any{}, Clocks: map[string]string{}}
	for _, name := range accepted {
		data.Fields[name] = jsonValue(r.values[name])
		data.Clocks[name] = r.clocks[name]
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var project sql.NullString
	if p, ok := after.(string); ok {
		project = sql.NullString{String: p, Valid: true}
	}
	if owner, ok := r.values[e.private].(string); e.private != "" && ok {
		project = sql.NullString{String: privateScope(owner), Valid: true}
	}
	*changed, *created = true, !r.exists
	if err := q.InsertChange(ctx, sqlitedb.InsertChangeParams{
		WorkspaceID: workspaceID, Seq: seq, Entity: e.name, EntityID: r.id,
		ProjectID: project, Data: string(raw), ActorID: v.userID, Now: now,
	}); err != nil {
		return err
	}
	if op.rest != nil && ValidBatch(op.Batch) {
		// What it changed from, so the batch can be undone (UndoBatch).
		was := map[string]any{}
		for _, name := range accepted {
			was[name] = jsonValue(previous[name])
		}
		before, err := json.Marshal(was)
		if err != nil {
			return err
		}
		if err := q.InsertBatchChange(ctx, sqlitedb.InsertBatchChangeParams{
			OpID: op.OpID, WorkspaceID: workspaceID, UserID: v.userID, Batch: op.Batch, Entity: e.name,
			EntityID: r.id, Made: !r.exists, Before: string(before), Hlc: op.HLC, Seq: seq, Now: now,
		}); err != nil {
			return err
		}
	}
	if err := s.recordActivity(ctx, q, e, r, news, wasDeleted, v.userID, workspaceID, now, op.derived); err != nil {
		return err
	}
	if s.watch != nil && len(news) > 0 {
		if err := s.watch.Applied(ctx, q, Applied{
			Entity: e.name, ID: r.id, WorkspaceID: workspaceID, ActorID: v.userID, Created: !r.exists,
			Accepted: news, Values: maps.Clone(r.values), Before: previous, At: now, Clock: hlcTime(op.HLC),
			Via: viaOf(ctx), Emailed: isEmailed(ctx), FromDoc: op.derived, Automated: isAutomated(ctx), Bulk: op.Bulk,
			Batch:  batchOf(op, v.userID, workspaceID, now),
			Fields: func() map[string]any { return r.snapshot(e).Fields },
		}); err != nil {
			return err
		}
	}
	return s.afterApply(ctx, q, tx, v, workspaceID, e, r, accepted, news, previous, !r.exists, op, now)
}

// checkAssignee refuses giving a task to someone who cannot reach it: a
// guest it is not shared with, or anyone who has left. Someone already
// assigned stays so when the task moves or they lose the project; this is
// only for making or bringing back an assignment.
func (s *Service) checkAssignee(ctx context.Context, q store.Queries, tx *sql.Tx, r *row, workspaceID string) error {
	const cannot = "That person cannot reach this task."
	task, err := loadRow(ctx, tx, s.st.DB().Dialect, entities["task"], r.values["task_id"].(string))
	if err != nil {
		return err
	}
	v, err := s.viewer(ctx, q, r.values["user_id"].(string), workspaceID)
	if apperr.KindOf(err) == apperr.NotFound {
		return refuse(apperr.Invalid, "user_id", cannot)
	}
	if err != nil {
		return err
	}
	if !v.sees(task.values["project_id"]) {
		return refuse(apperr.Invalid, "user_id", cannot)
	}
	return nil
}

// checkRefs makes sure every reference set by the op points at a row in
// the same workspace, that people named are members, that a task's status
// belongs to its project, and that subtasks stay with their parent.
func (s *Service) checkRefs(ctx context.Context, q store.Queries, tx *sql.Tx, v *viewer, e *entity, r *row, accepted []string, workspaceID string) error {
	d := s.st.DB()
	for _, name := range accepted {
		f := e.field(name)
		if f.kind == kMember {
			_, err := q.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: r.values[name].(string)})
			if errors.Is(err, sql.ErrNoRows) {
				return refuse(apperr.Invalid, name, "%s is not a member of this workspace.", name)
			}
			if err != nil {
				return err
			}
			continue
		}
		if f.kind != kRef || r.values[name] == nil {
			continue
		}
		target, err := loadRow(ctx, tx, d.Dialect, entities[f.ref], r.values[name].(string))
		if err != nil {
			return err
		}
		if !target.exists || target.workspaceID != workspaceID {
			return refuse(apperr.Invalid, name, "%s points at a %s that does not exist.", name, f.ref)
		}
		// To a guest, a row they cannot see does not exist either: the
		// messages after this (another project's status, a subtask's
		// parent elsewhere) would say it does.
		if v.guest() {
			scope, err := projectOf(ctx, tx, d.Dialect, entities[f.ref], target)
			if err != nil {
				return err
			}
			if !v.sees(scope) {
				return refuse(apperr.Invalid, name, "%s points at a %s that does not exist.", name, f.ref)
			}
		}
	}
	if e.name == "page" {
		if err := s.checkPage(ctx, tx, r, accepted, workspaceID); err != nil {
			return err
		}
	}
	if e.name == "page_comment" && !r.exists {
		// A thread's comments are on its page, whose project says who sees them.
		thread, err := loadRow(ctx, tx, d.Dialect, entities["page_thread"], r.values["thread_id"].(string))
		if err != nil {
			return err
		}
		if thread.values["page_id"] != r.values["page_id"] {
			return refuse(apperr.Invalid, "thread_id", "A comment must be on its thread's page.")
		}
	}
	if e.name == "template" && r.values["kind"] == "page" && slices.Contains(accepted, "data") {
		// An app from before page templates would write tasks over them.
		var data struct {
			Pages []json.RawMessage `json:"pages"`
		}
		text, _ := r.values["data"].(jsonText)
		if err := json.Unmarshal([]byte(text), &data); err != nil || len(data.Pages) == 0 {
			return refuse(apperr.Invalid, "data", "A page template must hold its pages.")
		}
	}
	if e.name == "template" && !r.exists {
		var n int
		if err := tx.QueryRowContext(ctx, rebind(d.Dialect, "SELECT COUNT(*) FROM templates WHERE workspace_id = ? AND deleted_at IS NULL"), workspaceID).Scan(&n); err != nil {
			return err
		}
		if n >= maxTemplates {
			return refuse(apperr.Invalid, "", "A workspace keeps %d templates at most. Delete some first.", maxTemplates)
		}
	}
	if e.name == "workspace_theme" && r.values["deleted_at"] == nil && (!r.exists || slices.Contains(accepted, "deleted_at")) {
		var n int
		if err := tx.QueryRowContext(ctx, rebind(d.Dialect, "SELECT COUNT(*) FROM workspace_themes WHERE workspace_id = ? AND deleted_at IS NULL AND id <> ?"), workspaceID, r.id).Scan(&n); err != nil {
			return err
		}
		if n >= maxWorkspaceThemes {
			return refuse(apperr.Invalid, "", "A workspace offers %d themes at most. Unpublish one first.", maxWorkspaceThemes)
		}
	}
	if e.name == "task_dependency" && r.values["deleted_at"] == nil && (!r.exists || slices.Contains(accepted, "deleted_at")) {
		if err := checkDependency(ctx, tx, d.Dialect, r); err != nil {
			return err
		}
	}
	if e.name == "task_assignee" && r.values["deleted_at"] == nil && (!r.exists || slices.Contains(accepted, "deleted_at")) {
		if err := s.checkAssignee(ctx, q, tx, r, workspaceID); err != nil {
			return err
		}
	}
	if e.name == "task" && r.values["status_id"] != nil {
		status, err := loadRow(ctx, tx, d.Dialect, entities["status"], r.values["status_id"].(string))
		if err != nil {
			return err
		}
		if !status.exists || status.values["project_id"] != r.values["project_id"] {
			return refuse(apperr.Invalid, "status_id", "A task's status must belong to the task's project.")
		}
	}
	if e.name == "custom_field_value" && (slices.Contains(accepted, "value") || slices.Contains(accepted, "field_id")) {
		// The value as the field's type has it; the type never changes.
		field, err := loadRow(ctx, tx, d.Dialect, entities["custom_field"], r.values["field_id"].(string))
		if err != nil {
			return err
		}
		if text, ok := r.values["value"].(jsonText); ok && field.exists {
			v, err := decodeJSON([]byte(text))
			if err != nil {
				return err
			}
			kind, _ := field.values["type"].(string)
			if err := valueShape(kind, v); err != nil {
				return refuse(apperr.Invalid, "value", "value: %v", err)
			}
		}
	}
	// Only a write that moves a task, or gives it another parent, is held
	// to its parent's project: moving a parent elsewhere leaves its
	// subtasks editable where they are until each is moved too.
	if e.name == "task" && r.values["parent_id"] != nil && (slices.Contains(accepted, "parent_id") || slices.Contains(accepted, "project_id")) {
		// Walk up from the new parent: a task cannot sit under itself, and
		// a subtask lives in its parent's project.
		parentID := r.values["parent_id"].(string)
		parent, err := loadRow(ctx, tx, d.Dialect, e, parentID)
		if err != nil {
			return err
		}
		if parent.values["project_id"] != r.values["project_id"] {
			return refuse(apperr.Invalid, "parent_id", "A subtask must be in the same project as its parent.")
		}
		for id, depth := parentID, 0; id != ""; depth++ {
			if id == r.id || depth > 50 {
				return refuse(apperr.Invalid, "parent_id", "A task cannot be its own subtask.")
			}
			up, err := loadRow(ctx, tx, d.Dialect, e, id)
			if err != nil {
				return err
			}
			id, _ = up.values["parent_id"].(string)
		}
	}
	return nil
}

var (
	datePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	clockPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// decodeJSON reads JSON as the shape checks take it, numbers as
// json.Number.
func decodeJSON(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, errors.New("must be JSON")
	}
	return v, nil
}

// decode reads one field's JSON value and checks it against the field.
func decode(f *field, raw json.RawMessage) (any, error) {
	if string(raw) == "null" {
		if !f.nullable {
			return nil, errors.New("must not be empty")
		}
		return nil, nil
	}
	switch f.kind {
	case kBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, errors.New("must be true or false")
		}
		return b, nil
	case kInt:
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil || n < 0 {
			return nil, errors.New("must be a whole number")
		}
		if f.max > 0 && n > int64(f.max) {
			return nil, fmt.Errorf("must be at most %d", f.max)
		}
		return n, nil
	case kJSON:
		// Stored as its compact JSON text, which the limit counts; the app
		// gives it meaning, and shape keeps to what the app can read.
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return nil, errors.New("must be JSON")
		}
		// Compacting keeps the bytes as sent, and Postgres refuses text
		// that isn't UTF-8 (a string field is decoded, which mends it).
		if !utf8.Valid(compact.Bytes()) {
			return nil, errors.New("must be UTF-8 text")
		}
		if f.max > 0 && compact.Len() > f.max {
			return nil, fmt.Errorf("must be at most %d bytes as compact JSON", f.max)
		}
		if f.shape != nil {
			v, err := decodeJSON(compact.Bytes())
			if err != nil {
				return nil, err
			}
			if err := f.shape(v); err != nil {
				return nil, err
			}
		}
		return jsonText(compact.String()), nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return nil, errors.New("must be a string")
	}
	switch f.kind {
	case kTime:
		t, err := time.Parse(time.RFC3339Nano, str)
		if err != nil {
			return nil, errors.New("must be an RFC 3339 time")
		}
		return t.UTC().Truncate(time.Microsecond), nil
	case kEnum:
		for _, allowed := range f.enum {
			if str == allowed {
				return str, nil
			}
		}
		return nil, fmt.Errorf("%q is not one of %v", str, f.enum)
	case kRef, kSelf, kMember:
		if !idPattern.MatchString(str) {
			return nil, errors.New("is not a valid ID")
		}
		return str, nil
	case kDate:
		if _, err := time.Parse("2006-01-02", str); err != nil || !datePattern.MatchString(str) {
			return nil, errors.New("must be a date like 2026-10-28")
		}
		return str, nil
	case kClock:
		if !clockPattern.MatchString(str) {
			return nil, errors.New("must be a time like 14:30")
		}
		return str, nil
	}
	// Postgres refuses a NUL in text, and nothing shows one; the app takes
	// them out before it sends an edit.
	if strings.ContainsRune(str, 0) {
		return nil, errors.New("must not hold a NUL character")
	}
	n := utf8.RuneCountInString(str)
	if f.kind == kString && str != trimmed(str) {
		return nil, errors.New("must not start or end with spaces")
	}
	// A title or a name goes into email as it is, where a line break, a
	// bare carriage return above all, could end the message early.
	if f.kind == kString && strings.ContainsAny(str, "\r\n") {
		return nil, errors.New("must be one line")
	}
	if n < f.min {
		return nil, errors.New("must not be empty")
	}
	if f.max > 0 && n > f.max {
		return nil, fmt.Errorf("must be at most %d characters", f.max)
	}
	if f.check != nil {
		if err := f.check(str); err != nil {
			return nil, err
		}
	}
	return str, nil
}

// lineBreaks is a run of line breaks and the spaces around them.
var lineBreaks = regexp.MustCompile(`[ \t]*[\r\n]+[ \t]*`)

// oneLine puts a one-line field written before such fields were checked
// on one line: each run of line breaks becomes a space.
func oneLine(s string) string { return trimmed(lineBreaks.ReplaceAllString(s, " ")) }

func trimmed(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// currentID is what a pair's row is called now, for the ID it had before
// pair IDs were hashes (migration 38): an edit on its way from before, or
// a REST caller that kept one.
func (s *Service) currentID(ctx context.Context, e *entity, id string) (string, error) {
	if _, ok := pairs[e.name]; !ok || ids.ShapeOf(id) != ids.Odd {
		return id, nil
	}
	now, err := s.st.GetPairID(ctx, sqlitedb.GetPairIDParams{Entity: e.name, OldID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return id, nil
	}
	return now, err
}

// checkChosenID holds the ID a device or script chose for a new row to
// what it may choose. IDs are one space for every workspace, and some are
// worked out from others (a pair's, a repeating task's next one and what
// it copies, a wiki's main page), so a row given one of those elsewhere
// would stand for good in the way of the row it belongs to. Drawn UUIDs
// and IDs that are no UUID are anyone's; one of the shape Mila works IDs
// out in must be the one it would work out for this row, here.
func checkChosenID(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row, workspaceID string) error {
	taken := refuse(apperr.Invalid, "id", "That ID is one Mila works out for something else. Choose another, or leave it out.")
	switch ids.ShapeOf(r.id) {
	case ids.Hashed:
		if _, ok := pairs[e.name]; ok {
			return nil // checkNew holds it to its pair
		}
		ok, err := followsHere(ctx, q, dialect, e, r, workspaceID)
		if err != nil || ok {
			return err
		}
		return taken
	case ids.Odd:
		if e.name == "page" {
			own := workspaceID
			if p, _ := r.values["project_id"].(string); p != "" {
				own = p
			}
			if r.id == MainPageID(own) {
				return nil
			}
		}
		return taken
	}
	return nil
}

// followsHere says whether a hashed ID is one the app works out when a
// repeating task is done, for a row in this workspace: the next task's
// (recur.NextTaskID of the task it follows), or a copy of a checklist item
// or reminder of the task it follows (recur.HashID of the next task's ID
// and the original's).
func followsHere(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row, workspaceID string) (bool, error) {
	// The task the row follows, found among the series of next.
	before := func(next, series string) (string, error) {
		if series == "" {
			return "", nil
		}
		list, err := queryIDs(ctx, q, rebind(dialect,
			"SELECT id FROM tasks WHERE workspace_id = ? AND (series_id = ? OR id = ?) AND id <> ?"), workspaceID, series, series, next)
		if err != nil {
			return "", err
		}
		for _, id := range list {
			if recur.NextTaskID(id) == next {
				return id, nil
			}
		}
		return "", nil
	}
	switch e.name {
	case "task":
		series, _ := r.values["series_id"].(string)
		prev, err := before(r.id, series)
		return prev != "", err
	case "checklist_item", "reminder":
		next, _ := r.values["task_id"].(string)
		task, err := loadRow(ctx, q, dialect, entities["task"], next)
		if err != nil || !task.exists || task.workspaceID != workspaceID {
			return false, err
		}
		series, _ := task.values["series_id"].(string)
		prev, err := before(next, series)
		if err != nil || prev == "" {
			return false, err
		}
		list, err := queryIDs(ctx, q, rebind(dialect, "SELECT id FROM "+e.table+" WHERE task_id = ?"), prev)
		if err != nil {
			return false, err
		}
		for _, id := range list {
			if recur.HashID(next+"/"+id) == r.id {
				return true, nil
			}
		}
	}
	return false, nil
}

// checkNew holds a new row to what the app makes, where a row made
// otherwise would be wrong or in someone's way: a pair's ID is made from
// the pair (PairID), so a row given another ID cannot stand in for it and
// block the real one; and a field's value is for a task in the field's
// project, unless the field is workspace-wide.
func checkNew(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row, project any) error {
	if pair, ok := pairs[e.name]; ok {
		a, _ := r.values[pair[0]].(string)
		b, _ := r.values[pair[1]].(string)
		if PairID(a, b) != r.id {
			return refuse(apperr.Invalid, "", "A %s's ID is made from its %s and %s.", e.name, pair[0], pair[1])
		}
	}
	if e.name == "custom_field_value" {
		fieldID, _ := r.values["field_id"].(string)
		field, err := loadRow(ctx, q, dialect, entities["custom_field"], fieldID)
		if err != nil {
			return err
		}
		if own := field.values["project_id"]; field.exists && own != nil && own != project {
			return refuse(apperr.Invalid, "field_id", "That field belongs to another project.")
		}
	}
	return nil
}

// fillTaskDates gives a task what reports read, as if the edit had said
// it, under the edit's own clock, so every device has it from the next
// pull: when it was made, for a new task that names no time of its own,
// and when it started, the first time it goes to a status of kind
// in-progress. A start stays once set, whatever happens to the task.
func fillTaskDates(ctx context.Context, tx *sql.Tx, dialect db.Dialect, r *row, hlc string, accepted *[]string) error {
	at := hlcTime(hlc).Truncate(time.Microsecond)
	if at.IsZero() {
		return nil
	}
	set := func(name string) {
		r.values[name] = at
		r.clocks[name] = hlc
		if !slices.Contains(*accepted, name) {
			*accepted = append(*accepted, name)
		}
	}
	if !r.exists && r.values["made_at"] == nil {
		set("made_at")
	}
	status, _ := r.values["status_id"].(string)
	if status == "" || r.values["started_at"] != nil || !slices.Contains(*accepted, "status_id") {
		return nil
	}
	var category string
	err := tx.QueryRowContext(ctx, rebind(dialect, "SELECT category FROM statuses WHERE id = ?"), status).Scan(&category)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // checkRefs says so
	}
	if err != nil {
		return err
	}
	if category == "in-progress" {
		set("started_at")
	}
	return nil
}
