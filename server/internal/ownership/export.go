package ownership

import (
	"archive/zip"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/version"
)

// FormatName and FormatVersion mark a JSON export. A change to its shape
// that older Mila could not read raises the version; imports refuse
// versions newer than they know.
const (
	FormatName    = "mila-export"
	FormatVersion = 1
)

// Manifest is manifest.json in a JSON export.
type Manifest struct {
	Format     string         `json:"format"`
	Version    int            `json:"version"`
	Schema     int64          `json:"schema"`
	App        string         `json:"app"`
	ExportedAt time.Time      `json:"exportedAt"`
	ExportedBy Person         `json:"exportedBy"`
	Workspace  WorkspaceInfo  `json:"workspace"`
	Counts     map[string]int `json:"counts"`
}

// WorkspaceInfo names the exported workspace.
type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Person is someone an export names. Email is left out where the member
// list is not the exporter's to see (a guest's export).
type Person struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Role    string `json:"role,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// Content is workspace.json in a JSON export: every row with its clocks,
// in sync order, the tasks' history, and the people both name.
type Content struct {
	Rows     []syncer.Row          `json:"rows"`
	Activity []syncer.HistoryEntry `json:"activity"`
	// Versions are the wiki pages' history.
	Versions []syncer.PageVersionEntry `json:"pageVersions"`
	People   []Person                  `json:"people"`
}

func (s *Service) handleExport(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	e, err := s.st.GetExport(ctx, r.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && e.Status != "queued" && e.Status != "running") {
		return nil // deleted, or done already
	}
	if err != nil {
		return err
	}
	if err := s.st.StartExport(ctx, e.ID); err != nil {
		return err
	}
	size, err := s.writeExport(ctx, e)
	now := s.now()
	if err != nil {
		// A failed export is not tried again by the runner: the person sees
		// why and can ask again.
		s.log.Error("export failed", "export", e.ID, "err", err)
		return s.st.FailExport(ctx, sqlitedb.FailExportParams{
			ID: e.ID, Error: sql.NullString{String: message(err, "The export failed on the server. Try again, or tell your admin."), Valid: true},
			Now: sql.NullTime{Time: now, Valid: true},
		})
	}
	if err := s.st.FinishExport(ctx, sqlitedb.FinishExportParams{
		ID: e.ID, Size: size, Now: sql.NullTime{Time: now, Valid: true}, ExpiresAt: now.Add(ExportLifetime),
	}); err != nil {
		return err
	}
	ws, err := s.st.GetWorkspace(ctx, e.WorkspaceID)
	if err != nil {
		return nil // the workspace went meanwhile; the export went with it
	}
	what := map[string]string{JSON: "JSON", CSV: "CSV", Markdown: "Markdown"}[e.Format]
	err = s.tell.Inform(ctx, e.UserID, e.WorkspaceID, notify.ExportReady, "Your export of "+ws.Name+" is ready",
		fmt.Sprintf("%s, %s. Download it from Settings, Import and export, within 7 days.", what, humanSize(size)))
	if err != nil {
		s.log.Warn("tell about export", "export", e.ID, "err", err)
	}
	return nil
}

// writeExport writes an export's zip beside its final name and moves it
// there once it is whole, so a download never gets half a file.
func (s *Service) writeExport(ctx context.Context, e sqlitedb.Export) (int64, error) {
	if err := files.CheckRoom(s.exports); err != nil {
		return 0, err
	}
	dump, err := s.sync.Dump(ctx, e.UserID, e.WorkspaceID)
	if err != nil {
		return 0, err
	}
	ws, err := s.st.GetWorkspace(ctx, e.WorkspaceID)
	if err != nil {
		return 0, err
	}
	people, err := s.people(ctx, e.UserID, e.WorkspaceID, dump)
	if err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(s.exports, e.ID+".tmp-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	z := zip.NewWriter(tmp)
	switch e.Format {
	case JSON:
		err = s.writeJSON(ctx, z, e, ws, dump, people)
	case CSV:
		err = writeCSV(z, newModel(dump, people))
	case Markdown:
		err = writeMarkdown(z, newModel(dump, people))
	default:
		err = fmt.Errorf("unknown format %q", e.Format)
	}
	if err == nil {
		err = z.Close()
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(tmp.Name())
	if err != nil {
		return 0, err
	}
	return info.Size(), os.Rename(tmp.Name(), s.exportPath(e.ID))
}

// people names everyone the dump mentions, and, for those who may see the
// member list, every member with their role and email. Someone who has
// left is named without an email, as the app shows them to members. A
// guest's export has names and no emails, and "Someone" for another guest
// they share no project with, as the app shows them to guests.
func (s *Service) people(ctx context.Context, userID, workspaceID string, dump *syncer.Dump) ([]Person, error) {
	full := authz.Can(dump.Role, authz.ViewMembers)
	byID := map[string]*Person{}
	var order []string
	add := func(p Person) {
		if byID[p.ID] == nil {
			order = append(order, p.ID)
			byID[p.ID] = &p
		}
	}
	if full {
		members, err := s.st.ListMembers(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			add(Person{ID: m.ID, Name: m.Name, Email: m.Email, Role: m.Role})
		}
	}
	for _, id := range dump.People {
		u, err := s.st.GetUser(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		name := u.Name
		if dump.Hidden[u.ID] {
			name = syncer.Someone
		}
		add(Person{ID: u.ID, Name: name, Deleted: u.DeletedAt.Valid})
	}
	out := make([]Person, len(order))
	for i, id := range order {
		out[i] = *byID[id]
	}
	return out, nil
}

func (s *Service) writeJSON(ctx context.Context, z *zip.Writer, e sqlitedb.Export, ws store.Workspace, dump *syncer.Dump, people []Person) error {
	schema, _, err := s.st.DB().SchemaVersion(ctx)
	if err != nil {
		return err
	}
	me, err := s.st.GetUser(ctx, e.UserID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, r := range dump.Rows {
		counts[r.Entity]++
	}
	m := Manifest{
		Format: FormatName, Version: FormatVersion, Schema: schema, App: version.Version, ExportedAt: s.now(),
		ExportedBy: Person{ID: me.ID, Name: me.Name, Email: me.Email},
		Workspace:  WorkspaceInfo{ID: ws.ID, Name: ws.Name}, Counts: counts,
	}
	if err := writeJSONFile(z, "manifest.json", m); err != nil {
		return err
	}
	if err := writeJSONFile(z, "workspace.json", Content{Rows: dump.Rows, Activity: dump.Activity, Versions: dump.Versions, People: people}); err != nil {
		return err
	}
	for _, r := range dump.Rows {
		if r.Entity != "attachment" && r.Entity != "page_attachment" {
			continue
		}
		// Each file is a copy, kept for a week: the disk is checked before
		// each, as an upload's is.
		if err := files.CheckRoom(s.exports); err != nil {
			return err
		}
		key, _ := r.Fields["storage_key"].(string)
		name, _ := r.Fields["filename"].(string)
		f, err := s.files.Open(key)
		if errors.Is(err, os.ErrNotExist) {
			continue // swept already; the row says what it was
		}
		if err != nil {
			return err
		}
		w, err := z.CreateHeader(&zip.FileHeader{Name: path.Join("attachments", r.ID, safeName(name, "file")), Method: zip.Store, Modified: s.now()})
		if err == nil {
			_, err = io.Copy(w, f)
		}
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	return writeFile(z, "README.txt", []byte(readmeJSON))
}

const readmeJSON = `This is a Mila export in JSON.

manifest.json says which workspace it is, who exported it and when.
workspace.json holds every project, task, label, field, view, comment,
checklist item and wiki page the exporter could see, with deleted ones still
in the trash, the history of each task and each page, and the people they
name. Task notes and pages are Markdown. attachments/ holds the files, a
task's or a page's, one folder per attachment.

Any Mila can import it into a new workspace: Settings, Import and export, Import.
`

func writeJSONFile(z *zip.Writer, name string, v any) error {
	w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(v)
}

func writeFile(z *zip.Writer, name string, b []byte) error {
	w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err == nil {
		_, err = w.Write(b)
	}
	return err
}

// model is a dump as the readable formats see it: live rows only, read
// into the shapes the CSV and Markdown writers walk.
type model struct {
	projects   []rowOf // in their sidebar order
	statuses   map[string]rowOf
	labels     map[string]rowOf
	fields     []rowOf // in their order
	tasks      []rowOf // in order, parents before subtasks
	taskByID   map[string]rowOf
	taskLabels map[string][]string // task to label names
	assignees  map[string][]string // task to user IDs
	blockers   map[string][]string // task to the IDs of the live tasks it waits for
	checklist  map[string][]rowOf
	comments   map[string][]rowOf
	files      map[string][]rowOf
	pages      []rowOf                   // live wiki pages, in their order
	threads    map[string][]rowOf        // a page's threads, oldest first
	threadOf   map[string][]rowOf        // a thread's comments, oldest first
	values     map[string]map[string]any // task to field to value
	names      map[string]string         // user ID to name
	entries    []rowOf                   // time logged on live tasks and on projects themselves, by day
	logged     map[string]int64          // task to the minutes logged on it
}

// rowOf is a row's fields, with its ID under "id".
type rowOf map[string]any

func (r rowOf) str(k string) string { s, _ := r[k].(string); return s }

func newModel(dump *syncer.Dump, people []Person) *model {
	m := &model{statuses: map[string]rowOf{}, labels: map[string]rowOf{}, taskByID: map[string]rowOf{},
		taskLabels: map[string][]string{}, assignees: map[string][]string{}, blockers: map[string][]string{}, checklist: map[string][]rowOf{},
		comments: map[string][]rowOf{}, files: map[string][]rowOf{}, values: map[string]map[string]any{}, names: map[string]string{},
		logged: map[string]int64{}, threads: map[string][]rowOf{}, threadOf: map[string][]rowOf{}}
	for _, p := range people {
		m.names[p.ID] = p.Name
	}
	live := map[string][]rowOf{}
	for _, r := range dump.Rows {
		if r.Fields["deleted_at"] != nil {
			continue
		}
		// Through JSON, so values held as JSON text (a field's value, its
		// choices) read as the values they are.
		row := rowOf{}
		if raw, err := json.Marshal(r.Fields); err == nil {
			_ = json.Unmarshal(raw, &row)
		}
		row["id"] = r.ID
		if at := clockTime(r.Clocks["author_id"]); !at.IsZero() {
			row["_made"] = at
		} else if at := clockTime(r.Clocks["created_by"]); !at.IsZero() {
			row["_made"] = at
		}
		live[r.Entity] = append(live[r.Entity], row)
	}
	bySort := func(list []rowOf) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].str("sort_key") < list[j].str("sort_key") })
	}
	m.projects = live["project"]
	bySort(m.projects)
	m.pages = live["page"]
	bySort(m.pages)
	for _, s := range live["status"] {
		m.statuses[s.str("id")] = s
	}
	for _, l := range live["label"] {
		m.labels[l.str("id")] = l
	}
	m.fields = live["custom_field"]
	bySort(m.fields)
	tasks := live["task"]
	bySort(tasks)
	for _, t := range tasks {
		m.taskByID[t.str("id")] = t
	}
	// Each top-level task, then its subtasks under it.
	children := map[string][]rowOf{}
	for _, t := range tasks {
		if p := t.str("parent_id"); p != "" && m.taskByID[p] != nil {
			children[p] = append(children[p], t)
		}
	}
	var walk func(t rowOf, depth int)
	walk = func(t rowOf, depth int) {
		m.tasks = append(m.tasks, t)
		if depth < 50 {
			for _, c := range children[t.str("id")] {
				walk(c, depth+1)
			}
		}
	}
	for _, t := range tasks {
		if p := t.str("parent_id"); p == "" || m.taskByID[p] == nil {
			walk(t, 0)
		}
	}
	for _, tl := range live["task_label"] {
		if l := m.labels[tl.str("label_id")]; l != nil {
			m.taskLabels[tl.str("task_id")] = append(m.taskLabels[tl.str("task_id")], l.str("name"))
		}
	}
	for _, a := range live["task_assignee"] {
		m.assignees[a.str("task_id")] = append(m.assignees[a.str("task_id")], a.str("user_id"))
	}
	for _, dep := range live["task_dependency"] {
		if b := dep.str("blocker_id"); m.taskByID[b] != nil {
			m.blockers[dep.str("task_id")] = append(m.blockers[dep.str("task_id")], b)
		}
	}
	for _, list := range m.blockers {
		sort.Strings(list)
	}
	for _, c := range live["checklist_item"] {
		m.checklist[c.str("task_id")] = append(m.checklist[c.str("task_id")], c)
	}
	for _, list := range m.checklist {
		bySort(list)
	}
	for _, c := range live["comment"] {
		m.comments[c.str("task_id")] = append(m.comments[c.str("task_id")], c)
	}
	for _, list := range m.comments {
		sort.SliceStable(list, func(i, j int) bool {
			a, _ := list[i]["_made"].(time.Time)
			b, _ := list[j]["_made"].(time.Time)
			return a.Before(b)
		})
	}
	byMade := func(list []rowOf) {
		sort.SliceStable(list, func(i, j int) bool {
			a, _ := list[i]["_made"].(time.Time)
			b, _ := list[j]["_made"].(time.Time)
			return a.Before(b)
		})
	}
	for _, t := range live["page_thread"] {
		m.threads[t.str("page_id")] = append(m.threads[t.str("page_id")], t)
	}
	for _, list := range m.threads {
		byMade(list)
	}
	for _, c := range live["page_comment"] {
		m.threadOf[c.str("thread_id")] = append(m.threadOf[c.str("thread_id")], c)
	}
	for _, list := range m.threadOf {
		byMade(list)
	}
	for _, f := range live["attachment"] {
		m.files[f.str("task_id")] = append(m.files[f.str("task_id")], f)
	}
	for _, e := range live["time_entry"] {
		if m.taskByID[e.str("task_id")] == nil {
			continue
		}
		m.entries = append(m.entries, e)
		m.logged[e.str("task_id")] += minutesOf(e)
	}
	// Time logged to a project itself has no task; its project must be here.
	inExport := map[string]bool{}
	for _, p := range m.projects {
		inExport[p.str("id")] = true
	}
	for _, e := range live["project_time_entry"] {
		if inExport[e.str("project_id")] {
			m.entries = append(m.entries, e)
		}
	}
	sort.SliceStable(m.entries, func(i, j int) bool { return m.entries[i].str("day") < m.entries[j].str("day") })
	for _, v := range live["custom_field_value"] {
		t := v.str("task_id")
		if m.values[t] == nil {
			m.values[t] = map[string]any{}
		}
		m.values[t][v.str("field_id")] = v["value"]
	}
	return m
}

// minutesOf is a row's whole number of minutes, as JSON read it.
func minutesOf(r rowOf) int64 {
	n, _ := r["minutes"].(float64)
	return int64(n)
}

// estimateOf is a task's estimate in minutes as text, or "" for none.
func estimateOf(t rowOf) string {
	if n, ok := t["estimate_minutes"].(float64); ok {
		return strconv.FormatInt(int64(n), 10)
	}
	return ""
}

// loggedOf is the minutes logged on a task as text, or "" for none.
func (m *model) loggedOf(t rowOf) string {
	if n := m.logged[t.str("id")]; n > 0 {
		return strconv.FormatInt(n, 10)
	}
	return ""
}

func (m *model) name(userID string) string {
	if n := m.names[userID]; n != "" {
		return n
	}
	return "Someone"
}

// groups are the projects with their tasks, the inbox first.
type group struct {
	name    string
	project string // "" for the inbox
	tasks   []rowOf
}

func (m *model) groups() []group {
	out := []group{{name: "Inbox"}}
	index := map[string]int{"": 0}
	for _, p := range m.projects {
		index[p.str("id")] = len(out)
		out = append(out, group{name: p.str("name"), project: p.str("id")})
	}
	for _, t := range m.tasks {
		if i, ok := index[t.str("project_id")]; ok {
			out[i].tasks = append(out[i].tasks, t)
		}
	}
	if len(out[0].tasks) == 0 {
		out = out[1:]
	}
	return out
}

// fieldsFor are the custom fields a project's tasks have: the workspace's
// and the project's own.
func (m *model) fieldsFor(project string) []rowOf {
	var out []rowOf
	for _, f := range m.fields {
		if p := f.str("project_id"); p == "" || p == project {
			out = append(out, f)
		}
	}
	return out
}

// fieldText is a field's value as a person reads it: choices by name,
// people by name, lists joined.
func (m *model) fieldText(field rowOf, v any) string {
	options := map[string]string{}
	if list, ok := field["options"].([]any); ok {
		for _, o := range list {
			if om, ok := o.(map[string]any); ok {
				id, _ := om["id"].(string)
				name, _ := om["name"].(string)
				options[id] = name
			}
		}
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		switch field.str("type") {
		case "select":
			if n, ok := options[x]; ok {
				return n
			}
		case "user":
			return m.name(x)
		}
		return x
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			s, _ := item.(string)
			if n, ok := options[s]; ok {
				s = n
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "; ")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (m *model) status(t rowOf) (name, category string) {
	if s := m.statuses[t.str("status_id")]; s != nil {
		return s.str("name"), s.str("category")
	}
	if t["completed_at"] != nil {
		return "", "done"
	}
	return "", ""
}

func (m *model) assigneeNames(task string) []string {
	var out []string
	for _, id := range m.assignees[task] {
		out = append(out, m.name(id))
	}
	sort.Strings(out)
	return out
}

// writeCSV writes one CSV per project, and one for the inbox: a row per
// task (subtasks too), a column per custom field.
func writeCSV(z *zip.Writer, m *model) error {
	used := map[string]bool{}
	for _, g := range m.groups() {
		fields := m.fieldsFor(g.project)
		header := []string{"ID", "Title", "Status", "Status category", "Priority", "Start date", "Due date", "Due time",
			"Completed", "Repeat", "Parent ID", "Parent", "Blocked by", "Labels", "Assignees", "Created by",
			"Estimate (minutes)", "Logged (minutes)", "Notes"}
		for _, f := range fields {
			header = append(header, f.str("name"))
		}
		var b strings.Builder
		writeRow(&b, header)
		for _, t := range g.tasks {
			status, category := m.status(t)
			labels := append([]string(nil), m.taskLabels[t.str("id")]...)
			sort.Strings(labels)
			parent := ""
			if p := m.taskByID[t.str("parent_id")]; p != nil {
				parent = p.str("title")
			}
			creator := ""
			if c := t.str("created_by"); c != "" {
				creator = m.name(c)
			}
			var blockedBy []string
			for _, b := range m.blockers[t.str("id")] {
				blockedBy = append(blockedBy, m.taskByID[b].str("title"))
			}
			rec := []string{t.str("id"), t.str("title"), status, category, t.str("priority"), t.str("start_date"),
				t.str("due_date"), t.str("due_time"), t.str("completed_at"), t.str("recurrence"), t.str("parent_id"), parent,
				strings.Join(blockedBy, "; "),
				strings.Join(labels, "; "), strings.Join(m.assigneeNames(t.str("id")), "; "), creator,
				estimateOf(t), m.loggedOf(t), t.str("notes")}
			for _, f := range fields {
				rec = append(rec, m.fieldText(f, m.values[t.str("id")][f.str("id")]))
			}
			writeRow(&b, rec)
		}
		if err := writeFile(z, unique(used, safeName(g.name, "Project"), ".csv"), []byte(b.String())); err != nil {
			return err
		}
	}
	return writeTimeCSV(z, m, used)
}

// writeTimeCSV writes the time logged, a row per entry, when there is any.
func writeTimeCSV(z *zip.Writer, m *model, used map[string]bool) error {
	if len(m.entries) == 0 {
		return nil
	}
	var b strings.Builder
	writeRow(&b, []string{"Day", "Person", "Minutes", "Running", "Task ID", "Task", "Project", "Note"})
	projects := map[string]string{}
	for _, p := range m.projects {
		projects[p.str("id")] = p.str("name")
	}
	for _, e := range m.entries {
		// Time on a project itself has no task: its task columns stay empty.
		t := m.taskByID[e.str("task_id")]
		project := "Inbox"
		if p := cmp.Or(e.str("project_id"), t.str("project_id")); p != "" {
			project = projects[p]
		}
		running := ""
		if on, _ := e["running"].(bool); on {
			running = "yes"
		}
		rec := []string{e.str("day"), m.name(e.str("user_id")), strconv.FormatInt(minutesOf(e), 10), running,
			t.str("id"), t.str("title"), project, e.str("note")}
		writeRow(&b, rec)
	}
	return writeFile(z, unique(used, "Time logged", ".csv"), []byte(b.String()))
}

// writeRow writes one CSV line (RFC 4180, CRLF line ends), each field
// through csvField.
func writeRow(b *strings.Builder, rec []string) {
	for i, s := range rec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(csvField(s))
	}
	b.WriteString("\r\n")
}

// csvField is a CSV field, quoted when it must be and kept from running as
// a formula, by the rule of field in web/src/lib/csv.ts, which
// shared/csv-vectors.json holds the two to. Text that starts like a
// formula (= + - or @) after any whitespace, or starts with a tab or CR,
// gets an apostrophe first, which spreadsheets hide; a plain negative
// number such as -5 is left alone. A field holding a comma, quote, line
// end, ; or tab is quoted: Excel in a ; locale splits lines on ;, and the
// part after one would otherwise be a cell of its own.
func csvField(s string) string {
	if formulaLike(s) && !negativeNumber.MatchString(s) {
		s = "'" + s
	}
	if strings.ContainsAny(s, "\",;\t\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

var negativeNumber = regexp.MustCompile(`^-[0-9]+(\.[0-9]+)?$`)

// formulaLike says whether s starts with a tab or CR, or with = + - or @
// after any whitespace, whitespace being what JavaScript's \s matches.
func formulaLike(s string) bool {
	if s != "" && (s[0] == '\t' || s[0] == '\r') {
		return true
	}
	t := strings.TrimLeftFunc(s, jsSpace)
	return t != "" && strings.IndexByte("=+-@", t[0]) >= 0
}

// jsSpace is JavaScript's \s, which differs from unicode.IsSpace by
// U+0085 (not space) and U+FEFF (space).
func jsSpace(r rune) bool {
	return r == '\ufeff' || (r != '\u0085' && unicode.IsSpace(r))
}

var mentionPattern = regexp.MustCompile(`@\[([^\]\n]{1,100})\]\(user:([A-Za-z0-9-]{1,64})\)`)

// writeMarkdown writes a folder per project (and the inbox) with a file
// per task: its fields as YAML frontmatter, the notes, the checklist, the
// files' names and the comments below, readable in Obsidian or any editor.
// A wiki is a Wiki folder, in its project's or at the top for the
// workspace's, with a file per page and a page's subpages in a folder
// named after it. Links between pages and tasks point at their files.
func writeMarkdown(z *zip.Writer, m *model) error {
	usedDirs := map[string]bool{}
	fileOf := map[string]string{} // task ID to its file name, for links
	pathOf := map[string]string{} // task or page ID to its file's path in the zip
	dirOf := map[string]string{}  // project ID to its folder
	type planned struct {
		dir  string
		file string
		task rowOf
		g    group
	}
	var plan []planned
	for _, g := range m.groups() {
		dir := unique(usedDirs, safeName(g.name, "Project"), "")
		dirOf[g.project] = dir
		used := map[string]bool{}
		for _, t := range g.tasks {
			file := unique(used, safeName(t.str("title"), "Untitled"), ".md")
			fileOf[t.str("id")] = strings.TrimSuffix(file, ".md")
			pathOf[t.str("id")] = dir + "/" + file
			plan = append(plan, planned{dir: dir, file: file, task: t, g: g})
		}
	}
	pages := m.planPages(usedDirs, dirOf, pathOf)
	for _, p := range plan {
		t := p.task
		var b strings.Builder
		b.WriteString("---\n")
		// Keys appear once: a field named like one of the task's own keys
		// (a field called "status") gets a suffix.
		keys := map[string]bool{}
		line := func(k, v string) {
			if v == "" {
				return
			}
			for keys[strings.ToLower(k)] {
				k += " (field)"
			}
			keys[strings.ToLower(k)] = true
			fmt.Fprintf(&b, "%s: %s\n", yamlKey(k), v)
		}
		list := func(k string, items []string) {
			if len(items) == 0 {
				return
			}
			quoted := make([]string, len(items))
			for i, s := range items {
				quoted[i] = yamlString(s)
			}
			keys[k] = true
			fmt.Fprintf(&b, "%s: [%s]\n", yamlKey(k), strings.Join(quoted, ", "))
		}
		status, category := m.status(t)
		line("id", yamlString(t.str("id")))
		line("title", yamlString(t.str("title")))
		if p.g.project != "" {
			line("project", yamlString(p.g.name))
		}
		if status != "" {
			line("status", yamlString(status))
		}
		if category != "" {
			line("category", yamlString(category))
		}
		if pr := t.str("priority"); pr != "" && pr != "none" {
			line("priority", yamlString(pr))
		}
		line("start", t.str("start_date"))
		line("due", t.str("due_date"))
		if dt := t.str("due_time"); dt != "" {
			line("due_time", yamlString(dt))
		}
		if c := t.str("completed_at"); c != "" {
			line("completed", yamlString(c))
		}
		if r := t.str("recurrence"); r != "" {
			line("repeat", yamlString(r))
		}
		if parent := fileOf[t.str("parent_id")]; parent != "" {
			line("parent", yamlString("[["+parent+"]]"))
		}
		var blockedBy []string
		for _, b := range m.blockers[t.str("id")] {
			if f := fileOf[b]; f != "" {
				blockedBy = append(blockedBy, "[["+f+"]]")
			} else {
				blockedBy = append(blockedBy, oneLine(m.taskByID[b].str("title")))
			}
		}
		list("blocked_by", blockedBy)
		if est := estimateOf(t); est != "" {
			line("estimate_minutes", est)
		}
		if logged := m.loggedOf(t); logged != "" {
			line("logged_minutes", logged)
		}
		labels := append([]string(nil), m.taskLabels[t.str("id")]...)
		sort.Strings(labels)
		list("labels", labels)
		list("assignees", m.assigneeNames(t.str("id")))
		if c := t.str("created_by"); c != "" {
			line("created_by", yamlString(m.name(c)))
		}
		for _, f := range m.fieldsFor(p.g.project) {
			if v, ok := m.values[t.str("id")][f.str("id")]; ok {
				line(f.str("name"), yamlString(m.fieldText(f, v)))
			}
		}
		b.WriteString("---\n\n# " + oneLine(t.str("title")) + "\n")
		if notes := strings.TrimSpace(t.str("notes")); notes != "" {
			b.WriteString("\n" + localLinks(notes, p.dir+"/"+p.file, pathOf) + "\n")
		}
		if items := m.checklist[t.str("id")]; len(items) > 0 {
			b.WriteString("\n## Checklist\n\n")
			for _, c := range items {
				mark := " "
				if done, _ := c["done"].(bool); done {
					mark = "x"
				}
				fmt.Fprintf(&b, "- [%s] %s\n", mark, oneLine(c.str("text")))
			}
		}
		if files := m.files[t.str("id")]; len(files) > 0 {
			b.WriteString("\n## Attachments\n\n")
			for _, f := range files {
				fmt.Fprintf(&b, "- %s\n", oneLine(f.str("filename")))
			}
		}
		if comments := m.comments[t.str("id")]; len(comments) > 0 {
			b.WriteString("\n## Comments\n")
			for _, c := range comments {
				body := mentionPattern.ReplaceAllString(c.str("body"), "@$1")
				fmt.Fprintf(&b, "\n**%s**", m.name(c.str("author_id")))
				if at, ok := c["_made"].(time.Time); ok {
					b.WriteString(", " + at.Format("2006-01-02 15:04") + " UTC")
				}
				if e := c.str("edited_at"); e != "" {
					b.WriteString(" (edited)")
				}
				b.WriteString("\n\n" + strings.TrimSpace(body) + "\n")
			}
		}
		if err := writeFile(z, p.dir+"/"+p.file, []byte(b.String())); err != nil {
			return err
		}
	}
	for _, pg := range pages {
		text := "# " + pg.title + "\n"
		if body := strings.TrimSpace(pg.page.str("body")); body != "" {
			text += "\n" + localLinks(body, pg.path, pathOf) + "\n"
		}
		text += m.pageComments(pg.page.str("id"))
		if err := writeFile(z, pg.path, []byte(text)); err != nil {
			return err
		}
	}
	return nil
}

// pageComments is a page's threads as the end of its Markdown file: the words
// each is on, and its comments, as a task's are written.
func (m *model) pageComments(pageID string) string {
	var b strings.Builder
	for _, t := range m.threads[pageID] {
		comments := m.threadOf[t.str("id")]
		if len(comments) == 0 {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("\n## Comments\n")
		}
		b.WriteString("\n> " + strings.Join(strings.Fields(t.str("quote")), " ") + "\n")
		if t.str("resolved_at") != "" {
			b.WriteString("\nResolved\n")
		}
		for _, c := range comments {
			body := mentionPattern.ReplaceAllString(c.str("body"), "@$1")
			fmt.Fprintf(&b, "\n**%s**", m.name(c.str("author_id")))
			if at, ok := c["_made"].(time.Time); ok {
				b.WriteString(", " + at.Format("2006-01-02 15:04") + " UTC")
			}
			if c.str("edited_at") != "" {
				b.WriteString(" (edited)")
			}
			b.WriteString("\n\n" + strings.TrimSpace(body) + "\n")
		}
	}
	return b.String()
}

// plannedPage is a wiki page's file in a Markdown export.
type plannedPage struct {
	page  rowOf
	title string
	path  string
}

// planPages places the wikis' pages: each wiki's main page at the top of
// its Wiki folder, named after the wiki, and every other page beside its
// parent's file, or for the main page's, beside the main page, with its
// own subpages in a folder of its name.
func (m *model) planPages(usedDirs map[string]bool, dirOf, pathOf map[string]string) []plannedPage {
	children := map[string][]rowOf{}
	var mains []rowOf
	for _, p := range m.pages {
		if parent := p.str("parent_id"); parent != "" {
			children[parent] = append(children[parent], p)
		} else {
			mains = append(mains, p)
		}
	}
	names := map[string]string{}
	for _, p := range m.projects {
		names[p.str("id")] = p.str("name")
	}
	var out []plannedPage
	for _, main := range mains {
		project := main.str("project_id")
		wiki := "Wiki"
		var base string
		if project == "" {
			base = unique(usedDirs, "Wiki", "")
		} else {
			wiki = names[project]
			dir, ok := dirOf[project]
			if !ok {
				continue // a page of a project the export leaves out
			}
			base = dir + "/Wiki"
		}
		used := map[string]map[string]bool{}
		place := func(dir, name string) string {
			if used[dir] == nil {
				used[dir] = map[string]bool{}
			}
			return dir + "/" + unique(used[dir], safeName(name, "Untitled"), ".md")
		}
		var walk func(p rowOf, title, dir string, depth int)
		walk = func(p rowOf, title, dir string, depth int) {
			file := place(dir, title)
			pathOf[p.str("id")] = file
			out = append(out, plannedPage{page: p, title: title, path: file})
			if depth > 64 {
				return
			}
			sub := dir
			if depth > 0 {
				sub = strings.TrimSuffix(file, ".md")
			}
			for _, c := range children[p.str("id")] {
				title := strings.TrimSpace(c.str("title"))
				if title == "" {
					title = "Untitled"
				}
				walk(c, title, sub, depth+1)
			}
		}
		walk(main, wiki, base, 0)
	}
	return out
}

// appLink finds the addresses of pages and tasks in Markdown links.
var appLink = regexp.MustCompile(`\]\((/[^)\s]*\?[^)\s]*)`)

// localLinks points the links in a document at the files a Markdown
// export writes for the pages and tasks they name; others stay as written.
func localLinks(md, from string, pathOf map[string]string) string {
	return appLink.ReplaceAllStringFunc(md, func(link string) string {
		target := pathOf[syncer.LinkTarget(link[2:])]
		if target == "" {
			return link
		}
		rel, err := filepath.Rel(filepath.FromSlash(path.Dir(from)), filepath.FromSlash(target))
		if err != nil {
			return link
		}
		return "](<" + filepath.ToSlash(rel) + ">"
	})
}

// yamlString writes s as a YAML double-quoted scalar, which reads back
// exactly whatever it holds (colons, quotes, leading dashes, new lines).
func yamlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || r == utf8.RuneError:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// yamlKey writes a key plainly when it is a simple word, else quoted.
func yamlKey(k string) string {
	for _, r := range k {
		if r != '_' && r != '-' && r != ' ' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return yamlString(k)
		}
	}
	if k == "" || strings.TrimSpace(k) != k || strings.HasPrefix(k, "-") || unicode.IsDigit(rune(k[0])) {
		return yamlString(k)
	}
	switch strings.ToLower(k) {
	case "true", "false", "yes", "no", "on", "off", "null", "y", "n":
		return yamlString(k)
	}
	return k
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

var reserved = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// safeName makes a name usable as a file or folder name on any system:
// no path separators, no characters Windows refuses, no names it keeps
// for devices, no trailing dots or spaces, not too long.
func safeName(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32 || r == 127, strings.ContainsRune(`<>:"/\|?*`, r):
			return '-'
		}
		return r
	}, oneLine(name))
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	name = strings.TrimRight(name, ". ")
	name = strings.TrimLeft(name, ". ")
	if name == "" {
		name = fallback
	}
	if reserved.MatchString(name) {
		name = "_" + name
	}
	return name
}

// unique adds " (2)", " (3)"... to a name taken already in used, ignoring
// case as Windows and macOS do, and records it.
func unique(used map[string]bool, name, ext string) string {
	out := name + ext
	for n := 2; used[strings.ToLower(out)]; n++ {
		out = fmt.Sprintf("%s (%d)%s", name, n, ext)
	}
	used[strings.ToLower(out)] = true
	return out
}

// fileSlug is a workspace name as part of a download's file name.
func fileSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		return "workspace"
	}
	return s
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// clockTime is when an HLC was made, to the minute's use: the 13 digits
// of milliseconds it starts with.
func clockTime(hlc string) time.Time {
	if len(hlc) < 13 {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(hlc[:13], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
