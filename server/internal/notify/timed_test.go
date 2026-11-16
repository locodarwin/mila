package notify_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/recur"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// tick sets the clock and runs the scan and what it queued.
func (w *world) tick(at time.Time) {
	w.t.Helper()
	w.clock = at
	must(w.t, w.svc.SendTimed(w.ctx))
	w.run()
}

func (w *world) reminder(role, task string, at time.Time, offset any) string {
	id := uuid.NewString()
	w.push(role, "reminder", id, map[string]any{"task_id": task, "user_id": w.users[role],
		"remind_at": at.UTC().Format(time.RFC3339Nano), "offset_minutes": offset, "deleted_at": nil})
	return id
}

func (w *world) zone(role, name string) *time.Location {
	w.t.Helper()
	_, err := accounts.UpdateSettings(w.ctx, w.st, w.users[role], func(s accounts.Settings) error {
		s["timezone"] = json.RawMessage(`"` + name + `"`)
		return nil
	})
	must(w.t, err)
	loc, err := time.LoadLocation(name)
	must(w.t, err)
	return loc
}

// A week from now, so every clock in a test is after the edits' own
// clocks, whenever the test runs.
func aWeekOut() string { return time.Now().UTC().AddDate(0, 0, 7).Format(time.DateOnly) }

func at(t *testing.T, day, clock string, loc *time.Location) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, loc)
	must(t, err)
	return v.UTC()
}

func TestReminders(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		chicago := w.zone("member", "America/Chicago")
		day := aWeekOut()
		// Due notices are another test's business.
		_, _, err := w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{notify.Due: {}}}, nil)
		must(t, err)
		task := w.task("owner", w.project, "Repaint the porch", map[string]any{"due_date": day, "due_time": "18:00"})
		w.assign("owner", task, "member", false)
		w.run()
		w.mail.take()

		// A reminder at a fixed time fires at that time, once.
		fixed := w.reminder("member", task, at(t, day, "08:30", chicago), nil)
		w.tick(at(t, day, "08:29", chicago))
		want(t, w.inbox("member"), "assigned: Shawn assigned you to Repaint the porch")
		w.tick(at(t, day, "08:30", chicago))
		want(t, w.inbox("member"), "reminder: Reminder: Repaint the porch", "assigned: Shawn assigned you to Repaint the porch")
		page, err := w.svc.List(w.ctx, w.users["member"], "", 1)
		must(t, err)
		if body := page.Items[0].Body; body != "Due today at 18:00. In Home" {
			t.Fatalf("body = %q", body)
		}
		sent := w.mail.take()
		if len(sent) != 1 || sent[0].Subject != "Reminder: Repaint the porch" ||
			!strings.Contains(sent[0].Text, "Due today at 18:00. In Home\n") || !strings.Contains(sent[0].Text, "/?task="+task+"&ws="+w.ws) {
			t.Fatalf("emailed %v", sent)
		}
		w.tick(at(t, day, "08:31", chicago))
		if got := w.inbox("member"); len(got) != 2 {
			t.Fatalf("fired twice: %q", got)
		}
		// Moved to a later time, it fires again then.
		w.push("member", "reminder", fixed, map[string]any{"remind_at": at(t, day, "10:00", chicago).Format(time.RFC3339)})
		w.tick(at(t, day, "10:00", chicago))
		if got := w.inbox("member"); len(got) != 3 {
			t.Fatalf("a moved reminder did not fire again: %q", got)
		}

		// A relative one follows the due time in its owner's timezone, even
		// when someone else moves the task, and whatever remind_at says.
		w.reminder("member", task, at(t, day, "01:00", time.UTC), 60)
		w.tick(at(t, day, "16:59", chicago))
		if got := w.inbox("member"); len(got) != 3 {
			t.Fatalf("fired early: %q", got)
		}
		w.push("owner", "task", task, map[string]any{"due_time": "19:00"})
		w.tick(at(t, day, "17:00", chicago))
		if got := w.inbox("member"); len(got) != 3 {
			t.Fatalf("fired for the old due time: %q", got)
		}
		w.tick(at(t, day, "18:00", chicago))
		if got := w.inbox("member"); len(got) != 4 {
			t.Fatalf("did not fire an hour before the new due time: %q", got)
		}
		w.mail.take()
		// Fired, it stays fired when its owner's timezone changes (a trip,
		// a laptop set to another zone), going west or east.
		denver := w.zone("member", "America/Denver")
		w.tick(at(t, day, "18:00", denver))
		w.zone("member", "America/New_York")
		w.tick(at(t, day, "19:05", chicago))
		w.zone("member", "America/Chicago")
		if got := w.inbox("member"); len(got) != 4 || len(w.mail.take()) != 0 {
			t.Fatalf("fired again after a change of timezone: %q", got)
		}

		// Nothing for a completed task, a deleted reminder, someone who lost
		// the task, or a time long past.
		done := w.task("member", w.project, "Done already", map[string]any{"due_date": day,
			"completed_at": time.Now().UTC().Format(time.RFC3339)})
		w.reminder("member", done, at(t, day, "20:00", chicago), nil)
		removed := w.reminder("member", task, at(t, day, "20:00", chicago), nil)
		w.push("member", "reminder", removed, map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339)})
		inbox := w.task("guest", w.project, "Guest's task", map[string]any{"due_date": day})
		w.reminder("guest", inbox, at(t, day, "20:00", time.UTC), nil)
		must(t, w.st.DeleteMembership(w.ctx, sqlitedb.DeleteMembershipParams{WorkspaceID: w.ws, UserID: w.users["guest"]}))
		w.reminder("member", task, at(t, day, "06:00", chicago), nil)
		w.tick(at(t, day, "20:00", chicago))
		if got := w.inbox("member"); len(got) != 4 {
			t.Fatalf("fired what it should not: %q", got)
		}
		if n := w.stored("guest"); n != 0 {
			t.Fatal("a former member was reminded")
		}
		for _, m := range w.mail.take() {
			if m.To == w.email("guest") {
				t.Fatalf("a former member was emailed %q", m.Subject)
			}
		}
	})
}

func TestReminderMoment(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	must(t, err)
	for _, c := range []struct {
		date, clock string
		offset      int64
		want        string
	}{
		// The day before, 9:00 is at 9:00 on either side of a change to or
		// from daylight saving time.
		{"2026-11-01", "", 1440, "2026-10-31 09:00 EDT"},
		{"2026-03-08", "", 1440, "2026-03-07 09:00 EST"},
		{"2026-11-01", "18:00", 1440 + 30, "2026-10-31 17:30 EDT"},
		{"2026-11-01", "18:00", 60, "2026-11-01 17:00 EST"},
		{"2026-10-30", "14:30", 330, "2026-10-30 09:00 EDT"},
	} {
		got, ok := notify.ReminderMoment(c.date, c.clock, c.offset, ny)
		if s := got.In(ny).Format("2006-01-02 15:04 MST"); !ok || s != c.want {
			t.Errorf("%s %s less %d minutes = %s, want %s", c.date, c.clock, c.offset, s, c.want)
		}
	}
}

func TestRemindersSetLate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		_, _, err := w.svc.SavePrefs(w.ctx, w.users["owner"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{notify.Due: {}}}, nil)
		must(t, err)
		now := time.Now().UTC().Truncate(time.Minute)
		task := w.task("owner", w.project, "Call Dana", nil)
		// Set after their moments had passed: a fixed time, and one relative
		// to a due moment 30 minutes off (the next task of an overdue
		// series, a task moved later than its reminder).
		w.reminder("owner", task, now.Add(-time.Hour), nil)
		due := now.Add(30 * time.Minute)
		soon := w.task("owner", w.project, "Pay rent", map[string]any{"due_date": due.Format(time.DateOnly), "due_time": due.Format("15:04")})
		w.reminder("owner", soon, now.Add(-30*time.Minute), 60)
		// Set before its moment, which still fires.
		w.reminder("owner", task, now.Add(2*time.Minute), nil)
		w.tick(now.Add(3 * time.Minute))
		want(t, w.inbox("owner"), "reminder: Reminder: Call Dana")
	})
}

func TestDueNotices(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		chicago := w.zone("member", "America/Chicago")
		day := aWeekOut()
		// Assigned: the assignees hear, not the creator.
		assigned := w.task("owner", w.project, "Pay rent", map[string]any{"due_date": day})
		w.assign("owner", assigned, "member", false)
		// Unassigned: the creator hears, in their timezone (UTC here).
		timed := w.task("owner", w.project, "Call Dana", map[string]any{"due_date": day, "due_time": "15:00"})
		w.task("owner", w.project, "Done", map[string]any{"due_date": day, "completed_at": time.Now().UTC().Format(time.RFC3339)})
		w.run()
		w.mail.take()

		// An all-day task is due at 09:00 where its person is.
		w.tick(at(t, day, "08:59", chicago))
		want(t, w.inbox("member"), "assigned: Shawn assigned you to Pay rent")
		w.tick(at(t, day, "09:00", chicago))
		want(t, w.inbox("member"), "due: Pay rent is due today", "assigned: Shawn assigned you to Pay rent")
		want(t, w.inbox("owner"), "due: Call Dana is due")
		page, err := w.svc.List(w.ctx, w.users["owner"], "", 1)
		must(t, err)
		if body := page.Items[0].Body; body != "Due today at 15:00. In Home" {
			t.Fatalf("body = %q", body)
		}
		// Due notices are emailed by default.
		if sent := w.mail.take(); len(sent) != 2 {
			t.Fatalf("emailed %d", len(sent))
		}
		// Once per due value; a new due time announces it again.
		w.tick(at(t, day, "10:00", chicago))
		if got := w.inbox("owner"); len(got) != 1 {
			t.Fatalf("announced twice: %q", got)
		}
		w.push("owner", "task", timed, map[string]any{"due_time": "17:30"})
		w.tick(at(t, day, "17:30", time.UTC))
		want(t, w.inbox("owner"), "due: Call Dana is due", "due: Call Dana is due")

		// A due date set after its moment is not news, nor is one long gone.
		yesterday := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
		w.task("owner", w.project, "Late already", map[string]any{"due_date": yesterday})
		w.tick(at(t, yesterday, "10:00", time.UTC))
		w.tick(at(t, day, "23:00", time.UTC).AddDate(0, 0, 1))
		if got := w.inbox("owner"); len(got) != 2 {
			t.Fatalf("announced what it should not: %q", got)
		}

		// Someone who turned due notices off gets none.
		_, _, err = w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{notify.Due: {}}}, nil)
		must(t, err)
		w.push("owner", "task", assigned, map[string]any{"due_time": "23:00"})
		w.tick(at(t, day, "23:00", chicago))
		if got := w.inbox("member"); len(got) != 2 {
			t.Fatalf("notified with due notices off: %q", got)
		}
	})
}

func TestDueMoment(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	must(t, err)
	for _, c := range []struct{ date, clock, want string }{
		{"2026-10-30", "", "2026-10-30T14:00:00Z"},      // CDT, all day at 09:00
		{"2026-11-02", "", "2026-11-02T15:00:00Z"},      // CST
		{"2026-11-01", "", "2026-11-01T15:00:00Z"},      // the day the clocks go back
		{"2026-11-02", "18:00", "2026-11-03T00:00:00Z"}, // the evening, the next day in UTC
	} {
		got, ok := notify.DueMoment(c.date, c.clock, chicago)
		if !ok || got.Format(time.RFC3339) != c.want {
			t.Errorf("DueMoment(%s %s) = %s, want %s", c.date, c.clock, got.Format(time.RFC3339), c.want)
		}
	}
	if _, ok := notify.DueMoment("someday", "", time.UTC); ok {
		t.Error("a broken date has a moment")
	}
}

func TestRepeatCopiesReminders(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		day := aWeekOut()
		first := w.task("owner", w.project, "Water the plants", map[string]any{"due_date": day, "recurrence": "FREQ=DAILY"})
		rel := w.reminder("member", first, at(t, day, "08:00", time.UTC), 60)
		w.push("owner", "task", first, map[string]any{"series_id": first, "completed_at": time.Now().UTC().Format(time.RFC3339)})
		nextDay, _ := time.Parse(time.DateOnly, day)
		next := recur.NextTaskID(first)
		w.push("owner", "task", next, map[string]any{"title": "Water the plants", "sort_key": "a1", "project_id": w.project,
			"created_by": w.users["owner"], "due_date": nextDay.AddDate(0, 0, 1).Format(time.DateOnly), "recurrence": "FREQ=DAILY", "series_id": first})
		w.run()
		snap, err := w.sync.Bootstrap(w.ctx, w.users["member"], w.ws)
		must(t, err)
		found := false
		for _, r := range snap.Rows {
			if r.Entity == "reminder" && r.ID == recur.HashID(next+"/"+rel) {
				found = r.Fields["task_id"] == next
			}
		}
		if !found {
			t.Fatal("the member's reminder did not follow the series")
		}
	})
}

func TestNoticesFollowDatePreferences(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		chicago := w.zone("member", "America/Chicago")
		// The member's device writes 12-hour times, and they chose day-month dates.
		_, err := accounts.UpdateSettings(w.ctx, w.st, w.users["member"], func(s accounts.Settings) error {
			s["locale"] = json.RawMessage(`{"clock":"12","dateOrder":"month-day"}`)
			s["preferences"] = json.RawMessage(`{"dateFormat":"day-month"}`)
			return nil
		})
		must(t, err)
		_, _, err = w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Kinds: map[notify.Kind]notify.Channels{notify.Due: {}}}, nil)
		must(t, err)
		day := aWeekOut()
		task := w.task("member", w.project, "Repaint the porch", map[string]any{"due_date": day, "due_time": "18:00"})
		w.reminder("member", task, at(t, day, "08:30", chicago), nil)
		w.reminder("member", task, at(t, day, "08:30", chicago).AddDate(0, 0, -2), nil)
		w.run()

		w.tick(at(t, day, "08:30", chicago).AddDate(0, 0, -2))
		due, _ := time.Parse(time.DateOnly, day)
		page, err := w.svc.List(w.ctx, w.users["member"], "", 1)
		must(t, err)
		if body, want := page.Items[0].Body, "Due "+due.Format("Mon, 2 Jan")+" at 6:00 PM. In Home"; body != want {
			t.Fatalf("body = %q, want %q", body, want)
		}
		w.tick(at(t, day, "08:30", chicago))
		page, err = w.svc.List(w.ctx, w.users["member"], "", 1)
		must(t, err)
		if body := page.Items[0].Body; body != "Due today at 6:00 PM. In Home" {
			t.Fatalf("body = %q", body)
		}
	})
}

// An archived project's tasks are still there, but no date brings them up:
// no reminder, no due notice, no digest line, no word that one can start.
// Bringing the project back within the late window lets them speak again.
func TestArchivedProjectsStayQuiet(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		day := aWeekOut()
		_, _, err := w.svc.SavePrefs(w.ctx, w.users["member"], notify.Prefs{Digest: notify.Digest{Enabled: true, Hour: 7}}, nil)
		must(t, err)
		archived := uuid.NewString()
		w.push("owner", "project", archived, map[string]any{"name": "Old", "icon": "", "color": "blue", "description": "", "sort_key": "a1", "archived_at": nil, "deleted_at": nil})

		quiet := w.task("owner", archived, "Quiet task", map[string]any{"due_date": day, "estimate_minutes": 10})
		w.assign("owner", quiet, "member", false)
		w.reminder("member", quiet, at(t, day, "08:00", time.UTC), nil)
		w.push("member", "time_entry", uuid.NewString(), map[string]any{"task_id": quiet, "user_id": w.users["member"], "day": day, "minutes": 30, "note": "", "running": false, "deleted_at": nil})
		waiting := w.task("owner", archived, "Waiting task", nil)
		w.assign("owner", waiting, "member", false)
		blocker := w.task("owner", w.project, "Blocker", nil)
		w.waitFor("owner", waiting, blocker)

		loud := w.task("owner", w.project, "Loud task", map[string]any{"due_date": day})
		w.assign("owner", loud, "member", false)
		w.push("owner", "project", archived, map[string]any{"archived_at": time.Now().UTC().Format(time.RFC3339Nano)})
		w.run()
		w.mail.take()

		w.finish("owner", blocker)
		w.run()
		w.tick(at(t, day, "09:00", time.UTC))
		for _, line := range w.inbox("member") {
			if strings.HasPrefix(line, "reminder:") || strings.HasPrefix(line, "unblocked:") || line == "due: Quiet task is due today" {
				t.Errorf("heard about an archived project's task: %q", line)
			}
		}
		want := "due: Loud task is due today"
		if got := w.inbox("member"); got[0] != want {
			t.Fatalf("newest = %q, want %q", got[0], want)
		}
		w.mail.take()
		must(t, w.svc.SendDigests(w.ctx))
		w.run()
		sent := w.mail.take()
		if len(sent) != 1 {
			t.Fatalf("sent %d digests", len(sent))
		}
		if !strings.Contains(sent[0].Text, "Loud task") {
			t.Fatalf("the digest lacks the live task:\n%s", sent[0].Text)
		}
		for _, line := range strings.Split(sent[0].Text, "\n") {
			// Time logged is a record, and nothing was logged yesterday here;
			// the unread list may name a task, but never as due or over.
			if strings.Contains(line, "Quiet task") && !strings.Contains(line, "assigned you") {
				t.Errorf("the digest counts an archived project's task: %q", line)
			}
		}

		w.push("owner", "project", archived, map[string]any{"archived_at": nil})
		w.tick(at(t, day, "09:01", time.UTC))
		if got := w.inbox("member"); got[0] != "reminder: Reminder: Quiet task" && got[0] != "due: Quiet task is due today" {
			t.Fatalf("brought back, newest = %q", got[0])
		}
	})
}
