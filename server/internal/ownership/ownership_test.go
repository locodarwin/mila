package ownership_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/ownership"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// told records what people were told, and whose workspaces changed.
type told struct {
	mu      sync.Mutex
	kinds   []notify.Kind
	titles  []string
	changed []string
}

func (t *told) Inform(_ context.Context, _, _ string, kind notify.Kind, title, _ string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.kinds = append(t.kinds, kind)
	t.titles = append(t.titles, title)
	return nil
}

func (t *told) MembershipChanged(ids ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.changed = append(t.changed, ids...)
}

type fixture struct {
	t      *testing.T
	d      *db.DB
	st     *store.Store
	sync   *syncer.Service
	files  *files.Store
	runner *jobs.Runner
	svc    *ownership.Service
	clock  *clock
	told   *told
	dir    string
	ws     string
	users  map[string]string
	ids    map[string]string // named rows
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T, d *db.DB, importMax int64) *fixture {
	t.Helper()
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := store.New(d)
	dir := t.TempDir()
	attachments, err := files.New(filepath.Join(dir, "attachments"))
	must(t, err)
	c := &clock{t: time.Now().UTC().Truncate(time.Microsecond)}
	runner := jobs.New(st, log, c.now)
	tl := &told{}
	sync := syncer.New(st, log, nil)
	svc, err := ownership.New(st, log, sync, attachments, runner, tl, tl, ownership.Config{DataDir: dir, ImportMax: importMax})
	must(t, err)
	f := &fixture{t: t, d: d, st: st, sync: sync, files: attachments, runner: runner, svc: svc, clock: c, told: tl, dir: dir,
		ws: uuid.NewString(), users: map[string]string{}, ids: map[string]string{}}
	now := store.Now()
	must(t, st.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: f.ws, Name: "Team: Alpha", Slug: "team-" + f.ws[:8], Now: now}))
	for _, role := range []string{"owner", "member", "guest"} {
		id := uuid.NewString()
		must(t, st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: role + "@example.com", Name: strings.ToUpper(role[:1]) + role[1:], Now: now}))
		must(t, st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: f.ws, UserID: id, Role: role, Now: now}))
		f.users[role] = id
	}
	return f
}

func (f *fixture) id(name string) string {
	if f.ids[name] == "" {
		f.ids[name] = uuid.NewString()
	}
	return f.ids[name]
}

func (f *fixture) push(role string, entity, id string, fields map[string]any) {
	f.t.Helper()
	raw := map[string]json.RawMessage{}
	for k, v := range fields {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	hlc := fmt.Sprintf("%013d-0000-test", time.Now().Add(-time.Minute).UnixMilli())
	res, _, err := f.sync.Push(context.Background(), f.users[role], f.ws, []syncer.Op{{OpID: uuid.NewString(), Entity: entity, ID: id, HLC: hlc, Fields: raw}})
	must(f.t, err)
	if res[0].Status != "applied" {
		f.t.Fatalf("%s %s: %s", entity, id, res[0].Message)
	}
}

func task(project any, title string) map[string]any {
	return map[string]any{"project_id": project, "status_id": nil, "title": title, "sort_key": "a0", "priority": "none"}
}

// fill gives the workspace a project with a field, a label, tasks (one
// whose title would trip a spreadsheet, one named like a Windows device,
// a subtask), a checklist, a comment with a mention and a file; and a
// project the guest cannot see.
func (f *fixture) fill() {
	ctx := context.Background()
	owner := f.users["owner"]
	f.push("owner", "project", f.id("shared"), map[string]any{"name": "Launch: v2", "sort_key": "a0"})
	f.push("owner", "project", f.id("private"), map[string]any{"name": "Secret", "sort_key": "a1"})
	f.push("owner", "status", f.id("todo"), map[string]any{"project_id": f.id("shared"), "name": "To do", "category": "todo", "sort_key": "a0"})
	f.push("owner", "label", f.id("label"), map[string]any{"name": "urgent", "sort_key": "a0"})
	f.push("owner", "custom_field", f.id("field"), map[string]any{"project_id": f.id("shared"), "name": "Size", "type": "select",
		"options": []map[string]string{{"id": "o1", "name": "Large", "color": "red"}}, "sort_key": "a0"})
	main := task(f.id("shared"), `Write "the" report: draft`)
	main["status_id"], main["notes"], main["priority"], main["due_date"] = f.id("todo"), "Notes\n---\nmore", "high", "2026-11-03"
	f.push("owner", "task", f.id("task"), main)
	f.push("owner", "task", f.id("formula"), task(f.id("shared"), "=HYPERLINK(\"http://x\")"))
	f.push("owner", "task", f.id("device"), task(f.id("shared"), "con"))
	sub := task(f.id("shared"), "Outline")
	sub["parent_id"] = f.id("task")
	f.push("owner", "task", f.id("sub"), sub)
	f.push("owner", "task", f.id("secret"), task(f.id("private"), "Hidden plan"))
	f.push("owner", "task", f.id("inbox"), task(nil, "Call the bank"))
	f.push("owner", "task_label", syncer.PairID(f.id("task"), f.id("label")), map[string]any{"task_id": f.id("task"), "label_id": f.id("label")})
	f.push("owner", "task_assignee", syncer.PairID(f.id("task"), f.users["member"]), map[string]any{"task_id": f.id("task"), "user_id": f.users["member"]})
	// "con" waits for the report; the formula task for one the guest cannot see.
	f.push("owner", "task_dependency", syncer.PairID(f.id("device"), f.id("task")), map[string]any{"task_id": f.id("device"), "blocker_id": f.id("task")})
	f.push("owner", "task_dependency", syncer.PairID(f.id("formula"), f.id("secret")), map[string]any{"task_id": f.id("formula"), "blocker_id": f.id("secret")})
	f.push("owner", "custom_field_value", syncer.PairID(f.id("task"), f.id("field")), map[string]any{"task_id": f.id("task"), "field_id": f.id("field"), "value": "o1"})
	f.push("owner", "checklist_item", f.id("item"), map[string]any{"task_id": f.id("task"), "text": "Intro", "done": true, "sort_key": "a0"})
	f.push("member", "comment", f.id("comment"), map[string]any{"task_id": f.id("task"), "author_id": f.users["member"], "body": "Done, @[Owner](user:" + owner + ")"})
	key, size, _, err := f.files.Save(strings.NewReader("hello file"), 1<<20)
	must(f.t, err)
	_, err = f.sync.CreateAttachment(ctx, owner, f.ws, syncer.NewAttachment{ID: f.id("file"), TaskID: f.id("task"), Filename: "notes.txt", Mime: "text/plain", StorageKey: key, Size: size})
	must(f.t, err)
	_, err = f.d.ExecContext(ctx, rebind(f.d, "INSERT INTO project_access (project_id, user_id, created_at) VALUES (?, ?, ?)"), f.id("shared"), f.users["guest"], store.Now())
	must(f.t, err)
}

func rebind(d *db.DB, q string) string {
	if d.Dialect != db.Postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// export makes an export and runs it, returning its zip.
func (f *fixture) export(role, format string) *zip.Reader {
	f.t.Helper()
	ctx := context.Background()
	e, err := f.svc.StartExport(ctx, f.users[role], f.ws, format)
	must(f.t, err)
	_, err = f.runner.RunDue(ctx)
	must(f.t, err)
	file, row, err := f.svc.OpenExport(ctx, f.users[role], e.ID)
	must(f.t, err)
	defer func() { _ = file.Close() }()
	if row.Status != "ready" || row.Size == 0 || !strings.HasSuffix(row.FileName, ".zip") {
		f.t.Fatalf("export row %+v", row)
	}
	b, err := io.ReadAll(file)
	must(f.t, err)
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	must(f.t, err)
	return z
}

func read(t *testing.T, z *zip.Reader, name string) string {
	t.Helper()
	for _, f := range z.File {
		if f.Name == name {
			r, err := f.Open()
			must(t, err)
			b, err := io.ReadAll(r)
			must(t, err)
			return string(b)
		}
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	t.Fatalf("no %s in %v", name, names)
	return ""
}

func TestExportJSON(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		f := newFixture(t, d, 1<<20)
		f.fill()
		z := f.export("owner", ownership.JSON)
		var m ownership.Manifest
		must(t, json.Unmarshal([]byte(read(t, z, "manifest.json")), &m))
		if m.Format != ownership.FormatName || m.Version != ownership.FormatVersion || m.Schema == 0 || m.Workspace.ID != f.ws ||
			m.ExportedBy.ID != f.users["owner"] || m.Counts["task"] != 6 {
			t.Errorf("manifest %+v", m)
		}
		var c ownership.Content
		must(t, json.Unmarshal([]byte(read(t, z, "workspace.json")), &c))
		if len(c.Rows) == 0 || len(c.Activity) == 0 || len(c.People) != 3 || c.People[0].Email == "" {
			t.Errorf("content: %d rows, %d history, people %+v", len(c.Rows), len(c.Activity), c.People)
		}
		if got := read(t, z, "attachments/"+f.id("file")+"/notes.txt"); got != "hello file" {
			t.Errorf("attachment %q", got)
		}
		if len(f.told.kinds) != 1 || f.told.kinds[0] != notify.ExportReady {
			t.Errorf("told %v", f.told.kinds)
		}

		// A guest's export holds only what is shared with them, and no one's
		// address.
		z = f.export("guest", ownership.JSON)
		c = ownership.Content{}
		must(t, json.Unmarshal([]byte(read(t, z, "workspace.json")), &c))
		for _, r := range c.Rows {
			if r.ID == f.id("secret") || r.ID == f.id("private") || r.ID == f.id("inbox") || r.ID == syncer.PairID(f.id("formula"), f.id("secret")) {
				t.Errorf("a guest's export holds %s %s", r.Entity, r.ID)
			}
		}
		for _, p := range c.People {
			if p.Email != "" {
				t.Errorf("a guest's export names %s", p.Email)
			}
		}
	})
}

func TestExportCSVAndMarkdown(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		f := newFixture(t, d, 1<<20)
		f.fill()
		// A field's name is a heading, and could trip a spreadsheet too.
		f.push("member", "custom_field", f.id("trap"), map[string]any{"project_id": f.id("shared"), "name": "=1+2", "type": "text", "sort_key": "a1"})
		// Time on the report: an estimate, and two pieces of time logged.
		f.push("member", "task", f.id("task"), map[string]any{"estimate_minutes": 90})
		f.push("member", "time_entry", f.id("t1"), map[string]any{"task_id": f.id("task"), "user_id": f.users["member"], "day": "2026-11-02", "minutes": 45, "note": "=SUM(1)"})
		f.push("owner", "time_entry", f.id("t2"), map[string]any{"task_id": f.id("task"), "user_id": f.users["owner"], "day": "2026-11-01", "minutes": 60})
		// Time on the project itself has no task.
		f.push("member", "project_time_entry", f.id("p1"), map[string]any{"project_id": f.id("shared"), "user_id": f.users["member"], "day": "2026-11-03", "minutes": 20, "note": "Planning;=1+2"})
		z := f.export("owner", ownership.CSV)
		records, err := csv.NewReader(strings.NewReader(read(t, z, "Launch- v2.csv"))).ReadAll()
		must(t, err)
		header := records[0]
		if !slices.Contains(header, "Size") || !slices.Contains(header, "'=1+2") || header[0] != "ID" {
			t.Fatalf("header %v", header)
		}
		byTitle := map[string][]string{}
		for _, r := range records[1:] {
			byTitle[r[1]] = r
		}
		main := byTitle[`Write "the" report: draft`]
		if main == nil || main[2] != "To do" || main[4] != "high" || main[13] != "urgent" || main[14] != "Member" || main[slices.Index(header, "Size")] != "Large" {
			t.Errorf("task row %v", main)
		}
		if est := slices.Index(header, "Estimate (minutes)"); est < 0 || main[est] != "90" || main[est+1] != "105" {
			t.Errorf("the time columns: %v %v", header, main)
		}
		timeFile := read(t, z, "Time logged.csv")
		// Excel in a ; locale splits an unquoted line on ;, so a field with
		// one is quoted, or the part after it runs as a formula.
		if !strings.Contains(timeFile, `,"Planning;=1+2"`) {
			t.Errorf("a field with ; went out unquoted: %q", timeFile)
		}
		times, err := csv.NewReader(strings.NewReader(timeFile)).ReadAll()
		must(t, err)
		if len(times) != 4 || times[1][0] != "2026-11-01" || times[1][1] != "Owner" || times[2][2] != "45" || times[2][5] != `Write "the" report: draft` || times[2][7] != "'=SUM(1)" {
			t.Errorf("Time logged.csv: %v", times)
		}
		if p := times[len(times)-1]; len(times) == 4 && (p[0] != "2026-11-03" || p[2] != "20" || p[4] != "" || p[5] != "" || p[6] != "Launch: v2" || p[7] != "Planning;=1+2") {
			t.Errorf("time on the project itself: %v", p)
		}
		if header[12] != "Blocked by" || byTitle["con"] == nil || byTitle["con"][12] != `Write "the" report: draft` {
			t.Errorf("the waiting task's row does not name what it waits for: %v", byTitle["con"])
		}
		if byTitle[`'=HYPERLINK("http://x")`] == nil {
			t.Errorf("a formula was left live: %v", records)
		}
		if byTitle["Outline"] == nil || byTitle["Outline"][11] != `Write "the" report: draft` {
			t.Error("the subtask does not name its parent")
		}
		inbox := read(t, z, "Inbox.csv")
		if !strings.Contains(inbox, "Call the bank") || strings.Contains(inbox, "Hidden plan") {
			t.Errorf("inbox.csv: %s", inbox)
		}

		z = f.export("owner", ownership.Markdown)
		md := read(t, z, `Launch- v2/Write -the- report- draft.md`)
		for _, want := range []string{
			`title: "Write \"the\" report: draft"`, `labels: ["urgent"]`, `assignees: ["Member"]`, `priority: "high"`,
			"due: 2026-11-03", "estimate_minutes: 90", "logged_minutes: 105", `Size: "Large"`, "Notes\n---\nmore", "- [x] Intro", "## Comments", "**Member**", "Done, @Owner", "- notes.txt",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("markdown lacks %q:\n%s", want, md)
			}
		}
		if !strings.Contains(read(t, z, "Launch- v2/Outline.md"), `parent: "[[Write -the- report- draft]]"`) {
			t.Error("the subtask's file does not link its parent")
		}
		if con := read(t, z, "Launch- v2/_con.md"); !strings.Contains(con, `blocked_by: ["[[Write -the- report- draft]]"]`) { // a name Windows keeps for a device
			t.Errorf("con.md does not link what it waits for:\n%s", con)
		}
		read(t, z, "Inbox/Call the bank.md")
	})
}

func TestMarkdownExportHoldsTheWikis(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		f := newFixture(t, d, 1<<20)
		f.fill()
		shared := f.id("shared")
		wsMain, projectMain := syncer.MainPageID(f.ws), syncer.MainPageID(shared)
		page := func(id string, project, parent any, title, body string) {
			f.push("owner", "page", id, map[string]any{"project_id": project, "parent_id": parent, "title": title, "body": body, "sort_key": "a0"})
		}
		colorsHref := "/projects/" + shared + "?tab=wiki&page=" + f.id("colors")
		page(wsMain, nil, nil, "", "See [Colors]("+colorsHref+") and [Outline](/?task="+f.id("sub")+").")
		page(projectMain, shared, nil, "", "The launch.")
		page(f.id("colors"), shared, projectMain, "Colors", "Back to [the launch](/projects/"+shared+"?tab=wiki&page="+projectMain+").")
		page(f.id("paint"), shared, f.id("colors"), "Paint", "")

		z := f.export("owner", ownership.Markdown)
		if got := read(t, z, "Wiki/Wiki.md"); got != "# Wiki\n\nSee [Colors](<../Launch- v2/Wiki/Colors.md>) and [Outline](<../Launch- v2/Outline.md>).\n" {
			t.Errorf("Wiki/Wiki.md:\n%s", got)
		}
		if got := read(t, z, "Launch- v2/Wiki/Launch- v2.md"); got != "# Launch: v2\n\nThe launch.\n" {
			t.Errorf("the project's main page:\n%s", got)
		}
		if got := read(t, z, "Launch- v2/Wiki/Colors.md"); !strings.Contains(got, "[the launch](<Launch- v2.md>)") {
			t.Errorf("Colors.md:\n%s", got)
		}
		if got := read(t, z, "Launch- v2/Wiki/Colors/Paint.md"); got != "# Paint\n" {
			t.Errorf("Paint.md:\n%s", got)
		}
		// A page's comments end its file, with the words they are on.
		thread := uuid.NewString()
		f.push("owner", "page_thread", thread, map[string]any{"page_id": f.id("paint"), "created_by": f.users["owner"], "quote": "the  primer"})
		f.push("owner", "page_comment", uuid.NewString(), map[string]any{"page_id": f.id("paint"), "thread_id": thread, "author_id": f.users["owner"], "body": "Which one?"})
		z = f.export("owner", ownership.Markdown)
		got := read(t, z, "Launch- v2/Wiki/Colors/Paint.md")
		for _, want := range []string{"# Paint\n\n## Comments\n\n> the primer\n", "Which one?"} {
			if !strings.Contains(got, want) {
				t.Errorf("Paint.md lacks %q:\n%s", want, got)
			}
		}
		// The guest sees the project's wiki and not the workspace's.
		z = f.export("guest", ownership.Markdown)
		read(t, z, "Launch- v2/Wiki/Colors.md")
		for _, file := range z.File {
			if strings.HasPrefix(file.Name, "Wiki/") {
				t.Errorf("the guest's export holds %s", file.Name)
			}
		}
	})
}

func TestExportRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		f.fill()
		owner := f.users["owner"]
		if _, err := f.svc.StartExport(ctx, owner, f.ws, "pdf"); apperr.KindOf(err) != apperr.Invalid {
			t.Errorf("an unknown format: %v", err)
		}
		if _, err := f.svc.StartExport(ctx, uuid.NewString(), f.ws, ownership.CSV); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("a stranger's export: %v", err)
		}
		var first ownership.Export
		for i := range 3 {
			e, err := f.svc.StartExport(ctx, owner, f.ws, ownership.CSV)
			must(t, err)
			if i == 0 {
				first = e
			}
		}
		if _, err := f.svc.StartExport(ctx, owner, f.ws, ownership.CSV); apperr.KindOf(err) != apperr.Conflict {
			t.Errorf("a fourth export at once: %v", err)
		}
		if _, _, err := f.svc.OpenExport(ctx, owner, first.ID); apperr.KindOf(err) != apperr.Conflict {
			t.Errorf("an unfinished download: %v", err)
		}
		_, err := f.runner.RunDue(ctx)
		must(t, err)
		if _, _, err := f.svc.OpenExport(ctx, f.users["member"], first.ID); apperr.KindOf(err) != apperr.NotFound {
			t.Errorf("someone else's export: %v", err)
		}
		list, err := f.svc.Exports(ctx, owner)
		must(t, err)
		if len(list) != 3 || list[0].Status != "ready" || list[0].WorkspaceName != "Team: Alpha" {
			t.Fatalf("exports %+v", list)
		}
		must(t, f.svc.DeleteExport(ctx, owner, list[0].ID))
		if _, err := os.Stat(filepath.Join(f.dir, "exports", list[0].ID+".zip")); !os.IsNotExist(err) {
			t.Error("a deleted export's file stayed")
		}

		// A week later the rest expire, rows and files, and a stray file goes.
		stray := filepath.Join(f.dir, "exports", "stray.zip")
		must(t, os.WriteFile(stray, []byte("x"), 0o600))
		old := time.Now().Add(-2 * time.Hour)
		entries, _ := os.ReadDir(filepath.Join(f.dir, "exports"))
		for _, e := range entries {
			must(t, os.Chtimes(filepath.Join(f.dir, "exports", e.Name()), old, old))
		}
		must(t, f.svc.Tidy(ctx))
		if list, _ := f.svc.Exports(ctx, owner); len(list) != 2 {
			t.Fatalf("expired too soon: %d left", len(list))
		}
		if _, err := os.Stat(stray); !os.IsNotExist(err) {
			t.Error("a file no export names stayed")
		}
		f.clock.advance(ownership.ExportLifetime + time.Minute)
		must(t, f.svc.Tidy(ctx))
		if list, _ := f.svc.Exports(ctx, owner); len(list) != 0 {
			t.Errorf("%d exports outlived their week", len(list))
		}
		if entries, _ := os.ReadDir(filepath.Join(f.dir, "exports")); len(entries) != 0 {
			t.Errorf("files left: %v", entries)
		}
	})
}

// importZip runs an import of a zip's bytes for a user.
func (f *fixture) importZip(user string, b []byte, keep bool) (ownership.Import, error) {
	f.t.Helper()
	ctx := context.Background()
	im, err := f.svc.StartImport(ctx, user, bytes.NewReader(b), "export.zip", keep)
	if err != nil {
		return im, err
	}
	_, err = f.runner.RunDue(ctx)
	must(f.t, err)
	list, err := f.svc.Imports(ctx, user)
	must(f.t, err)
	for _, i := range list {
		if i.ID == im.ID {
			return i, nil
		}
	}
	f.t.Fatal("the import went missing")
	return im, nil
}

func zipBytes(t *testing.T, z *zip.Reader, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range z.File {
		if _, skip := extra[f.Name]; skip {
			continue
		}
		r, err := f.Open()
		must(t, err)
		out, err := w.Create(f.Name)
		must(t, err)
		_, err = io.Copy(out, r)
		must(t, err)
	}
	for name, body := range extra {
		if body == "" {
			continue
		}
		out, err := w.Create(name)
		must(t, err)
		_, err = out.Write([]byte(body))
		must(t, err)
	}
	must(t, w.Close())
	return buf.Bytes()
}

func TestImportRoundTrip(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		f.fill()
		owner := f.users["owner"]
		z := f.export("owner", ownership.JSON)
		// A name that climbs out of the folder is only ever a name.
		b := zipBytes(t, z, map[string]string{"attachments/x/../../../escape.txt": "nope"})

		im, err := f.importZip(owner, b, false)
		must(t, err)
		if im.Status != "ready" || !im.WorkspaceID.Valid || im.WorkspaceName != "Team: Alpha" {
			t.Fatalf("import %+v", im)
		}
		var rep ownership.Report
		must(t, json.Unmarshal([]byte(im.Report), &rep))
		if rep.Restored["task"] != 6 || rep.Files != 1 || rep.Dropped["task_assignee"] != 1 {
			t.Errorf("report %+v", rep)
		}
		if _, err := os.Stat(filepath.Join(f.dir, "..", "escape.txt")); err == nil {
			t.Error("a zip entry was written outside")
		}
		list, err := f.st.ListWorkspacesForUser(ctx, owner)
		must(t, err)
		if len(list) != 2 {
			t.Fatalf("workspaces after import: %+v", list)
		}
		if !slices.Contains(f.told.changed, owner) || !slices.Contains(f.told.kinds, notify.ImportDone) {
			t.Error("nobody was told")
		}
		before, err := f.sync.Bootstrap(ctx, owner, f.ws)
		must(t, err)
		after, err := f.sync.Bootstrap(ctx, owner, im.WorkspaceID.String)
		must(t, err)
		count := func(rows []syncer.Row) map[string]int {
			out := map[string]int{}
			for _, r := range rows {
				out[r.Entity]++
			}
			return out
		}
		want := count(before.Rows)
		delete(want, "task_assignee") // the member is not in the new workspace
		if got := count(after.Rows); !maps(got, want) {
			t.Errorf("imported %v, want %v", got, want)
		}
		for _, r := range after.Rows {
			if r.Entity == "attachment" {
				file, err := f.files.Open(r.Fields["storage_key"].(string))
				must(t, err)
				got, _ := io.ReadAll(file)
				_ = file.Close()
				if string(got) != "hello file" {
					t.Errorf("attachment holds %q", got)
				}
			}
		}

		// Keeping IDs that are still here fails, and leaves nothing behind.
		im, err = f.importZip(owner, b, true)
		must(t, err)
		if im.Status != "failed" || !strings.Contains(im.Error.String, "new IDs") || im.WorkspaceID.Valid {
			t.Errorf("a clashing import: %+v", im)
		}
		var n int
		must(t, d.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&n))
		if n != 2 {
			t.Errorf("%d workspaces after a failed import, want 2", n)
		}
		if entries, _ := os.ReadDir(filepath.Join(f.dir, "imports")); len(entries) != 0 {
			t.Errorf("uploads left: %v", entries)
		}
	})
}

func maps(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestImportRefusesWhatIsNotAnExport(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 64<<10)
		owner := f.users["owner"]
		f.fill()
		z := f.export("owner", ownership.JSON)
		start := func(b []byte) error {
			_, err := f.svc.StartImport(ctx, owner, bytes.NewReader(b), "x.zip", false)
			return err
		}
		if err := start([]byte("not a zip")); apperr.KindOf(err) != apperr.Invalid {
			t.Errorf("not a zip: %v", err)
		}
		if err := start(zipBytes(t, z, map[string]string{"manifest.json": ""})); apperr.KindOf(err) != apperr.Invalid {
			t.Errorf("no manifest: %v", err)
		}
		newer := `{"format":"mila-export","version":99}`
		if err := start(zipBytes(t, z, map[string]string{"manifest.json": newer})); err == nil || !strings.Contains(err.Error(), "newer Mila") {
			t.Errorf("a newer format: %v", err)
		}
		csvZip := f.export("owner", ownership.CSV)
		if err := start(zipBytes(t, csvZip, nil)); apperr.KindOf(err) != apperr.Invalid {
			t.Errorf("a CSV export: %v", err)
		}
		if err := start(bytes.Repeat([]byte("x"), 65<<10)); err != ownership.ErrTooLarge {
			t.Errorf("too large: %v", err)
		}
		// A damaged workspace.json fails the import with a reason.
		im, err := f.importZip(owner, zipBytes(t, z, map[string]string{"workspace.json": "{nope"}), false)
		must(t, err)
		if im.Status != "failed" || !strings.Contains(im.Error.String, "damaged") {
			t.Errorf("a damaged export: %+v", im)
		}
		// A small zip cannot unpack into more rows than its size allows.
		rows := `{"rows":[` + strings.Repeat(`{"entity":"task","id":"x"},`, 20_000) + `{"entity":"task","id":"x"}]}`
		bomb := zipBytes(t, z, map[string]string{"workspace.json": rows})
		if len(bomb) > 64<<10 {
			t.Fatalf("the test zip is %d bytes", len(bomb))
		}
		im, err = f.importZip(owner, bomb, false)
		must(t, err)
		if im.Status != "failed" || !strings.Contains(im.Error.String, "more rows") {
			t.Errorf("a zip of too many rows: %+v", im)
		}
		// Nor into many times its size: 40 MB of zeros zip into 40 KB, and
		// would take 300 MB in memory.
		zeros := `{"rows":[` + strings.Repeat(`{"entity":"task","id":"x","fields":{"title":[0`+strings.Repeat(",0", 1_750_000)+`]}},`, 12) + `{"entity":"task","id":"x"}]}`
		dense := zipBytes(t, z, map[string]string{"workspace.json": zeros})
		if len(dense) > 64<<10 {
			t.Fatalf("the test zip is %d bytes", len(dense))
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		im, err = f.importZip(owner, dense, false)
		runtime.ReadMemStats(&after)
		must(t, err)
		if im.Status != "failed" || !strings.Contains(im.Error.String, "zip of its size") && !strings.Contains(im.Error.String, "larger than an import reads") {
			t.Errorf("a zip that unpacks to 40 MB of zeros: %+v", im)
		}
		if took := after.TotalAlloc - before.TotalAlloc; took > 64<<20 {
			t.Errorf("importing a zip of %d KB took %d MB", len(dense)>>10, took>>20)
		}
		// One import at a time.
		if _, err := f.svc.StartImport(ctx, owner, bytes.NewReader(zipBytes(t, z, nil)), "x.zip", false); err != nil {
			t.Fatal(err)
		}
		if err := start(zipBytes(t, z, nil)); apperr.KindOf(err) != apperr.Conflict {
			t.Errorf("a second import at once: %v", err)
		}
	})
}

// An import the server died under (it may be what killed it) is not
// tried again: it fails, and its upload and hidden workspace go.
func TestImportNotRetriedAfterACrash(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		f.fill()
		owner := f.users["owner"]
		z := f.export("owner", ownership.JSON)
		im, err := f.svc.StartImport(ctx, owner, bytes.NewReader(zipBytes(t, z, nil)), "x.zip", false)
		must(t, err)
		_, err = d.ExecContext(ctx, rebind(d, "UPDATE imports SET status = 'running' WHERE id = ?"), im.ID)
		must(t, err)
		_, err = f.runner.RunDue(ctx)
		must(t, err)
		list, err := f.svc.Imports(ctx, owner)
		must(t, err)
		if len(list) != 1 || list[0].Status != "failed" || !strings.Contains(list[0].Error.String, "stopped partway") {
			t.Fatalf("imports after a crash: %+v", list)
		}
		if entries, _ := os.ReadDir(filepath.Join(f.dir, "imports")); len(entries) != 0 {
			t.Errorf("uploads left: %v", entries)
		}
	})
}

// An export names people, but nothing proves it came from this instance,
// so an import credits their work to the importer: anyone could write one
// naming someone they want to speak for. A deleted account's work stays
// with "Deleted user". And an export gives the address of current members
// only: people who left are named without one.
func TestImportTrustsNoNames(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		f.fill()
		member, guest := f.users["member"], f.users["guest"]
		gone := uuid.NewString()
		must(t, f.st.CreateUser(ctx, sqlitedb.CreateUserParams{ID: gone, Email: "gone@example.com", Name: "Gone", Now: store.Now()}))
		must(t, f.st.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: f.ws, UserID: gone, Role: "member", Now: store.Now()}))
		f.users["gone"] = gone
		f.push("gone", "comment", f.id("farewell"), map[string]any{"task_id": f.id("task"), "author_id": gone, "body": "Bye"})
		_, err := d.ExecContext(ctx, rebind(d, "UPDATE users SET deleted_at = ? WHERE id = ?"), store.Now(), gone)
		must(t, err)
		must(t, f.st.DeleteMembership(ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: f.ws, UserID: gone}))
		must(t, f.st.DeleteMembership(ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: f.ws, UserID: member}))

		z := f.export("owner", ownership.JSON)
		var c ownership.Content
		must(t, json.Unmarshal([]byte(read(t, z, "workspace.json")), &c))
		named := map[string]ownership.Person{}
		for _, p := range c.People {
			named[p.ID] = p
		}
		if named[f.users["owner"]].Email == "" || named[member].Name != "Member" || named[member].Email != "" || !named[gone].Deleted {
			t.Errorf("people %+v", c.People)
		}

		// The guest imports it: the member's comment and history become the
		// guest's, the deleted account's comment stays theirs.
		im, err := f.importZip(guest, zipBytes(t, z, nil), false)
		must(t, err)
		if im.Status != "ready" {
			t.Fatalf("import %+v", im)
		}
		var rep ownership.Report
		must(t, json.Unmarshal([]byte(im.Report), &rep))
		if rep.Credited != 1 {
			t.Errorf("credited %d rows, want the member's comment", rep.Credited)
		}
		snap, err := f.sync.Bootstrap(ctx, guest, im.WorkspaceID.String)
		must(t, err)
		authors := map[string]string{}
		for _, r := range snap.Rows {
			if r.Entity == "comment" {
				authors[r.Fields["body"].(string)] = r.Fields["author_id"].(string)
			}
		}
		if authors["Bye"] != gone || authors["Done, @[Owner](user:"+f.users["owner"]+")"] != guest {
			t.Errorf("comment authors %v", authors)
		}

		// Its export, the guest's as owner, says nothing of the member.
		e, err := f.svc.StartExport(ctx, guest, im.WorkspaceID.String, ownership.JSON)
		must(t, err)
		_, err = f.runner.RunDue(ctx)
		must(t, err)
		file, _, err := f.svc.OpenExport(ctx, guest, e.ID)
		must(t, err)
		b, err := io.ReadAll(file)
		_ = file.Close()
		must(t, err)
		z, err = zip.NewReader(bytes.NewReader(b), int64(len(b)))
		must(t, err)
		body := read(t, z, "workspace.json")
		c = ownership.Content{}
		must(t, json.Unmarshal([]byte(body), &c))
		if strings.Contains(body, "member@example.com") {
			t.Error("the guest's export gives the member's address")
		}
		for _, p := range c.People {
			if p.ID == member {
				t.Errorf("the guest's export names the member: %+v", p)
			}
		}
		for _, h := range c.Activity {
			if h.ActorID == member {
				t.Errorf("history credited to the member: %+v", h)
			}
		}
	})
}

func TestImportBringsBackPagesAndTheirHistory(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		f := newFixture(t, d, 1<<20)
		f.fill()
		owner := f.users["owner"]
		main := syncer.MainPageID(f.id("shared"))
		f.push("owner", "page", main, map[string]any{"project_id": f.id("shared"), "parent_id": nil, "title": "", "body": "First.", "sort_key": "a0"})
		time.Sleep(2 * time.Millisecond) // a later clock, or the second write is not the newer
		f.push("owner", "page", main, map[string]any{"body": "Second."})
		z := f.export("owner", ownership.JSON)
		if !strings.Contains(read(t, z, "workspace.json"), `"pageVersions"`) {
			t.Fatal("the export holds no page history")
		}
		im, err := f.importZip(owner, zipBytes(t, z, nil), false)
		must(t, err)
		var rep ownership.Report
		must(t, json.Unmarshal([]byte(im.Report), &rep))
		if rep.Restored["page"] != 1 || rep.Versions != 2 {
			t.Errorf("report %+v", rep)
		}
		snap, err := f.sync.Bootstrap(ctx, owner, im.WorkspaceID.String)
		must(t, err)
		var pageID string
		docs := 0
		for _, r := range snap.Rows {
			switch r.Entity {
			case "page":
				pageID = r.ID
			case "page_update":
				docs++
			}
		}
		if docs == 0 {
			t.Error("the page came back without its document, so no one could edit it")
		}
		versions, err := f.sync.PageVersions(ctx, owner, im.WorkspaceID.String, pageID)
		must(t, err)
		if len(versions) != 2 || versions[0].Body != "Second." || versions[1].Body != "First." {
			t.Errorf("history after the import: %+v", versions)
		}
	})
}
