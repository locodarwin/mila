// Package automations runs people's rules: when something
// happens to a task, and it fits, the changes they chose. A change to a task is heard
// inside its transaction and only queues a job; the job then checks the rule against the
// task as it stands and makes the changes through syncer.Write, as whoever last saved
// the rule, marked as the rule's, so history and webhooks say so and no other rule starts.
package automations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/taskops"
)

const (
	jobRun = "automation.run"
	// A task that finished may free others, worked out once it has been saved.
	jobFreed = "automation.freed"

	// MaxRunsAnHour pauses a rule that runs more than this. A change to many
	// tasks at once (one batch) counts once, up to MaxBatchedRunsAnHour runs
	// in all, so that no batch can keep a rule running without end.
	MaxRunsAnHour        = 200
	MaxBatchedRunsAnHour = 2000
	// keepRuns is how long what a rule did stays listed.
	keepRuns = 30 * 24 * time.Hour
	// lookBack is how long a rule on due dates catches up, after the server was off
	// (the desktop app's is off whenever it's closed).
	lookBack = 7
	// lookAhead is the furthest ahead of a due date a rule can start.
	lookAhead = 60
	// CheckEvery is how often due dates are looked at.
	CheckEvery = 5 * time.Minute

	// What became of a run.
	Ran    = "ran"
	Partly = "partly"
	Failed = "failed"
)

// Notices is how a rule tells people.
type Notices interface {
	Send(ctx context.Context, notices ...notify.Notice) error
}

// Service runs rules.
type Service struct {
	st      *store.Store
	log     *slog.Logger
	sync    *syncer.Service
	jobs    *jobs.Runner
	notices Notices
	now     func() time.Time
}

// New makes the service and registers its jobs.
func New(st *store.Store, log *slog.Logger, sync *syncer.Service, runner *jobs.Runner, notices Notices) *Service {
	s := &Service{st: st, log: log, sync: sync, jobs: runner, notices: notices, now: store.Now}
	runner.Handle(jobRun, s.handleRun)
	runner.Handle(jobFreed, s.handleFreed)
	runner.Every("automation.due", CheckEvery, s.CheckDue)
	return s
}

// event is something that happened that a rule can start on.
type event struct {
	on, value, task string
}

// eventsOf says what an accepted change is, to a rule. A task made with a status or
// priority was made, and that's all: those are for a task that changes.
func eventsOf(a syncer.Applied) []event {
	changed := func(name string) bool {
		for _, n := range a.Accepted {
			if n == name {
				return true
			}
		}
		return false
	}
	text := func(name string) string {
		v, _ := a.Values[name].(string)
		return v
	}
	cameBack := a.Created || (changed("deleted_at") && a.Values["deleted_at"] == nil)
	switch a.Entity {
	case "task":
		if a.Created {
			// The next of a repeating task is the same work coming back.
			if text("series_id") != "" {
				return nil
			}
			out := []event{{on: syncer.AutomationCreated, task: a.ID}}
			if a.Emailed {
				out = append(out, event{on: syncer.AutomationEmail, task: a.ID})
			}
			return out
		}
		if a.Values["deleted_at"] != nil {
			return nil
		}
		var out []event
		if changed("completed_at") {
			if a.Values["completed_at"] != nil {
				out = append(out, event{on: syncer.AutomationCompleted, task: a.ID})
			} else {
				out = append(out, event{on: syncer.AutomationReopened, task: a.ID})
			}
		}
		if changed("status_id") && text("status_id") != "" {
			out = append(out, event{on: syncer.AutomationStatus, value: text("status_id"), task: a.ID})
		}
		if changed("priority") {
			out = append(out, event{on: syncer.AutomationPriority, value: text("priority"), task: a.ID})
		}
		return out
	case "task_assignee":
		if cameBack && a.Values["deleted_at"] == nil {
			return []event{{on: syncer.AutomationAssigned, value: text("user_id"), task: text("task_id")}}
		}
	case "task_label":
		if cameBack && a.Values["deleted_at"] == nil {
			return []event{{on: syncer.AutomationLabel, value: text("label_id"), task: text("task_id")}}
		}
	case "custom_field_value":
		if a.Values["deleted_at"] == nil && (a.Created || changed("value")) {
			return []event{{on: syncer.AutomationField, value: text("field_id"), task: text("task_id")}}
		}
	case "comment":
		if a.Created {
			return []event{{on: syncer.AutomationComment, task: text("task_id")}}
		}
	}
	return nil
}

// Applied queues the rules a change starts, in the change's transaction.
func (s *Service) Applied(ctx context.Context, q store.Queries, a syncer.Applied) error {
	if a.Automated || a.Bulk || a.FromDoc {
		return nil
	}
	events := eventsOf(a)
	if len(events) == 0 {
		return nil
	}
	rules, err := q.ListLiveAutomations(ctx, a.WorkspaceID)
	if err != nil || len(rules) == 0 {
		return err
	}
	freed := a.Entity == "task" && !a.Created && containsName(a.Accepted, "completed_at") && a.Values["completed_at"] != nil
	project := sql.NullString{}
	if a.Entity == "task" {
		if p, ok := a.Values["project_id"].(string); ok {
			project = sql.NullString{String: p, Valid: true}
		}
	} else {
		project, err = q.AutomationTaskProject(ctx, events[0].task)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	for _, r := range rules {
		rule, err := syncer.ParseAutomationRule(r.Rule)
		if err != nil {
			s.log.Warn("automation: unreadable rule", "rule", r.ID, "err", err)
			continue
		}
		// Finishing a task can free tasks waiting for it in any project. The run
		// checks each of them against where the rule works.
		if freed && rule.When.On == syncer.AutomationUnblocked {
			if err := jobs.Enqueue(ctx, q, jobFreed, freedPayload{Rule: r.ID, Done: a.ID, Actor: a.ActorID, Batch: a.Batch}, a.At, a.At); err != nil {
				return err
			}
		}
		if r.ProjectID.Valid && r.ProjectID != project {
			continue
		}
		for _, e := range events {
			if s.starts(ctx, q, rule, e) {
				if err := jobs.Enqueue(ctx, q, jobRun, runPayload{Rule: r.ID, Task: e.task, Actor: a.ActorID, Run: uuid.NewString(), Batch: a.Batch}, a.At, a.At); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// starts says whether an event is what a rule's When asks for.
func (s *Service) starts(ctx context.Context, q store.Queries, rule syncer.AutomationRule, e event) bool {
	if rule.When.On != e.on {
		return false
	}
	want := rule.When.Value
	if want == "" {
		return true
	}
	if e.on == syncer.AutomationStatus {
		if kind, ok := strings.CutPrefix(want, kindPrefix); ok {
			category, err := q.AutomationStatusCategory(ctx, e.value)
			return err == nil && category == kind
		}
	}
	return want == e.value
}

const kindPrefix = taskops.KindPrefix

// Committed wakes the runner for what was queued.
func (s *Service) Committed() { s.jobs.Wake() }

type runPayload struct {
	Rule, Task, Actor, Run string
	// Batch is the change to many tasks that started it, if one did.
	Batch string `json:",omitempty"`
}

type freedPayload struct {
	Rule, Done, Actor string
	Batch             string `json:",omitempty"`
}

// handleFreed runs a rule on each task that a finished one was the last to hold up.
func (s *Service) handleFreed(ctx context.Context, payload []byte) error {
	var p freedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	waiting, err := s.sync.Waiting(ctx, p.Done)
	if err != nil {
		return err
	}
	for _, w := range waiting {
		if w.Completed || w.Deleted {
			continue
		}
		blockers, err := s.sync.Blockers(ctx, w.ID)
		if err != nil {
			return err
		}
		free := true
		for _, b := range blockers {
			free = free && (b.Completed || b.Deleted)
		}
		if !free {
			continue
		}
		run := runPayload{Rule: p.Rule, Task: w.ID, Actor: p.Actor, Run: recur.HashID("freed\n" + p.Rule + "\n" + p.Done + "\n" + w.ID), Batch: p.Batch}
		if _, err := s.run(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleRun(ctx context.Context, payload []byte) error {
	var p runPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	_, err := s.run(ctx, p)
	return err
}

// live is a rule ready to run.
type live struct {
	id, workspace, name, maker string
	project                    sql.NullString
	rule                       syncer.AutomationRule
}

// load reads a rule that is on, and checks whoever it runs as still may. A rule that
// has become impossible pauses and says so. Reports false when there's nothing to run.
func (s *Service) load(ctx context.Context, id string) (live, bool, error) {
	r, err := s.st.GetLiveAutomation(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return live{}, false, nil
	}
	if err != nil {
		return live{}, false, err
	}
	rule, err := syncer.ParseAutomationRule(r.Rule)
	if err != nil {
		s.log.Warn("automation: unreadable rule", "rule", id, "err", err)
		return live{}, false, nil
	}
	l := live{id: r.ID, workspace: r.WorkspaceID, name: r.Name, maker: r.MadeBy.String, project: r.ProjectID, rule: rule}
	may := false
	if r.MadeBy.Valid {
		may, err = s.sync.AutomationActor(ctx, l.maker, l.workspace, !l.project.Valid)
		if err != nil {
			return live{}, false, err
		}
	}
	if !may {
		if !r.MadeBy.Valid {
			return live{}, false, nil
		}
		return live{}, false, s.pause(ctx, l, "Whoever saved this last can no longer run it. Save it to make it yours.", false)
	}
	return l, true, nil
}

// pause stops a rule and tells whoever saved it last, if they're still there to hear.
func (s *Service) pause(ctx context.Context, l live, why string, tell bool) error {
	if err := s.sync.PauseAutomation(ctx, l.workspace, l.id, l.maker, why); err != nil {
		return err
	}
	if tell && s.notices != nil {
		if err := s.notices.Send(ctx, notify.Notice{
			UserID: l.maker, WorkspaceID: l.workspace, Kind: notify.Automation,
			Title: "An automation stopped: " + l.name, Body: why,
		}); err != nil {
			s.log.Warn("automation: no notification", "rule", l.id, "err", err)
		}
	}
	return nil
}

// placeOf is where someone is, for what day it is.
func (s *Service) placeOf(ctx context.Context, userID string) (*time.Location, error) {
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return accounts.Location(u), nil
}

// run checks a rule against a task as it stands now, and makes its changes, and reports
// whether the rule is stopped, so that there's no use trying it on anything else. Doing it
// twice, as a job cut short or tried again is, changes nothing more: what a change makes
// has an ID from the run, and the rest sets what is already set. What went wrong that
// wasn't the rule's doing comes back as an error, for the job to try again.
func (s *Service) run(ctx context.Context, p runPayload) (stopped bool, err error) {
	l, ok, err := s.load(ctx, p.Rule)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	since := s.now().Add(-time.Hour)
	// The other changes this hour: this one, a batch's runs included, counts once more.
	n, err := s.st.CountAutomationChangesSince(ctx, sqlitedb.CountAutomationChangesSinceParams{AutomationID: l.id, Since: since, Batch: sql.NullString{String: p.Batch, Valid: true}})
	if err != nil {
		return false, err
	}
	all, err := s.st.CountAutomationRunsSince(ctx, sqlitedb.CountAutomationRunsSinceParams{AutomationID: l.id, Since: since})
	if err != nil {
		return false, err
	}
	if n >= MaxRunsAnHour || all >= MaxBatchedRunsAnHour {
		return true, s.pause(ctx, l, fmt.Sprintf("It ran %d times in an hour, so Mila stopped it. Turn it back on when you've looked at it.", max(n, all)), true)
	}
	task, there, err := s.sync.TaskFactsOf(ctx, p.Task)
	if err != nil || !there || task.Deleted || task.WorkspaceID != l.workspace {
		return false, err
	}
	if l.project.Valid && l.project.String != task.ProjectID {
		return false, nil
	}
	loc, err := s.placeOf(ctx, l.maker)
	if err != nil {
		return false, err
	}
	today := s.now().In(loc).Format(time.DateOnly)
	fits, err := s.sync.Fits(ctx, task, l.rule.Config, l.maker, today)
	if err != nil || !fits {
		return false, err
	}
	ctx = syncer.WithAutomation(ctx, l.name)
	did, failed, err := s.changes(ctx, l, task, p, today)
	if err != nil {
		return false, err
	}
	outcome := Ran
	switch {
	case len(failed) > 0 && len(did) > 0:
		outcome = Partly
	case len(failed) > 0:
		outcome = Failed
	}
	detail := strings.Join(append(did, failed...), ". ")
	if err := s.st.InsertAutomationRun(ctx, sqlitedb.InsertAutomationRunParams{
		ID: uuid.NewString(), WorkspaceID: l.workspace, AutomationID: l.id, TaskID: task.ID, TaskTitle: task.Title,
		Outcome: outcome, Detail: clip(detail, 1000), Now: store.Now(),
		Batch: sql.NullString{String: p.Batch, Valid: p.Batch != ""},
	}); err != nil {
		return false, err
	}
	if len(failed) > 0 && s.notices != nil {
		brief, there, err := s.sync.Task(ctx, task.ID)
		if err == nil && there {
			if err := s.notices.Send(ctx, notify.Notice{
				UserID: l.maker, WorkspaceID: l.workspace, Kind: notify.Automation, Task: &brief,
				Title: "An automation couldn't finish: " + l.name, Body: clip(strings.Join(failed, ". "), 200),
			}); err != nil {
				s.log.Warn("automation: no notification", "rule", l.id, "err", err)
			}
		}
	}
	return false, nil
}

func clip(s string, most int) string {
	r := []rune(s)
	if len(r) <= most {
		return s
	}
	return string(r[:most])
}

// changes makes a rule's changes in order, telling people last: everything else can be
// done again when something goes wrong, and telling someone can't. A change that can't be
// made doesn't stop the rest. Anything else that goes wrong does, for the job to try
// again, except in telling someone, which is only said.
func (s *Service) changes(ctx context.Context, l live, t syncer.TaskFacts, p runPayload, today string) (did, failed []string, err error) {
	for _, telling := range []bool{false, true} {
		for i, c := range l.rule.Then {
			if (c.Do == syncer.DoNotify) != telling {
				continue
			}
			said, err := s.change(ctx, l, &t, p, today, i, c)
			var refused *apperr.Error
			switch {
			case err == nil:
				if said != "" {
					did = append(did, said)
				}
			case errors.As(err, &refused):
				failed = append(failed, err.Error())
			case telling:
				s.log.Warn("automation: no notification", "rule", l.id, "task", t.ID, "err", err)
				failed = append(failed, "Someone couldn't be told")
			default:
				return nil, nil, err
			}
		}
	}
	return did, failed, nil
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func (s *Service) write(ctx context.Context, l live, w syncer.Write) error {
	_, _, err := s.sync.Write(ctx, l.maker, l.workspace, w)
	return err
}

// ops does a task's actions as whoever the rule runs as.
func (s *Service) ops(l live) taskops.Ops {
	return taskops.Ops{Sync: s.sync, Now: s.now, Write: func(ctx context.Context, w syncer.Write) error { return s.write(ctx, l, w) }}
}

// createOnce makes a row under an ID from its run, so a run done twice makes it once.
func (s *Service) createOnce(ctx context.Context, l live, entity, id string, fields map[string]json.RawMessage) error {
	return s.ops(l).CreateOnce(ctx, entity, id, fields)
}

func (s *Service) setTask(ctx context.Context, l live, id string, fields map[string]json.RawMessage) error {
	return s.ops(l).SetTask(ctx, id, fields)
}

// whoIs turns who a change names into a person.
func whoIs(value string, t syncer.TaskFacts, p runPayload) string {
	switch value {
	case syncer.WhoMadeTask:
		return t.CreatedBy
	case syncer.WhoDidThis:
		return p.Actor
	}
	return value
}

// nameOf is a person's name for what a rule did, if they're someone in its workspace.
func (s *Service) nameOf(ctx context.Context, workspaceID, userID string) string {
	if _, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID}); err != nil {
		return "someone"
	}
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return "someone"
	}
	return u.Name
}

// change makes one change, and says what it did.
func (s *Service) change(ctx context.Context, l live, t *syncer.TaskFacts, p runPayload, today string, index int, c syncer.AutomationChange) (string, error) {
	made := func() string { return recur.HashID(p.Run + "/" + strconv.Itoa(index)) }
	switch c.Do {
	case syncer.DoPriority:
		if err := s.setTask(ctx, l, t.ID, map[string]json.RawMessage{"priority": raw(c.Value)}); err != nil {
			return "", err
		}
		t.Priority = c.Value
		return "Set the priority to " + c.Value, nil
	case syncer.DoDue:
		day, err := dueDay(c.Value, today)
		if err != nil {
			return "", apperr.New(apperr.Invalid, "That due date isn't one Mila knows.")
		}
		if err := s.setTask(ctx, l, t.ID, map[string]json.RawMessage{"due_date": raw(day)}); err != nil {
			return "", err
		}
		t.DueDate = day
		return "Made it due " + day, nil
	case syncer.DoStatus:
		return s.ops(l).SetStatus(ctx, t, c.Value, today)
	case syncer.DoComplete:
		return s.ops(l).Complete(ctx, t, today)
	case syncer.DoAssign:
		who := whoIs(c.Value, *t, p)
		if who == "" {
			return "", apperr.New(apperr.Invalid, "There's nobody to assign.")
		}
		if err := s.write(ctx, l, syncer.Write{Entity: "task_assignee", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": raw(t.ID), "user_id": raw(who)}}); err != nil {
			return "", fmt.Errorf("%s wasn't assigned: %w", s.nameOf(ctx, l.workspace, who), err)
		}
		return "Assigned " + s.nameOf(ctx, l.workspace, who), nil
	case syncer.DoUnassign:
		for _, who := range t.Assignees {
			if err := s.write(ctx, l, syncer.Write{Entity: "task_assignee", ID: syncer.PairID(t.ID, who), Mode: syncer.Delete}); err != nil && apperr.KindOf(err) != apperr.NotFound {
				return "", err
			}
		}
		t.Assignees = nil
		return "Unassigned everyone", nil
	case syncer.DoAddLabel:
		if err := s.write(ctx, l, syncer.Write{Entity: "task_label", Mode: syncer.Upsert,
			Fields: map[string]json.RawMessage{"task_id": raw(t.ID), "label_id": raw(c.Value)}}); err != nil {
			return "", err
		}
		return "Added a label", nil
	case syncer.DoRemoveLabel:
		if err := s.write(ctx, l, syncer.Write{Entity: "task_label", ID: syncer.PairID(t.ID, c.Value), Mode: syncer.Delete}); err != nil && apperr.KindOf(err) != apperr.NotFound {
			return "", err
		}
		return "Removed a label", nil
	case syncer.DoMove:
		return s.ops(l).Move(ctx, l.workspace, t, c.Value)
	case syncer.DoComment:
		if err := s.createOnce(ctx, l, "comment", made(), map[string]json.RawMessage{"task_id": raw(t.ID), "body": raw(c.Value)}); err != nil {
			return "", err
		}
		return "Added a comment", nil
	case syncer.DoSubtask:
		fields := map[string]json.RawMessage{"title": raw(c.Value), "parent_id": raw(t.ID)}
		if t.ProjectID != "" {
			fields["project_id"] = raw(t.ProjectID)
		}
		if err := s.createOnce(ctx, l, "task", made(), fields); err != nil {
			return "", err
		}
		return "Added a subtask, " + c.Value, nil
	case syncer.DoNotify:
		who := whoIs(c.Value, *t, p)
		if who == "" || s.notices == nil {
			return "", apperr.New(apperr.Invalid, "There's nobody to notify.")
		}
		sees, err := s.sync.SeesProject(ctx, who, l.workspace, t.ProjectID)
		if err != nil {
			return "", err
		}
		if !sees {
			return "", apperr.New(apperr.Invalid, "%s can't see this task, so wasn't told.", s.nameOf(ctx, l.workspace, who))
		}
		brief, there, err := s.sync.Task(ctx, t.ID)
		if err != nil {
			return "", err
		}
		if !there {
			return "", nil
		}
		// The rule tells: not the person who saved it, so they're told too.
		// Rules are for members, so a guest is not told what one is called.
		title := l.name + ": " + t.Title
		if m, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: l.workspace, UserID: who}); err != nil {
			return "", err
		} else if authz.Role(m.Role) == authz.Guest {
			title = "An automation: " + t.Title
		}
		if err := s.notices.Send(ctx, notify.Notice{
			UserID: who, WorkspaceID: l.workspace, Kind: notify.Automation, Task: &brief,
			Title: title,
		}); err != nil {
			return "", err
		}
		return "Told " + s.nameOf(ctx, l.workspace, who), nil
	}
	return "", apperr.New(apperr.Invalid, "That change isn't one Mila knows.")
}

// dueDay works out a day a rule named: today, tomorrow, +3, or a weekday's next.
func dueDay(value, today string) (string, error) {
	day, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return "", err
	}
	switch value {
	case "today":
		return today, nil
	case "tomorrow":
		return day.AddDate(0, 0, 1).Format(time.DateOnly), nil
	case "next_monday":
		ahead := (int(time.Monday) - int(day.Weekday()) + 7) % 7
		if ahead == 0 {
			ahead = 7
		}
		return day.AddDate(0, 0, ahead).Format(time.DateOnly), nil
	}
	n, err := strconv.Atoi(strings.TrimPrefix(value, "+"))
	if err != nil || n < 0 || n > 3650 {
		return "", errors.New("not a day")
	}
	return day.AddDate(0, 0, n).Format(time.DateOnly), nil
}

// fireDay is the day a rule on a due date starts for a task due on a day: the day it
// passed for "overdue", or so many days ahead of it.
func fireDay(value, due string) (string, bool) {
	day, err := time.Parse(time.DateOnly, due)
	if err != nil {
		return "", false
	}
	if value == "overdue" {
		return day.AddDate(0, 0, 1).Format(time.DateOnly), true
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > lookAhead {
		return "", false
	}
	return day.AddDate(0, 0, -n).Format(time.DateOnly), true
}

// CheckDue starts rules on due dates: once for each date a task has, on the day it
// comes to that, or as soon as the server's next up. A rule acts from the day it was
// turned on, so turning one on changes nothing already overdue. A rule whose run goes
// wrong gets another go next time, and takes nobody else's with it.
func (s *Service) CheckDue(ctx context.Context) error {
	rules, err := s.st.ListAllLiveAutomations(ctx)
	if err != nil {
		return err
	}
	var tasks []syncer.TaskBrief
	loaded := false
	now := s.now()
	for _, r := range rules {
		rule, err := syncer.ParseAutomationRule(r.Rule)
		if err != nil || rule.When.On != syncer.AutomationDue || !r.MadeBy.Valid {
			continue
		}
		if !loaded {
			tasks, err = s.sync.DueBetween(ctx, now.AddDate(0, 0, -lookBack-2).Format(time.DateOnly), now.AddDate(0, 0, lookAhead+2).Format(time.DateOnly))
			if err != nil {
				return err
			}
			loaded = true
		}
		loc, err := s.placeOf(ctx, r.MadeBy.String)
		if err != nil {
			continue
		}
		today := now.In(loc)
		day := today.Format(time.DateOnly)
		oldest := today.AddDate(0, 0, -lookBack).Format(time.DateOnly)
		on := syncer.AutomationDay(r.EnabledAt, loc)
		if on == "" {
			on = day
		}
		if on > oldest {
			oldest = on
		}
		for _, t := range tasks {
			if t.WorkspaceID != r.WorkspaceID || t.Completed || t.Deleted || t.Archived || t.DueDate == "" {
				continue
			}
			if r.ProjectID.Valid && r.ProjectID.String != t.ProjectID {
				continue
			}
			fire, ok := fireDay(rule.When.Value, t.DueDate)
			if !ok || fire > day || fire < oldest {
				continue
			}
			stopped, err := s.dueRun(ctx, r.ID, t.ID, t.DueDate, r.MadeBy.String)
			if err != nil {
				if ctx.Err() != nil {
					return err
				}
				s.log.Warn("automation: a due date's run failed", "rule", r.ID, "task", t.ID, "err", err)
				break
			}
			if stopped {
				break
			}
		}
	}
	return nil
}

// dueRun runs a rule on a date it hasn't done yet, and notes it once that's done, so a
// run that stopped short or never started is still to do.
func (s *Service) dueRun(ctx context.Context, rule, task, due, maker string) (stopped bool, err error) {
	n, err := s.st.AutomationWasFired(ctx, sqlitedb.AutomationWasFiredParams{AutomationID: rule, TaskID: task, DueDate: due})
	if err != nil || n > 0 {
		return false, err
	}
	stopped, err = s.run(ctx, runPayload{Rule: rule, Task: task, Actor: maker, Run: recur.HashID("due\n" + rule + "\n" + task + "\n" + due)})
	if err != nil || stopped {
		return stopped, err
	}
	_, err = s.st.MarkAutomationFired(ctx, sqlitedb.MarkAutomationFiredParams{AutomationID: rule, TaskID: task, DueDate: due, Now: store.Now()})
	return false, err
}

// Tidy forgets what's past keeping.
func (s *Service) Tidy(ctx context.Context, now time.Time) error {
	if err := s.st.PurgeAutomationRuns(ctx, now.Add(-keepRuns)); err != nil {
		return err
	}
	return s.st.PurgeAutomationFired(ctx, now.Add(-keepRuns*12))
}

// Run is one line of what a rule did.
type Run = sqlitedb.ListAutomationRunsRow

// Runs lists the newest runs in a workspace, or of one rule.
func (s *Service) Runs(ctx context.Context, workspaceID, automationID string, limit int64) ([]Run, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if automationID == "" {
		return s.st.ListAutomationRuns(ctx, sqlitedb.ListAutomationRunsParams{WorkspaceID: workspaceID, MaxRows: limit})
	}
	rows, err := s.st.ListAutomationRunsOf(ctx, sqlitedb.ListAutomationRunsOfParams{WorkspaceID: workspaceID, AutomationID: automationID, MaxRows: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Run, len(rows))
	for i, r := range rows {
		out[i] = Run(r)
	}
	return out, nil
}
