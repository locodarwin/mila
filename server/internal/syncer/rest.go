package syncer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/recur"
)

// The REST endpoints for synced rows: lists,
// reads and writes of one row at a time. A write is one op through the
// same merge as a push, with the same checks, log, history and pokes;
// what differs is that the server stamps its HLC, and that it can ask
// for its row to exist or not, or to be unchanged since it was read.

// WriteMode says what a REST write does to its row.
type WriteMode int

const (
	// Create makes a new row, and fails if its ID is taken.
	Create WriteMode = iota + 1
	// Update changes a row the caller can see.
	Update
	// Upsert makes a pair's row, or brings back the one that exists.
	Upsert
	// Delete sets deleted_at, once.
	Delete
)

func (m WriteMode) String() string {
	return [...]string{"", "create", "update", "upsert", "delete"}[m]
}

// restCond is what a REST op needs of its row before it applies.
type restCond struct {
	mode    WriteMode
	ifMatch string
}

// Query picks a page of rows. Filters map a field to the value it must
// hold, "" being null.
type Query struct {
	Filters map[string]string
	// In maps an ID field to the values it may hold, for the server's own
	// reads of many tasks' rows at once (REST filters take one value). An
	// empty list keeps no row.
	In             map[string][]string
	IncludeDeleted bool
	After          string
	Limit          int
}

// Page is a page of rows in ID order. Next is the cursor for the page
// after, or "" when this one came back short.
type Page struct {
	Rows []Record
	Next string
}

// Record is a row as REST shows it: its fields and clocks, and when it
// was made and last changed.
type Record struct {
	Row
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Write is one REST write. Fields are the request's, JSON as sent; ID is
// the path's, or the body's on a create; Key is the Idempotency-Key.
type Write struct {
	Entity  string
	ID      string
	Fields  map[string]json.RawMessage
	Mode    WriteMode
	IfMatch string
	Key     string
	// Batch groups this write with others under a name of the caller's
	// (ValidBatch): rules count the batch once, and UndoBatch undoes it.
	Batch string
}

const (
	// DefaultLimit and MaxLimit bound a page of a list.
	DefaultLimit = 100
	MaxLimit     = 500
)

// ETag stands for a row's clocks: it changes when any field does, and is
// the same on every server process.
func ETag(clocks map[string]string) string {
	raw, _ := json.Marshal(clocks) // map keys are sorted
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:])[:16] + `"`
}

// ETagMatches says whether an If-Match or If-None-Match header names an
// ETag (RFC 9110 section 13.1): "*" names any, and a list names each of
// its tags. If-Match compares strongly, so a weak tag (W/"...") never
// matches; If-None-Match compares weakly, ignoring the W/.
func ETagMatches(header, etag string, weak bool) bool {
	header = strings.TrimSpace(header)
	if header == "*" {
		return true
	}
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "W/") {
			if !weak {
				continue
			}
			tag = tag[2:]
		}
		if tag == etag {
			return true
		}
	}
	return false
}

// label is an entity's name for messages: "custom field value".
func (e *entity) label() string { return strings.ReplaceAll(e.name, "_", " ") }

func noSuch(e *entity) error {
	return apperr.New(apperr.NotFound, "No such %s.", e.label())
}

func restEntity(name string) (*entity, error) {
	e := entities[name]
	if e == nil {
		return nil, apperr.New(apperr.NotFound, "No such endpoint.")
	}
	return e, nil
}

// fieldError is an Invalid error naming a field.
func fieldError(field, format string, args ...any) error {
	return &apperr.Error{Kind: apperr.Invalid, Message: fmt.Sprintf(format, args...), Field: field}
}

// scopeOf is what decides who sees a row: its project, everyone, or
// "@<userId>" for a private row.
func scopeOf(ctx context.Context, q querier, dialect db.Dialect, e *entity, r *row) (any, error) {
	scope, err := projectOf(ctx, q, dialect, e, r)
	if err != nil {
		return nil, err
	}
	if owner, ok := r.values[e.private].(string); e.private != "" && ok {
		scope = privateScope(owner)
	}
	return scope, nil
}

// record is a row read with its timestamps.
type record struct {
	*row
	created, updated time.Time
}

func (r record) out(e *entity) Record {
	return Record{Row: r.snapshot(e), CreatedAt: r.created.UTC(), UpdatedAt: r.updated.UTC()}
}

func queryRecords(ctx context.Context, q querier, dialect db.Dialect, e *entity, where string, args ...any) ([]record, error) {
	return queryColumns(ctx, q, dialect, e, restColumns(e), where, args...)
}

// queryColumns is queryRecords reading cols, which are restColumns(e) in
// order, some perhaps standing in for a column left unread.
func queryColumns(ctx context.Context, q querier, dialect db.Dialect, e *entity, cols []string, where string, args ...any) ([]record, error) {
	rows, err := q.QueryContext(ctx, rebind(dialect, "SELECT "+strings.Join(cols, ", ")+" FROM "+e.table+where), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []record
	for rows.Next() {
		var rec record
		if rec.row, err = scanRow(e, rows.Scan, &rec.created, &rec.updated); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Get returns one row the user may see, deleted or not. A row deleted for
// good, in another workspace, or out of the user's sight is NotFound.
func (s *Service) Get(ctx context.Context, userID, workspaceID, entity, id string) (*Record, error) {
	e, err := restEntity(entity)
	if err != nil {
		return nil, err
	}
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	if id, err = s.currentID(ctx, e, id); err != nil {
		return nil, err
	}
	d := s.st.DB()
	recs, err := queryRecords(ctx, d, d.Dialect, e, " WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || recs[0].workspaceID != workspaceID {
		return nil, noSuch(e)
	}
	scope, err := scopeOf(ctx, d, d.Dialect, e, recs[0].row)
	if err != nil {
		return nil, err
	}
	if !v.sees(scope) {
		return nil, noSuch(e)
	}
	out := recs[0].out(e)
	return &out, nil
}

// filterable says whether a list can be filtered on a field: references,
// people, choices, flags and dates.
func filterable(f *field) bool {
	switch f.kind {
	case kRef, kMember, kSelf, kEnum, kBool, kDate:
		return true
	}
	return false
}

// marks is n placeholders, "?, ?, ?".
func marks(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

// listWhere is the WHERE clause, and its arguments, for the rows of a
// list the viewer sees. Visibility is decided in the SQL, so every page
// is full and the cursor is exact. none means the viewer sees no row of
// the entity at all.
func listWhere(dialect db.Dialect, e *entity, v *viewer, workspaceID string, q Query) (where string, args []any, none bool, err error) {
	conds := []string{"workspace_id = ?", "id > ?"}
	args = []any{workspaceID, q.After}
	if !q.IncludeDeleted {
		conds = append(conds, "deleted_at IS NULL")
	}
	if e.private != "" {
		if e.field(e.private).nullable {
			conds = append(conds, "("+e.private+" = ? OR "+e.private+" IS NULL)")
		} else {
			conds = append(conds, e.private+" = ?")
		}
		args = append(args, v.userID)
	}
	var projects []any
	if v.guest() {
		for _, p := range slices.Sorted(maps.Keys(v.projects)) {
			projects = append(projects, p)
		}
	}
	switch {
	case e.parent != "":
		// The parent is a task, or a page for a page's files. Each row's
		// parent is looked up by its key: a list of every parent in the
		// workspace (an IN of a subquery) was made again for every page,
		// so reading all of a large workspace's rows took the square of
		// its size.
		sub := "SELECT 1 FROM " + entities[e.parentOf()].table + " parent WHERE parent.id = " + e.table + "." + e.parent + " AND parent.workspace_id = ?"
		subArgs := []any{workspaceID}
		if !q.IncludeDeleted {
			// As Rows leaves them out: what hangs off a deleted task or
			// page is in the trash with it.
			sub += " AND parent.deleted_at IS NULL"
		}
		if v.guest() {
			if len(projects) == 0 {
				return "", nil, true, nil
			}
			sub += " AND parent.project_id IN (" + marks(len(projects)) + ")"
			subArgs = append(subArgs, projects...)
		}
		if v.guest() || !q.IncludeDeleted {
			conds = append(conds, "EXISTS ("+sub+")")
			args = append(args, subArgs...)
		}
		if e.alsoIn != "" && v.guest() {
			// A row in two tasks (a dependency) is seen only by those who
			// see both, as bothScope has it.
			conds = append(conds, "EXISTS (SELECT 1 FROM tasks other WHERE other.id = "+e.table+"."+e.alsoIn+
				" AND other.workspace_id = ? AND other.project_id IN ("+marks(len(projects))+"))")
			args = append(append(args, workspaceID), projects...)
		}
	case e.membersOnly && v.guest():
		return "", nil, true, nil
	case !v.guest() || e.workspaceWide:
	case e.wideWhenNull && len(projects) == 0:
		conds = append(conds, e.projectOf+" IS NULL")
	case e.wideWhenNull:
		conds = append(conds, "("+e.projectOf+" IS NULL OR "+e.projectOf+" IN ("+marks(len(projects))+"))")
		args = append(args, projects...)
	case len(projects) == 0:
		return "", nil, true, nil
	default:
		conds = append(conds, e.projectOf+" IN ("+marks(len(projects))+")")
		args = append(args, projects...)
	}
	for _, name := range slices.Sorted(maps.Keys(q.Filters)) {
		value := q.Filters[name]
		f := e.field(name)
		if f == nil || !filterable(f) {
			return "", nil, false, fieldError(name, "A list of %s rows cannot be filtered on %q.", e.label(), name)
		}
		if value == "" {
			if !f.nullable {
				return "", nil, false, fieldError(name, "%s: must not be empty", name)
			}
			conds = append(conds, name+" IS NULL")
			continue
		}
		raw := json.RawMessage(strconv.Quote(value))
		if f.kind == kBool {
			raw = json.RawMessage(value)
		}
		val, err := decode(f, raw)
		if err != nil {
			return "", nil, false, fieldError(name, "%s: %v", name, err)
		}
		conds = append(conds, name+" = ?")
		args = append(args, val)
	}
	for _, name := range slices.Sorted(maps.Keys(q.In)) {
		values := q.In[name]
		if f := e.field(name); name != "id" && (f == nil || (f.kind != kRef && f.kind != kMember && f.kind != kSelf)) {
			return "", nil, false, fieldError(name, "A list of %s rows cannot be read by %q.", e.label(), name)
		}
		if len(values) == 0 {
			return "", nil, true, nil
		}
		if dialect == db.Postgres {
			// One array, as a plan Postgres keeps for a list of so many
			// values reads the whole workspace's rows.
			conds = append(conds, name+" = ANY(?)")
			args = append(args, values)
			continue
		}
		conds = append(conds, name+" IN ("+marks(len(values))+")")
		for _, v := range values {
			args = append(args, v)
		}
	}
	return " WHERE " + strings.Join(conds, " AND "), args, false, nil
}

// List returns a page of the rows the user may see, in ID order.
func (s *Service) List(ctx context.Context, userID, workspaceID, entity string, q Query) (Page, error) {
	e, err := restEntity(entity)
	if err != nil {
		return Page{}, err
	}
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		return Page{}, fieldError("limit", "limit must be from 1 to %d.", MaxLimit)
	}
	if q.After != "" && !idPattern.MatchString(q.After) {
		return Page{}, fieldError("after", "after must be the ID of the last row of the page before.")
	}
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return Page{}, err
	}
	d := s.st.DB()
	where, args, none, err := listWhere(d.Dialect, e, v, workspaceID, q)
	if err != nil || none {
		return Page{Rows: []Record{}}, err
	}
	recs, err := queryRecords(ctx, d, d.Dialect, e, where+" ORDER BY id LIMIT ?", append(args, q.Limit)...)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.seen(ctx, e, v, recs)
	if err != nil {
		return Page{}, err
	}
	page := Page{Rows: rows}
	if len(recs) == q.Limit {
		page.Next = recs[len(recs)-1].id
	}
	return page, nil
}

// seen is the rows of recs the viewer sees, as REST shows them. The SQL
// decided who sees what; this checks it again, row by row.
func (s *Service) seen(ctx context.Context, e *entity, v *viewer, recs []record) ([]Record, error) {
	d := s.st.DB()
	// projects reads the projects of the rows of a table that recs name in
	// a field.
	projects := func(table, field string) (map[string]any, error) {
		out := map[string]any{}
		var ids []any
		for _, r := range recs {
			ids = append(ids, r.values[field])
		}
		if len(ids) == 0 {
			return out, nil
		}
		rows, err := d.QueryContext(ctx, rebind(d.Dialect, "SELECT id, project_id FROM "+table+" WHERE id IN ("+marks(len(ids))+")"), ids...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			var project sql.NullString
			if err := rows.Scan(&id, &project); err != nil {
				return nil, err
			}
			out[id] = nil
			if project.Valid {
				out[id] = project.String
			}
		}
		return out, rows.Err()
	}
	var taskProject, otherProject map[string]any
	if e.parent != "" {
		var err error
		if taskProject, err = projects(entities[e.parentOf()].table, e.parent); err != nil {
			return nil, err
		}
		if e.alsoIn != "" {
			if otherProject, err = projects(entities["task"].table, e.alsoIn); err != nil {
				return nil, err
			}
		}
	}
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		var scope any
		switch {
		case e.workspaceWide, e.wideWhenNull && r.values[e.projectOf] == nil:
			scope = everyone
		case e.parent != "" && e.alsoIn != "":
			other, _ := r.values[e.alsoIn].(string)
			scope = bothScope(taskProject[r.values[e.parent].(string)], otherProject[other])
		case e.parent != "":
			scope = taskProject[r.values[e.parent].(string)]
		case e.projectOf == "id":
			scope = r.id
		default:
			scope = r.values[e.projectOf]
		}
		if owner, ok := r.values[e.private].(string); e.private != "" && ok {
			scope = privateScope(owner)
		}
		if !v.sees(scope) {
			// The SQL let it through: it moved between the two reads, or the
			// SQL is wrong. Either way it is left out, and said in the log.
			s.log.Warn("a REST list left out a row its query let through", "entity", e.name, "row", r.id, "user", v.userID)
			continue
		}
		out = append(out, r.out(e))
	}
	return out, nil
}

// Write makes, changes or deletes one row, through the merge. It returns
// the row as it stands afterward, and whether this write made it.
func (s *Service) Write(ctx context.Context, userID, workspaceID string, w Write) (*Record, bool, error) {
	e, err := restEntity(w.Entity)
	if err != nil {
		return nil, false, err
	}
	v, err := s.viewer(ctx, s.st, userID, workspaceID)
	if err != nil {
		return nil, false, err
	}
	fields := maps.Clone(w.Fields)
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	creating := w.Mode == Create || w.Mode == Upsert
	me, _ := json.Marshal(userID)
	// Fields that name their maker can only name the caller. Checked
	// before a pair's ID is made from one, so naming someone else learns
	// nothing about their rows.
	for _, f := range e.fields {
		raw, given := fields[f.name]
		if f.kind != kSelf || !given {
			continue
		}
		var who *string
		if json.Unmarshal(raw, &who) != nil || who != nil && *who != userID {
			return nil, false, &apperr.Error{Kind: apperr.Forbidden, Message: f.name + " must be you.", Field: f.name}
		}
	}
	if e.name == "automation" {
		// A rule runs as whoever last wrote what it does.
		if _, given := fields["rule"]; given {
			fields["made_by"] = me
		}
		// It acts on due dates from when it was turned on.
		var on bool
		if _, stamped := fields["enabled_at"]; !stamped && json.Unmarshal(fields["enabled"], &on) == nil && on {
			fields["enabled_at"], _ = json.Marshal(s.now().UTC().Format(time.RFC3339Nano))
		}
	}
	if creating {
		// Whoever makes a row is its author, assignee of their own My Day,
		// or its maker; a saved view stays shared unless it names its owner.
		for _, f := range e.fields {
			if _, given := fields[f.name]; f.kind == kSelf && !given && (f.required || f.name == "created_by") {
				fields[f.name] = me
			}
		}
	}
	id := w.ID
	pair, isPair := pairs[e.name]
	serverID := false
	switch {
	case !creating:
		if !idPattern.MatchString(id) {
			return nil, false, noSuch(e)
		}
	case isPair:
		var ab [2]string
		for i, name := range pair {
			raw, ok := fields[name]
			if !ok {
				return nil, false, fieldError(name, "A new %s needs %s.", e.label(), name)
			}
			if json.Unmarshal(raw, &ab[i]) != nil {
				return nil, false, fieldError(name, "%s: must be a string", name)
			}
		}
		derived := PairID(ab[0], ab[1])
		if derived == "" {
			return nil, false, fieldError(pair[0], "%s and %s must be UUIDs.", pair[0], pair[1])
		}
		if id != "" && id != derived {
			return nil, false, fieldError("id", "A %s's id is made from its %s and %s (%s); leave it out.", e.label(), pair[0], pair[1], derived)
		}
		id = derived
	case id != "":
		if !idPattern.MatchString(id) {
			return nil, false, fieldError("id", "id: is not a valid ID")
		}
	case w.Key != "":
		// A retry of the same create names the same row, so it finds its
		// op already applied rather than making a second row.
		id = recur.HashID("rest\n" + userID + "\n" + workspaceID + "\n" + e.name + "\n" + w.Key)
		serverID = true
	default:
		id = uuid.NewString()
		serverID = true
	}
	opID := uuid.NewString()
	if w.Key != "" {
		// Scoped to its user and target: applied_ops is shared by every
		// workspace.
		sum := sha256.Sum256([]byte(strings.Join([]string{userID, workspaceID, w.Mode.String(), e.name, id, w.Key}, "\n")))
		opID = "rest-" + hex.EncodeToString(sum[:])[:40]
	}
	if w.Mode == Delete {
		at, _ := json.Marshal(s.now().UTC().Format(time.RFC3339Nano))
		fields = map[string]json.RawMessage{"deleted_at": at}
	}

	op := Op{OpID: opID, Entity: e.name, ID: id, Fields: fields, rest: &restCond{mode: w.Mode, ifMatch: w.IfMatch}, serverID: serverID, Batch: w.Batch}
	res, changed, err := s.apply(ctx, v, workspaceID, op)
	if w.Mode == Upsert && db.IsUniqueViolation(err) {
		// Another request made the pair's row between this one's read and
		// its insert (on Postgres, where a lock on a missing row locks
		// nothing). Now it exists, and this one brings it back.
		res, changed, err = s.apply(ctx, v, workspaceID, op)
	}
	if db.IsUniqueViolation(err) {
		return nil, false, apperr.New(apperr.Conflict, "A %s with that ID already exists.", e.label())
	}
	if err != nil {
		return nil, false, err
	}
	if changed {
		if seq, err := s.st.GetChangeSeq(ctx, workspaceID); err == nil && s.notify != nil {
			s.notify.Changed(workspaceID, seq)
		}
		if s.watch != nil {
			s.watch.Committed()
		}
	}
	if res.Status == "rejected" {
		if res.reason == nil {
			return nil, false, apperr.New(apperr.Invalid, "%s", res.Message)
		}
		return nil, false, &apperr.Error{Kind: res.reason.kind, Message: res.reason.msg, Field: res.reason.field}
	}
	d := s.st.DB()
	recs, err := queryRecords(ctx, d, d.Dialect, e, " WHERE id = ?", id)
	if err != nil {
		return nil, false, err
	}
	if len(recs) == 0 || recs[0].workspaceID != workspaceID {
		return nil, false, noSuch(e)
	}
	// A retry answered from the applied op never reached the checks: a
	// guest who has since lost the project must not read the row now.
	scope, err := scopeOf(ctx, d, d.Dialect, e, recs[0].row)
	if err != nil {
		return nil, false, err
	}
	if !v.sees(scope) {
		return nil, false, noSuch(e)
	}
	out := recs[0].out(e)
	return &out, res.created, nil
}

// prepareREST runs inside a REST op's transaction, after its row is
// loaded (and locked): it checks the op's condition, gives a new row
// every field it was not given, and stamps the op.
func (s *Service) prepareREST(ctx context.Context, tx *sql.Tx, dialect db.Dialect, v *viewer, e *entity, r *row, op *Op, workspaceID string, now time.Time) error {
	c := op.rest
	visible := false
	if r.exists && r.workspaceID == workspaceID {
		scope, err := scopeOf(ctx, tx, dialect, e, r)
		if err != nil {
			return err
		}
		visible = v.sees(scope)
	}
	switch {
	case c.mode == Create && r.exists:
		// Whichever workspace has it.
		return refuse(apperr.Conflict, "id", "A %s with that ID already exists.", e.label())
	case (c.mode == Update || c.mode == Delete) && !visible, c.mode == Upsert && r.exists && !visible:
		// Missing, elsewhere and out of sight look the same.
		return refuse(apperr.NotFound, "", "No such %s.", e.label())
	case c.ifMatch != "" && r.exists && !ETagMatches(c.ifMatch, ETag(r.clocks), false):
		return refuse(apperr.PreconditionFailed, "", "This %s changed since that version was read; get it again and retry.", e.label())
	}
	fields := maps.Clone(op.Fields)
	if c.mode == Delete && r.values["deleted_at"] != nil {
		// Deleted already: it keeps the time it was deleted, but this
		// delete is the newest word on it, over a restore made offline
		// before it.
		fields["deleted_at"], _ = json.Marshal(jsonValue(r.values["deleted_at"]))
	}
	if body, given := fields["body"]; (e.name == "comment" || e.name == "page_comment") && r.exists && given {
		// As the app does: a comment whose words change is marked edited.
		if _, marked := fields["edited_at"]; !marked {
			if val, err := decode(e.field("body"), body); err == nil && !sameValue(r.values["body"], val) {
				fields["edited_at"], _ = json.Marshal(now.UTC().Format(time.RFC3339Nano))
			}
		}
	}
	if !r.exists {
		// A row made through REST carries every field, so every device
		// holds all of it from the change log alone.
		for _, f := range e.fields {
			if _, given := fields[f.name]; given || f.serverSet {
				continue
			}
			if f.name == "sort_key" {
				key, err := nextSortKey(ctx, tx, dialect, e, workspaceID)
				if err != nil {
					return err
				}
				fields[f.name], _ = json.Marshal(key)
				continue
			}
			if def, ok := defaultOf(&f); ok {
				fields[f.name], _ = json.Marshal(def)
			}
		}
	} else if _, given := fields["deleted_at"]; c.mode == Upsert && !given {
		// Adding a pair again brings it back, and changes nothing else.
		fields["deleted_at"] = json.RawMessage("null")
	}
	op.Fields = fields
	op.HLC = stampAfter(now, r.clocks, slices.Collect(maps.Keys(fields)))
	return nil
}

// defaultOf is the value a new row gets for a field not given: its def,
// or null for a nullable one. Required fields have none.
func defaultOf(f *field) (any, bool) {
	switch {
	case f.required:
		return nil, false
	case f.def != nil:
		return f.def, true
	case f.nullable:
		return nil, true
	}
	return nil, false
}

// nextSortKey is a key after every valid key of the entity in the
// workspace, so a new row lands last in whatever list it joins. Keys are
// checked on every write, but a row from before that may hold one the
// server cannot follow, which is passed over.
func nextSortKey(ctx context.Context, tx *sql.Tx, dialect db.Dialect, e *entity, workspaceID string) (string, error) {
	// Keys compare byte by byte, as in the app; Postgres would otherwise
	// order them by the database's locale.
	col := "sort_key"
	if dialect == db.Postgres {
		col = `sort_key COLLATE "C"`
	}
	var top sql.NullString
	if err := tx.QueryRowContext(ctx, rebind(dialect, "SELECT MAX("+col+") FROM "+e.table+" WHERE workspace_id = ?"), workspaceID).Scan(&top); err != nil {
		return "", err
	}
	last := top.String
	if last != "" && validateKey(last) != nil {
		var err error
		if last, err = lastValidKey(ctx, tx, dialect, col, e, workspaceID); err != nil {
			return "", err
		}
	}
	key, err := keyAfter(last)
	if err != nil {
		return "", refuse(apperr.Invalid, "sort_key", "Give sort_key; this list holds a key the server cannot follow.")
	}
	return key, nil
}

// lastValidKey is the largest key of the entity in the workspace that the
// server can follow, or "" for none: the rare case after MAX gave one it
// cannot.
func lastValidKey(ctx context.Context, tx *sql.Tx, dialect db.Dialect, col string, e *entity, workspaceID string) (string, error) {
	rows, err := tx.QueryContext(ctx, rebind(dialect, "SELECT sort_key FROM "+e.table+" WHERE workspace_id = ? ORDER BY "+col+" DESC"), workspaceID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return "", err
		}
		if validateKey(key) == nil {
			return key, nil
		}
	}
	return "", rows.Err()
}

// restNode is the node of the HLCs REST writes are stamped with.
const restNode = "api"

// stampAfter is a REST op's HLC: now, or just after the newest clock of
// the fields it writes, whichever is later. A write then never loses to
// one the server already holds (from a device up to maxClockSkew ahead),
// and a skewed clock on one field does not push the rest of the row's
// stamps ahead. The row lock orders two writes in one millisecond: the
// second observes the first.
func stampAfter(now time.Time, clocks map[string]string, fields []string) string {
	ms, counter := now.UnixMilli(), int64(0)
	for _, name := range fields {
		m := hlcPattern.FindStringSubmatch(clocks[name])
		if m == nil {
			continue
		}
		cms, _ := strconv.ParseInt(m[1], 10, 64)
		cc, _ := strconv.ParseInt(m[2], 16, 64)
		if cms > ms || cms == ms && cc >= counter {
			ms, counter = cms, cc+1
		}
	}
	if counter > 0xffff {
		ms, counter = ms+1, 0
	}
	return fmt.Sprintf("%013d-%04x-%s", ms, counter, restNode)
}

// sameValue says whether a decoded value equals a stored one.
func sameValue(stored, val any) bool {
	switch x := stored.(type) {
	case time.Time:
		y, ok := val.(time.Time)
		return ok && x.Equal(y)
	case jsonText:
		y, ok := val.(jsonText)
		return ok && x == y
	}
	return stored == val
}
