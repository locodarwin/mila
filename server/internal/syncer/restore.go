package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/ids"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// RestoreOptions says how the rows of an export become a new workspace's.
type RestoreOptions struct {
	// FreshIDs gives every row a new ID. Otherwise rows keep theirs, and
	// the restore is refused if any of them is taken on this instance.
	FreshIDs bool
	// Owner is who the workspace is restored for. Exporter, who made the
	// export, becomes them: it was their copy of the workspace.
	Owner, Exporter string
	// Keep reports whether work the export credits to someone else may
	// stay theirs; anyone else's work is credited to Owner.
	Keep func(ctx context.Context, userID string) (bool, error)
	// From is the ID of the workspace the export was made from. The
	// addresses a task's notes and a page's body hold (links to pages and
	// tasks, images of attached files) name it and the rows' IDs, which
	// are rewritten to the new ones.
	From string
	// Versions are the pages' history.
	Versions []PageVersionEntry
}

// RestoreReport says what a restore made of an export.
type RestoreReport struct {
	// Restored and Dropped count rows by entity: those written, and those
	// left out (not valid, pointing at something missing, or private to
	// someone else).
	Restored map[string]int `json:"restored"`
	Dropped  map[string]int `json:"dropped"`
	// Credited counts rows by other people, now the owner's.
	Credited int `json:"credited"`
	// Activity counts the history entries restored.
	Activity int `json:"activity"`
	// Versions counts the versions of pages restored.
	Versions int `json:"versions"`
}

// MaxRestoreRows bounds what one restore takes in.
const MaxRestoreRows = 2_000_000

// restoreChunk is how many rows one transaction writes, so a large
// restore never holds the database for long.
const restoreChunk = 500

// Restore writes the rows of an export (and its history) into a workspace
// made for it, which nobody can see until the caller shows it. Every
// field is checked as an op's would be; rows that do not fit are left out
// and counted rather than failing the whole restore.
func (s *Service) Restore(ctx context.Context, workspaceID string, rows []Row, history []HistoryEntry, opts RestoreOptions) (RestoreReport, error) {
	rep := RestoreReport{Restored: map[string]int{}, Dropped: map[string]int{}}
	if len(rows) > MaxRestoreRows {
		return rep, apperr.New(apperr.Invalid, "The export holds more than %d rows, more than one import takes.", MaxRestoreRows)
	}
	now := s.now()
	nowHLC := serverHLC(now)
	drop := func(entity string) { rep.Dropped[entity]++ }

	// Read and check every row, by entity in sync order.
	byEntity := map[string][]*restoring{}
	for _, in := range rows {
		e := entities[in.Entity]
		if e == nil {
			drop("unknown")
			continue
		}
		// A live document's rows are made again from its Markdown (SeedDocs
		// below), which holds the addresses rewritten for the new IDs.
		if e.internal {
			continue
		}
		r, ok := checkRow(e, in, now, nowHLC)
		if !ok {
			drop(e.name)
			continue
		}
		byEntity[e.name] = append(byEntity[e.name], r)
	}

	// People: the exporter becomes the owner; private rows of anyone else
	// and assignments of anyone but the owner go (only the owner is a
	// member); other work is the owner's unless Keep says otherwise.
	keep := map[string]bool{}
	author := func(id string) (string, error) {
		if id == opts.Exporter || id == opts.Owner {
			return opts.Owner, nil
		}
		v, ok := keep[id]
		if !ok && opts.Keep != nil {
			var err error
			if v, err = opts.Keep(ctx, id); err != nil {
				return "", err
			}
			keep[id] = v
		}
		if v {
			return id, nil
		}
		return opts.Owner, nil
	}
	for _, name := range syncOrder {
		e := entities[name]
		kept := byEntity[name][:0]
	rows:
		for _, r := range byEntity[name] {
			credited := false
			for _, f := range e.fields {
				u, ok := r.values[f.name].(string)
				if !ok || (f.kind != kSelf && f.kind != kMember) {
					continue
				}
				if u == opts.Exporter {
					u = opts.Owner
				}
				switch {
				case u == opts.Owner:
				case f.name == e.private || f.kind == kMember:
					drop(name)
					continue rows
				default:
					who, err := author(u)
					if err != nil {
						return rep, err
					}
					credited = credited || who != u
					u = who
				}
				r.values[f.name] = u
			}
			if credited {
				rep.Credited++
			}
			kept = append(kept, r)
		}
		byEntity[name] = kept
	}

	// A wiki's main page is the one page without a parent, named after
	// its project or workspace, which may have new IDs here (the
	// workspace's always does). A second one in the same wiki goes.
	mains := map[string]string{}
	seen := map[string]bool{}
	pages := byEntity["page"][:0]
	for _, r := range byEntity["page"] {
		if parent, _ := r.values["parent_id"].(string); parent == "" {
			scope, _ := r.values["project_id"].(string)
			if seen[scope] {
				drop("page")
				continue
			}
			seen[scope] = true
			mains[r.id] = scope
		}
		pages = append(pages, r)
	}
	byEntity["page"] = pages

	ids := newIDMap(opts.FreshIDs, byEntity)
	ids.mains, ids.workspace = mains, workspaceID
	if !opts.FreshIDs {
		ids.unexplained(byEntity)
	}
	if opts.Exporter != "" && opts.Exporter != opts.Owner {
		ids.people[opts.Exporter] = opts.Owner
	}
	// Every row's new ID first, so references to rows later in the order
	// (inside a view's filter, say) find theirs.
	for _, name := range syncOrder {
		for _, r := range byEntity[name] {
			if !r.paired {
				ids.of(r.id)
			}
		}
	}
	for _, name := range syncOrder {
		for _, r := range byEntity[name] {
			ids.rewrite(entities[name], r)
		}
	}
	for _, k := range docKinds {
		for _, r := range byEntity[k.target] {
			if text, ok := r.values[k.field].(string); ok {
				r.values[k.field] = ids.text(text, opts.From, workspaceID)
			}
		}
	}
	// A comment's references are addresses like a note's links. Its
	// mentions stay as written: they name people, who are not rows here.
	for _, name := range []string{"comment", "page_comment"} {
		for _, r := range byEntity[name] {
			if text, ok := r.values["body"].(string); ok {
				r.values["body"] = ids.text(text, opts.From, workspaceID)
			}
		}
	}
	// A pin is a place in the page's document as it was, and an import makes
	// the document again: the thread finds its place by its words instead.
	for _, r := range byEntity["page_thread"] {
		r.values["anchor"] = ""
	}
	if !opts.FreshIDs {
		if err := s.checkFree(ctx, byEntity); err != nil {
			return rep, err
		}
	}

	ordered, err := settle(byEntity, drop)
	if err != nil {
		return rep, err
	}

	d := s.st.DB()
	for start := 0; start < len(ordered); start += restoreChunk {
		chunk := ordered[start:min(start+restoreChunk, len(ordered))]
		err := s.st.InTxConn(ctx, func(_ store.Queries, tx *sql.Tx) error {
			for _, r := range chunk {
				r.workspaceID = workspaceID
				if err := saveRow(ctx, tx, d.Dialect, r.e, r.row, r.given, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return rep, err
		}
		for _, r := range chunk {
			rep.Restored[r.e.name]++
		}
	}

	// History, for the tasks that came in.
	tasks := map[string]bool{}
	for _, r := range byEntity["task"] {
		tasks[r.id] = true
	}
	var entries []HistoryEntry
	for _, h := range history {
		h.TaskID = ids.of(h.TaskID)
		if !tasks[h.TaskID] || !idPattern.MatchString(h.EntityID) || entities[h.Entity] == nil {
			continue
		}
		h.EntityID = ids.of(h.EntityID)
		switch h.Kind {
		case "created", "changed", "deleted", "restored":
		default:
			continue
		}
		// The actor, and the people a row's history names as its author,
		// are credited as the row was.
		if !idPattern.MatchString(h.ActorID) {
			h.ActorID = opts.Owner
		} else if who, err := author(h.ActorID); err != nil {
			return rep, err
		} else {
			h.ActorID = who
		}
		for _, f := range entities[h.Entity].fields {
			if u, ok := h.Fields[f.name].(string); ok && f.kind == kSelf {
				who, err := author(u)
				if err != nil {
					return rep, err
				}
				h.Fields[f.name] = who
			}
		}
		entries = append(entries, h)
	}
	for start := 0; start < len(entries); start += restoreChunk {
		chunk := entries[start:min(start+restoreChunk, len(entries))]
		err := s.st.InTx(ctx, func(q store.Queries) error {
			for _, h := range chunk {
				scrub(h.Entity, h.Fields)
				fields := ids.json(h.Fields)
				data, err := json.Marshal(fields)
				if err != nil {
					return err
				}
				at := h.At.UTC().Truncate(time.Microsecond)
				if at.IsZero() || at.After(now) {
					at = now
				}
				if err := q.InsertActivity(ctx, sqlitedb.InsertActivityParams{
					ID: uuid.NewString(), WorkspaceID: workspaceID, TaskID: h.TaskID, ActorID: h.ActorID,
					Entity: h.Entity, EntityID: h.EntityID, Kind: h.Kind, Data: string(data), Now: at,
				}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return rep, err
		}
		rep.Activity += len(chunk)
	}
	// The pages' history, for the pages that came in.
	restoredPages := map[string]bool{}
	for _, r := range ordered {
		if r.e.name == "page" {
			restoredPages[r.id] = true
		}
	}
	var versions []PageVersionEntry
	for _, pv := range opts.Versions {
		pv.PageID = ids.of(pv.PageID)
		if !restoredPages[pv.PageID] {
			continue
		}
		switch {
		case pv.ActorID == "":
		case !idPattern.MatchString(pv.ActorID):
			pv.ActorID = opts.Owner
		default:
			who, err := author(pv.ActorID)
			if err != nil {
				return rep, err
			}
			pv.ActorID = who
		}
		pv.Body = ids.text(pv.Body, opts.From, workspaceID)
		versions = append(versions, pv)
	}
	for start := 0; start < len(versions); start += restoreChunk {
		chunk := versions[start:min(start+restoreChunk, len(versions))]
		err := s.st.InTx(ctx, func(q store.Queries) error {
			for _, pv := range chunk {
				at := pv.At.UTC().Truncate(time.Microsecond)
				if at.IsZero() || at.After(now) {
					at = now
				}
				if err := q.InsertPageVersion(ctx, sqlitedb.InsertPageVersionParams{
					ID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: workspaceID, PageID: pv.PageID,
					ActorID: sql.NullString{String: pv.ActorID, Valid: pv.ActorID != ""}, Body: pv.Body, Now: at,
				}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return rep, err
		}
		rep.Versions += len(chunk)
	}
	pruned := map[string]bool{}
	for _, pv := range versions {
		if pruned[pv.PageID] {
			continue
		}
		pruned[pv.PageID] = true
		if err := s.st.InTx(ctx, func(q store.Queries) error { return pruneVersions(ctx, q, pv.PageID) }); err != nil {
			return rep, err
		}
	}
	// Tasks from an export made before they kept when they were made and
	// started get both from the history that came with them.
	if err := s.st.InTxConn(ctx, func(_ store.Queries, tx *sql.Tx) error {
		return db.FillTaskDates(ctx, tx, s.st.DB().Dialect, workspaceID)
	}); err != nil {
		return rep, err
	}
	// Documents are made from the Markdown restored.
	if _, err := s.SeedDocs(ctx, workspaceID); err != nil {
		return rep, err
	}
	return rep, nil
}

// restoring is a row on its way into a restored workspace.
type restoring struct {
	*row
	e *entity
	// given lists the fields the export had, in entity order; the rest
	// take their columns' defaults.
	given []string
	// paired is true for a row named by its pair, as the app names it.
	paired bool
}

// checkRow reads one exported row, checking each field as an op's field
// is checked. Server-only fields are allowed: an export is the server's
// own record. A row missing a required field, or with a field that does
// not pass, is refused.
func checkRow(e *entity, in Row, now time.Time, nowHLC string) (*restoring, bool) {
	if !idPattern.MatchString(in.ID) {
		return nil, false
	}
	r := &restoring{row: &row{id: in.ID, values: map[string]any{}, clocks: map[string]string{}}, e: e}
	for _, f := range e.fields {
		v, given := in.Fields[f.name]
		if !given {
			if f.required {
				return nil, false
			}
			continue
		}
		// Exported before one-line fields were held to one line, or before
		// text was kept free of NULs (on SQLite, which takes them).
		if s, ok := v.(string); ok && (f.kind == kString || f.kind == kText) {
			s = strings.ReplaceAll(s, "\x00", "")
			if f.kind == kString {
				s = oneLine(s)
			}
			v = s
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		val, err := decode(&f, raw)
		if err != nil {
			return nil, false
		}
		clock := in.Clocks[f.name]
		if checkHLC(clock, now) != nil {
			clock = nowHLC
		}
		r.values[f.name] = val
		r.clocks[f.name] = clock
		r.given = append(r.given, f.name)
	}
	if pair, ok := pairs[e.name]; ok {
		a, _ := r.values[pair[0]].(string)
		b, _ := r.values[pair[1]].(string)
		// Made either way: a pair's IDs hashed, or before that XORed.
		r.paired = PairID(a, b) != "" && (PairID(a, b) == r.id || LegacyPairID(a, b) == r.id)
	}
	return r, true
}

// idMap gives the rows of a restore their IDs: their own, or fresh ones
// that keep the IDs made from others (the next task of a series, a pair's
// row, copies of a repeating task's checklist and reminders) made the way
// the app makes them, so later edits from devices meet the same rows.
type idMap struct {
	fresh bool
	// people are users who become someone else (the exporter, the owner).
	people map[string]string
	// mains are wikis' main pages, by their old IDs: the workspace's
	// (scope "") or a project's. They are named after the new ones.
	mains     map[string]string
	workspace string
	memo      map[string]string
	derive    map[string]func() string
	busy      map[string]bool
}

func newIDMap(fresh bool, byEntity map[string][]*restoring) *idMap {
	m := &idMap{fresh: fresh, people: map[string]string{}, memo: map[string]string{}, derive: map[string]func() string{}, busy: map[string]bool{}}
	if !fresh {
		return m
	}
	for _, t := range byEntity["task"] {
		prev := t.id
		m.derive[recur.NextTaskID(prev)] = func() string { return recur.NextTaskID(m.of(prev)) }
	}
	// A repeating task's next copy brings its checklist and the reminders
	// under IDs made from the copy and the item it copies.
	for _, name := range []string{"checklist_item", "reminder"} {
		for _, item := range byEntity[name] {
			task, _ := item.values["task_id"].(string)
			next, from := recur.NextTaskID(task), item.id
			m.derive[recur.HashID(next+"/"+from)] = func() string { return recur.HashID(m.of(next) + "/" + m.of(from)) }
		}
	}
	return m
}

// unexplained gives a fresh ID, where rows keep theirs, to each row whose ID
// has the shape of one Mila works out but which nothing in the export
// works out: an export can be anyone's making, and a row under the ID of
// another workspace's next task or main page would stand in its way for
// good (checkChosenID holds pushes to the same). A row from a REST create
// that named a key, say, merely gets another.
func (m *idMap) unexplained(byEntity map[string][]*restoring) {
	worked := map[string]bool{}
	for _, t := range byEntity["task"] {
		worked[recur.NextTaskID(t.id)] = true
	}
	for _, name := range []string{"checklist_item", "reminder"} {
		for _, item := range byEntity[name] {
			task, _ := item.values["task_id"].(string)
			worked[recur.HashID(recur.NextTaskID(task)+"/"+item.id)] = true
		}
	}
	for _, name := range syncOrder {
		for _, r := range byEntity[name] {
			if _, main := m.mains[r.id]; r.paired || main {
				continue
			}
			switch ids.ShapeOf(r.id) {
			case ids.Hashed:
				if !worked[r.id] {
					m.memo[r.id] = uuid.NewString()
				}
			case ids.Odd:
				m.memo[r.id] = uuid.NewString()
			}
		}
	}
}

// of is the new ID for an old one; the same old ID always gets the same.
func (m *idMap) of(old string) string {
	if old == "" {
		return old
	}
	if id, ok := m.memo[old]; ok {
		return id
	}
	if scope, ok := m.mains[old]; ok {
		id := MainPageID(m.workspace)
		if scope != "" {
			id = MainPageID(m.of(scope))
		}
		m.memo[old] = id
		return id
	}
	if !m.fresh {
		return old
	}
	id := ""
	if f, ok := m.derive[old]; ok && !m.busy[old] {
		m.busy[old] = true
		id = f()
		delete(m.busy, old)
	}
	if id == "" {
		id = uuid.NewString()
	}
	m.memo[old] = id
	return id
}

// rewrite gives a row its new ID and points its references at the new
// IDs, including IDs held inside JSON (a view's filter). A pair's row is
// named by its pair again, which also follows a person who changed.
func (m *idMap) rewrite(e *entity, r *restoring) {
	// An imported rule waits to be looked at and turned on, in a workspace that's new.
	// A backup comes back another way, with its rules as they were.
	if e.name == "automation" {
		r.values["enabled"] = false
	}
	for _, f := range e.fields {
		switch v := r.values[f.name].(type) {
		case string:
			if f.kind == kRef || f.name == "series_id" {
				r.values[f.name] = m.of(v)
			}
		case jsonText:
			// A template's snapshot holds IDs inside text too: its pages'
			// bodies and its tasks' notes link to its own rows.
			if e.name == "template" {
				v = jsonText(m.text(string(v), "", ""))
			}
			var parsed any
			if json.Unmarshal([]byte(v), &parsed) == nil {
				if out, err := json.Marshal(m.json(parsed)); err == nil {
					r.values[f.name] = jsonText(out)
				}
			}
		}
	}
	if pair := pairs[e.name]; r.paired {
		a, _ := r.values[pair[0]].(string)
		b, _ := r.values[pair[1]].(string)
		old := r.id
		r.id = PairID(a, b)
		if m.fresh {
			m.memo[old] = r.id
		}
		return
	}
	r.id = m.of(r.id)
}

// uuidText finds IDs in text.
var uuidText = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// text replaces, in a document's Markdown, the IDs of rows given new ones
// and the old workspace's ID, so its links and images point at the rows
// restored.
func (m *idMap) text(s, from, to string) string {
	return uuidText.ReplaceAllStringFunc(s, func(id string) string {
		if from != "" && strings.EqualFold(id, from) {
			return to
		}
		if n, ok := m.memo[strings.ToLower(id)]; ok {
			return n
		}
		return id
	})
}

// json replaces, anywhere in a JSON value, strings that are IDs of rows
// given new ones, and people who became someone else.
func (m *idMap) json(v any) any {
	switch x := v.(type) {
	case string:
		if id, ok := m.memo[x]; ok {
			return id
		}
		if id, ok := m.people[x]; ok {
			return id
		}
	case []any:
		for i := range x {
			x[i] = m.json(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = m.json(x[k])
		}
	}
	return v
}

// checkFree refuses a restore keeping IDs when any of them is in use on
// this instance (the workspace is still here, or was imported before).
func (s *Service) checkFree(ctx context.Context, byEntity map[string][]*restoring) error {
	d := s.st.DB()
	for _, name := range syncOrder {
		list := byEntity[name]
		for start := 0; start < len(list); start += restoreChunk {
			chunk := list[start:min(start+restoreChunk, len(list))]
			args := make([]any, len(chunk))
			for i, r := range chunk {
				args[i] = r.id
			}
			marks := strings.TrimSuffix(strings.Repeat("?, ", len(chunk)), ", ")
			taken, err := queryIDs(ctx, d, rebind(d.Dialect, "SELECT id FROM "+entities[name].table+" WHERE id IN ("+marks+")"), args...)
			if err != nil {
				return err
			}
			if len(taken) > 0 {
				return apperr.New(apperr.Conflict, "Some of what this export holds is already on this instance (the workspace may still be here). Import it with new IDs instead.")
			}
		}
	}
	return nil
}

// settle checks references across the rows, in sync order, and returns
// them in the order they can be written: a reference to something that
// did not come in is cleared where the field may be empty and changes no
// one's view of the row, and otherwise the row is left out. A task's
// status must be its project's, and a subtask must sit in its parent's
// project, under no loop; parents are written before their subtasks.
func settle(byEntity map[string][]*restoring, drop func(string)) ([]*restoring, error) {
	have := map[string]map[string]*restoring{}
	var out []*restoring
	for _, name := range syncOrder {
		e := entities[name]
		have[name] = map[string]*restoring{}
		var kept []*restoring
	rows:
		for _, r := range byEntity[name] {
			if have[name][r.id] != nil {
				drop(name) // the same row twice
				continue
			}
			for _, f := range e.fields {
				target, ok := r.values[f.name].(string)
				if f.kind != kRef || !ok || f.ref == name {
					continue
				}
				if have[f.ref][target] == nil {
					if !f.nullable || f.immutable {
						drop(name)
						continue rows
					}
					r.values[f.name] = nil
				}
			}
			if name == "task" {
				if st, ok := r.values["status_id"].(string); ok && have["status"][st].values["project_id"] != r.values["project_id"] {
					r.values["status_id"] = nil
				}
			}
			have[name][r.id] = r
			kept = append(kept, r)
		}
		switch name {
		case "task":
			kept = parentsFirst(kept, have["task"])
		case "page":
			kept = pagesInWikis(kept, have["page"], drop)
		}
		out = append(out, kept...)
	}
	return out, nil
}

// parentsFirst clears parents that are missing, in another project or in
// a loop, and orders tasks so each parent comes before its subtasks.
func parentsFirst(tasks []*restoring, byID map[string]*restoring) []*restoring {
	parent := func(r *restoring) *restoring {
		id, _ := r.values["parent_id"].(string)
		return byID[id]
	}
	for _, r := range tasks {
		p := parent(r)
		if p == nil || p.values["project_id"] != r.values["project_id"] {
			r.values["parent_id"] = nil
			continue
		}
		for up, depth := p, 0; up != nil; up, depth = parent(up), depth+1 {
			if up == r || depth > 50 {
				r.values["parent_id"] = nil
				break
			}
		}
	}
	done := map[string]bool{}
	out := make([]*restoring, 0, len(tasks))
	var place func(r *restoring)
	place = func(r *restoring) {
		if done[r.id] {
			return
		}
		done[r.id] = true
		if p := parent(r); p != nil {
			place(p)
		}
		out = append(out, r)
	}
	for _, r := range tasks {
		place(r)
	}
	return out
}

// pagesInWikis keeps every page in its wiki: one whose parent is missing,
// in another wiki or below the page itself goes under its wiki's main
// page, and with no main page to go under it goes, with its subpages.
// Parents come before their subpages. Pages that go leave byID, so their
// documents go too.
func pagesInWikis(pages []*restoring, byID map[string]*restoring, drop func(string)) []*restoring {
	parent := func(r *restoring) *restoring {
		id, _ := r.values["parent_id"].(string)
		return byID[id]
	}
	mains := map[any]*restoring{}
	for _, r := range pages {
		if r.values["parent_id"] == nil {
			mains[r.values["project_id"]] = r
		}
	}
	for _, r := range pages {
		if r.values["parent_id"] == nil {
			continue
		}
		p := parent(r)
		fits := p != nil && p.values["project_id"] == r.values["project_id"]
		for up, depth := p, 0; fits && up != nil; up, depth = parent(up), depth+1 {
			fits = up != r && depth <= maxPageDepth
		}
		if !fits {
			r.values["parent_id"] = ""
			if m := mains[r.values["project_id"]]; m != nil {
				r.values["parent_id"] = m.id
			}
		}
	}
	// A page stays if its parents lead up to a main page.
	stays := map[string]bool{}
	var leads func(r *restoring) bool
	leads = func(r *restoring) bool {
		if v, ok := stays[r.id]; ok {
			return v
		}
		stays[r.id] = false
		v := r.values["parent_id"] == nil
		if p := parent(r); p != nil {
			v = leads(p)
		}
		stays[r.id] = v
		return v
	}
	done := map[string]bool{}
	out := make([]*restoring, 0, len(pages))
	var place func(r *restoring)
	place = func(r *restoring) {
		if done[r.id] {
			return
		}
		done[r.id] = true
		if p := parent(r); p != nil {
			place(p)
		}
		out = append(out, r)
	}
	for _, r := range pages {
		if !leads(r) {
			delete(byID, r.id)
			drop("page")
		}
	}
	for _, r := range pages {
		if stays[r.id] {
			place(r)
		}
	}
	return out
}
