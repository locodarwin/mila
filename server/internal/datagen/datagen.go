// Package datagen builds test workspaces of any size for the performance
// budgets: projects with their statuses, labels,
// a select field, tasks with subtasks, checklists, comments, assignees,
// due dates, repeats and My Day entries, and one project of board cards.
// Every synced row goes in through syncer.Service.Push, as a device's
// edits would, so the change log, the clocks and the task history are
// all real. What is made follows from the seed; IDs are new on each run,
// so one database can hold several generated workspaces.
package datagen

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Spec is what to generate. Zero values take the defaults below.
type Spec struct {
	// Email names the owner, who is made (with a personal workspace) if
	// nobody has that address yet; the first user of an instance is its
	// admin. Name is the owner's name when they are made.
	Email, Name string
	// Workspace is the new workspace's name.
	Workspace string
	// Tasks counts every task made, subtasks and board cards included.
	Tasks int
	// Projects counts the list projects; the board project is one more.
	Projects int
	// BoardCards is how many of the tasks are cards in the board project.
	BoardCards int
	// Members are other people in the workspace, to assign and comment.
	// Negative means none.
	Members int
	// Seed decides everything made, apart from IDs.
	Seed uint64
	// Today is the date due dates gather around; zero means today in UTC.
	Today time.Time
}

// Defaults.
const (
	DefaultTasks      = 5000
	DefaultProjects   = 12
	DefaultBoardCards = 500
	DefaultMembers    = 4
	DefaultWorkspace  = "Generated"
)

// Result says what was made.
type Result struct {
	OwnerID     string
	WorkspaceID string
	// BoardProjectID is the project holding the board cards.
	BoardProjectID string
	// Rows counts the rows made, by entity.
	Rows map[string]int
	// Ops counts the ops pushed.
	Ops int
}

// pushBatch is the most ops one push takes.
const pushBatch = 500

func (s *Spec) fill() error {
	if s.Email == "" {
		return errors.New("an owner's email address is needed")
	}
	s.Email = strings.ToLower(strings.TrimSpace(s.Email))
	if s.Name == "" {
		s.Name = "Perf Owner"
	}
	if s.Workspace == "" {
		s.Workspace = DefaultWorkspace
	}
	if s.Tasks == 0 {
		s.Tasks = DefaultTasks
	}
	if s.Projects == 0 {
		s.Projects = DefaultProjects
	}
	if s.BoardCards == 0 {
		s.BoardCards = min(DefaultBoardCards, s.Tasks)
	}
	if s.Members == 0 {
		s.Members = DefaultMembers
	}
	if s.Members < 0 {
		s.Members = 0
	}
	if s.Today.IsZero() {
		s.Today = time.Now().UTC()
	}
	switch {
	case s.Tasks < 0, s.Projects < 0, s.BoardCards < 0:
		return errors.New("counts must not be negative")
	case s.BoardCards > s.Tasks:
		return fmt.Errorf("%d board cards is more than the %d tasks", s.BoardCards, s.Tasks)
	}
	return nil
}

// Generate makes a workspace as spec describes.
func Generate(ctx context.Context, st *store.Store, sync *syncer.Service, spec Spec) (Result, error) {
	if err := spec.fill(); err != nil {
		return Result{}, err
	}
	g := &gen{
		spec:   spec,
		rnd:    rand.New(rand.NewPCG(spec.Seed, spec.Seed^0x6d696c61)),
		sync:   sync,
		res:    Result{Rows: map[string]int{}},
		queued: map[string][]syncer.Op{},
		// Clocks start an hour back, so every op is in the past when it
		// arrives (see hlc).
		clock: time.Now().Add(-time.Hour).UnixMilli(),
	}
	if err := g.people(ctx, st); err != nil {
		return Result{}, err
	}
	g.workspace()
	for _, who := range append([]string{g.res.OwnerID}, g.members...) {
		if err := g.flush(ctx, who); err != nil {
			return Result{}, err
		}
	}
	return g.res, nil
}

type gen struct {
	spec    Spec
	rnd     *rand.Rand
	sync    *syncer.Service
	res     Result
	members []string
	// queued holds each person's ops, pushed in order once everything is
	// made: the owner's first, since others' comments need the tasks.
	queued map[string][]syncer.Op
	clock  int64 // the first op's milliseconds
	clocks int   // HLCs handed out
	keys   int   // sort keys handed out
}

// people finds or makes the owner and the members, and the workspace.
func (g *gen) people(ctx context.Context, st *store.Store) error {
	return st.InTx(ctx, func(q store.Queries) error {
		owner, err := user(ctx, q, g.spec.Email, g.spec.Name)
		if err != nil {
			return err
		}
		g.res.OwnerID = owner
		for i := range g.spec.Members {
			id, err := user(ctx, q, fmt.Sprintf("member%d.%s", i+1, g.spec.Email), memberNames[i%len(memberNames)])
			if err != nil {
				return err
			}
			g.members = append(g.members, id)
		}
		now := store.Now()
		ws := uuid.Must(uuid.NewV7()).String()
		if err := q.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{
			ID: ws, Name: g.spec.Workspace, Slug: "generated-" + ws[len(ws)-12:], Now: now,
		}); err != nil {
			return err
		}
		g.res.WorkspaceID = ws
		if err := q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: ws, UserID: owner, Role: "owner", Now: now}); err != nil {
			return err
		}
		for _, m := range g.members {
			if err := q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: ws, UserID: m, Role: "member", Now: now}); err != nil {
				return err
			}
		}
		return nil
	})
}

// user returns the ID of the user with this address, making them, with a
// personal workspace, if there is none.
func user(ctx context.Context, q store.Queries, email, name string) (string, error) {
	u, err := q.GetUserByEmail(ctx, email)
	if err == nil {
		return u.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	n, err := q.CountUsers(ctx)
	if err != nil {
		return "", err
	}
	now := store.Now()
	id := uuid.Must(uuid.NewV7()).String()
	if err := q.CreateUser(ctx, sqlitedb.CreateUserParams{ID: id, Email: email, Name: name, IsInstanceAdmin: n == 0, Now: now}); err != nil {
		return "", err
	}
	ws := uuid.Must(uuid.NewV7()).String()
	if err := q.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{
		ID: ws, Name: "Personal", Slug: "personal-" + ws[len(ws)-12:], IsPersonal: true, Now: now,
	}); err != nil {
		return "", err
	}
	return id, q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: ws, UserID: id, Role: "owner", Now: now})
}

// op queues one op by who.
func (g *gen) op(who, entity, id string, fields map[string]any) {
	raw := make(map[string]json.RawMessage, len(fields))
	for k, v := range fields {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err) // only plain values are passed
		}
		raw[k] = b
	}
	g.queued[who] = append(g.queued[who], syncer.Op{
		OpID: uuid.NewString(), Entity: entity, ID: id, HLC: g.hlc(), Fields: raw,
	})
	g.res.Rows[entity]++
}

// flush pushes who's ops in batches; any refused op fails the run.
func (g *gen) flush(ctx context.Context, who string) error {
	ops := g.queued[who]
	for start := 0; start < len(ops); start += pushBatch {
		batch := ops[start:min(start+pushBatch, len(ops))]
		results, _, err := g.sync.Push(ctx, who, g.res.WorkspaceID, batch)
		if err != nil {
			return err
		}
		for i, r := range results {
			if r.Status != "applied" {
				return fmt.Errorf("%s %s was %s: %s", batch[i].Entity, batch[i].ID, r.Status, r.Message)
			}
		}
		g.res.Ops += len(batch)
	}
	delete(g.queued, who)
	return nil
}

// hlc is the next op's clock: sixteen ops to a millisecond, told apart by
// the HLC's counter, so they sort in the order made. An hour holds 57.6
// million ops before one would be in the future, where the server's
// clock check refuses it.
func (g *gen) hlc() string {
	n := g.clocks
	g.clocks++
	return fmt.Sprintf("%013d-%04x-%s", g.clock+int64(n/16), n%16, "datagen")
}

// key is the next sort key: an integer of the app's fractional ordering
// (web/src/lib/sync/sortkey.ts), whose first character says how many
// base-62 digits follow. They start at "c000" and, after "czzz", go on
// with "d0000", one digit longer, and so on, so they sort in the order
// made however many there are.
func (g *gen) key() string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	n := g.keys
	g.keys++
	head, width := byte('c'), 3
	for size := 62 * 62 * 62; n >= size; size *= 62 {
		n -= size
		head++
		width++
	}
	b := make([]byte, 1+width)
	b[0] = head
	for i := width; i > 0; i-- {
		b[i] = digits[n%62]
		n /= 62
	}
	return string(b)
}

func (g *gen) chance(p float64) bool { return g.rnd.Float64() < p }

func pick[T any](g *gen, list []T) T { return list[g.rnd.IntN(len(list))] }

func newID() string { return uuid.Must(uuid.NewV7()).String() }

type project struct {
	id       string
	statuses map[string]string // category -> status ID
	order    []string          // status IDs in board order
	tasks    []string          // top-level tasks, for subtasks to hang off
}

// workspace queues every row of the workspace.
func (g *gen) workspace() {
	owner := g.res.OwnerID
	s := g.spec

	var labels []string
	for i, name := range labelNames {
		id := newID()
		labels = append(labels, id)
		g.op(owner, "label", id, map[string]any{"name": name, "color": colors[i%len(colors)], "sort_key": g.key()})
	}

	var projects []*project
	for i := range s.Projects + 1 {
		board := i == s.Projects
		name := projectNames[i%len(projectNames)]
		if i >= len(projectNames) {
			name = fmt.Sprintf("%s %d", name, i/len(projectNames)+1)
		}
		if board {
			name = "Release board"
		}
		p := &project{id: newID(), statuses: map[string]string{}}
		desc := ""
		if g.chance(0.5) || board {
			desc = sentence(g, 12)
		}
		g.op(owner, "project", p.id, map[string]any{
			"name": name, "icon": "", "color": colors[g.rnd.IntN(len(colors))], "description": desc, "sort_key": g.key(),
		})
		for _, st := range starterStatuses {
			id := newID()
			p.statuses[st.category] = id
			p.order = append(p.order, id)
			g.op(owner, "status", id, map[string]any{"project_id": p.id, "name": st.name, "category": st.category, "sort_key": g.key()})
		}
		projects = append(projects, p)
	}
	board := projects[len(projects)-1]
	g.res.BoardProjectID = board.id

	// A select field on the board project, set on most of its cards.
	field := newID()
	var options []map[string]string
	for i, name := range []string{"Web", "Server", "Docs", "Design"} {
		options = append(options, map[string]string{"id": newID(), "name": name, "color": colors[i*3%len(colors)]})
	}
	g.op(owner, "custom_field", field, map[string]any{"project_id": board.id, "name": "Area", "type": "select", "options": options, "sort_key": g.key()})

	people := append([]string{owner}, g.members...)
	today := time.Date(s.Today.Year(), s.Today.Month(), s.Today.Day(), 0, 0, 0, 0, time.UTC)
	tasks := make([]string, 0, s.Tasks)
	for n := range s.Tasks {
		var p *project
		switch {
		case n < s.BoardCards:
			p = board
		case len(projects) > 1 && !g.chance(0.08):
			p = projects[g.rnd.IntN(len(projects)-1)]
		}
		id := newID()
		tasks = append(tasks, id)
		task := map[string]any{
			"title": title(g), "sort_key": g.key(), "priority": pick(g, priorities),
			"created_by": owner, "recurrence_mode": "schedule",
		}
		done := g.chance(0.2)
		if p != nil {
			task["project_id"] = p.id
			category := "todo"
			switch {
			case done:
				category = "done"
			case p == board:
				category = pick(g, []string{"backlog", "todo", "todo", "in-progress", "in-progress"})
			case g.chance(0.15):
				category = "in-progress"
			}
			task["status_id"] = p.statuses[category]
			// Some tasks in list projects are subtasks of an earlier one.
			if p != board && len(p.tasks) > 0 && g.chance(0.12) {
				task["parent_id"] = pick(g, p.tasks)
			} else {
				p.tasks = append(p.tasks, id)
			}
		}
		if done {
			task["completed_at"] = today.Add(-time.Duration(g.rnd.IntN(60*24)) * time.Hour).Format(time.RFC3339)
		}
		if g.chance(0.3) {
			task["notes"] = paragraph(g)
		}
		if g.chance(0.4) {
			due := today.AddDate(0, 0, g.rnd.IntN(90)-30)
			task["due_date"] = due.Format(time.DateOnly)
			if g.chance(0.25) {
				task["due_time"] = fmt.Sprintf("%02d:%02d", 8+g.rnd.IntN(10), 15*g.rnd.IntN(4))
			}
			if g.chance(0.2) {
				task["start_date"] = due.AddDate(0, 0, -g.rnd.IntN(7)).Format(time.DateOnly)
			}
			if !done && g.chance(0.04) {
				task["recurrence"] = pick(g, repeats)
				task["series_id"] = id
			}
		}
		g.op(owner, "task", id, task)

		for _, l := range distinct(g, labels, g.rnd.IntN(3)) {
			g.op(owner, "task_label", syncer.PairID(id, l), map[string]any{"task_id": id, "label_id": l})
		}
		if g.chance(0.3) {
			for _, u := range distinct(g, people, 1+g.rnd.IntN(2)) {
				g.op(owner, "task_assignee", syncer.PairID(id, u), map[string]any{"task_id": id, "user_id": u})
			}
		}
		if g.chance(0.15) {
			for range 1 + g.rnd.IntN(5) {
				g.op(owner, "checklist_item", newID(), map[string]any{
					"task_id": id, "text": title(g), "done": g.chance(0.4), "sort_key": g.key(),
				})
			}
		}
		if g.chance(0.1) {
			for range 1 + g.rnd.IntN(3) {
				who := pick(g, people)
				g.op(who, "comment", newID(), map[string]any{"task_id": id, "author_id": who, "body": paragraph(g)})
			}
		}
		if !done && g.chance(0.02) {
			g.op(owner, "my_day_entry", syncer.PairID(id, owner), map[string]any{
				"task_id": id, "user_id": owner, "day": today.Format(time.DateOnly), "sort_key": g.key(),
			})
		}
		if p == board && g.chance(0.7) {
			g.op(owner, "custom_field_value", syncer.PairID(id, field), map[string]any{
				"task_id": id, "field_id": field, "value": pick(g, options)["id"],
			})
		}
	}

	// A saved view across the workspace, shared with everyone.
	g.op(owner, "saved_view", newID(), map[string]any{
		"name": "Open work", "type": "list", "config": map[string]any{"filter": map[string]any{"showDone": false}}, "sort_key": g.key(),
	})

	// Two workspace-wide fields, for sorting and filtering saved views on
	// them. They come last and draw from a stream of their own, so every
	// row above is the same for a seed as before they existed.
	rnd := rand.New(rand.NewPCG(s.Seed+1, (s.Seed+1)^0x6d696c61))
	estimate, reviewBy := newID(), newID()
	g.op(owner, "custom_field", estimate, map[string]any{"project_id": nil, "name": "Estimate", "type": "number", "options": []any{}, "sort_key": g.key()})
	g.op(owner, "custom_field", reviewBy, map[string]any{"project_id": nil, "name": "Review by", "type": "date", "options": []any{}, "sort_key": g.key()})
	for _, id := range tasks {
		if rnd.Float64() < 0.3 {
			g.op(owner, "custom_field_value", syncer.PairID(id, estimate), map[string]any{
				"task_id": id, "field_id": estimate, "value": []int{1, 2, 3, 5, 8, 13}[rnd.IntN(6)],
			})
		}
		if rnd.Float64() < 0.3 {
			g.op(owner, "custom_field_value", syncer.PairID(id, reviewBy), map[string]any{
				"task_id": id, "field_id": reviewBy, "value": today.AddDate(0, 0, rnd.IntN(120)-30).Format(time.DateOnly),
			})
		}
	}
}

// distinct picks n different items, or all of them.
func distinct[T any](g *gen, list []T, n int) []T {
	if n >= len(list) {
		n = len(list)
	}
	idx := g.rnd.Perm(len(list))[:n]
	out := make([]T, n)
	for i, j := range idx {
		out[i] = list[j]
	}
	return out
}

func title(g *gen) string {
	t := pick(g, verbs) + " " + pick(g, objects)
	if g.chance(0.4) {
		t += " " + pick(g, tails)
	}
	return t
}

func sentence(g *gen, words int) string {
	var b strings.Builder
	for i := range 4 + g.rnd.IntN(words) {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(pick(g, filler))
	}
	s := b.String()
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func paragraph(g *gen) string {
	parts := make([]string, 1+g.rnd.IntN(4))
	for i := range parts {
		parts[i] = sentence(g, 14)
	}
	return strings.Join(parts, " ")
}

var (
	starterStatuses = []struct{ name, category string }{
		{"Backlog", "backlog"}, {"To do", "todo"}, {"In progress", "in-progress"}, {"Done", "done"}, {"Canceled", "canceled"},
	}
	priorities   = []string{"none", "none", "none", "low", "medium", "high", "urgent"}
	colors       = []string{"red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}
	repeats      = []string{"FREQ=DAILY", "FREQ=WEEKLY;BYDAY=MO", "FREQ=WEEKLY;INTERVAL=2;BYDAY=FR", "FREQ=MONTHLY;BYMONTHDAY=1"}
	labelNames   = []string{"bug", "chore", "design", "docs", "errand", "finance", "health", "home", "idea", "urgent", "waiting", "writing"}
	memberNames  = []string{"Ada Park", "Ben Ortiz", "Chloe Brandt", "Dev Raman", "Eli Novak", "Fay Moreau"}
	projectNames = []string{"Garden", "Kitchen remodel", "Taxes", "Website", "Reading list", "Trip to Lisbon",
		"Car", "Newsletter", "Podcast", "Wedding", "Move house", "Side project", "Volunteering", "Health"}
	verbs   = []string{"Plan", "Fix", "Write", "Call", "Review", "Order", "Book", "Clean", "Update", "Draft", "Check", "Sort out", "Email", "Pay", "Measure", "Test", "Paint", "Ship"}
	objects = []string{"the fence", "invoice", "the release notes", "dentist", "quarterly report", "grocery list", "the backlog",
		"flights", "the gutter", "insurance renewal", "sign-up form", "slides", "the budget", "kitchen tiles", "the landing page", "bike tires"}
	tails  = []string{"before Friday", "with Sam", "for the team", "again", "this week", "after lunch", "if it rains", "for the launch"}
	filler = []string{"the", "a", "plan", "needs", "check", "with", "before", "after", "notes", "from", "last", "week", "and", "quote",
		"call", "back", "about", "price", "draft", "ready", "ask", "for", "maybe", "next", "meeting", "list", "of", "items", "done"}
)
