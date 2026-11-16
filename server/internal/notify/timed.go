package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// LateBy is how long after its moment a reminder or due notice still goes
// out, for a server that was down at the time. Older ones are let go: a
// reminder a day late is no reminder.
const LateBy = 12 * time.Hour

// allDayAt is the local time an all-day task is due, for its due notice
// and for reminders relative to it.
const allDayAt = "09:00"

// DueMoment is when a task with a due date falls due in a timezone: its
// due time, or 09:00 for an all-day task.
func DueMoment(date, clock string, loc *time.Location) (time.Time, bool) {
	if clock == "" {
		clock = allDayAt
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, loc)
	return t.UTC(), err == nil
}

// ReminderMoment is when a reminder offset minutes before a task's due
// moment fires in a timezone. Whole days of the offset count as calendar
// days, so "the day before, 9:00" stays at 9:00 across a change to or
// from daylight saving time.
func ReminderMoment(date, clock string, offset int64, loc *time.Location) (time.Time, bool) {
	if clock == "" {
		clock = allDayAt
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, loc)
	if err != nil {
		return time.Time{}, false
	}
	days, rest := offset/(24*60), offset%(24*60)
	return t.AddDate(0, 0, -int(days)).Add(-time.Duration(rest) * time.Minute).UTC(), true
}

// SendTimed fires the reminders and due notices whose moment has come. It
// runs from the job runner every poll; each fires once for its moment,
// which the same transaction that makes the notification records. Tasks in
// archived projects stay quiet; nothing is recorded, so a project brought
// back within the late window still has its say.
func (s *Service) SendTimed(ctx context.Context) error {
	now := s.now()
	// Where each person is, and how they read dates.
	type reader struct {
		loc   *time.Location
		style accounts.DateStyle
	}
	readers := map[string]reader{}
	zone := func(userID string) (*time.Location, error) {
		if r, ok := readers[userID]; ok {
			return r.loc, nil
		}
		u, err := s.st.GetUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		readers[userID] = reader{accounts.Location(u), accounts.DateStyleOf(u)}
		return readers[userID].loc, nil
	}
	styleOf := func(userID string) accounts.DateStyle { return readers[userID].style }
	// Every timezone's today, with the late window, is within a day of the
	// UTC date either side.
	earliest := now.Add(-LateBy).AddDate(0, 0, -1).Format(time.DateOnly)
	inWindow := func(at time.Time) bool { return !at.After(now) && at.After(now.Add(-LateBy)) }
	sent := false

	reminders, err := s.tasks.Reminders(ctx, now.Add(-LateBy), now, earliest)
	if err != nil {
		return err
	}
	for _, r := range reminders {
		task, ok, err := s.tasks.Task(ctx, r.TaskID)
		if err != nil {
			return err
		}
		if !ok || task.Deleted || task.Completed || task.Archived {
			continue
		}
		loc, err := zone(r.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		// key is the moment it is recorded as sent for. For a relative
		// reminder that is its moment as if in UTC, which a change to its
		// owner's timezone (travel, a second device) leaves alone, while a
		// new due date, time or offset arms it again.
		at, setAt := r.RemindAt, r.SetAt
		key := at
		if r.Relative && task.DueDate != "" {
			var ok bool
			if at, ok = ReminderMoment(task.DueDate, task.DueTime, r.Offset, loc); !ok {
				continue
			}
			key, _ = ReminderMoment(task.DueDate, task.DueTime, r.Offset, time.UTC)
			if task.DueSetAt.After(setAt) {
				setAt = task.DueSetAt
			}
		}
		// As with due notices, a moment already past when it was set (a
		// task moved later than its reminder, the next task of an overdue
		// series) is not news.
		if !inWindow(at) || !setAt.Before(at) {
			continue
		}
		if n, err := s.st.ReminderDelivered(ctx, sqlitedb.ReminderDeliveredParams{ReminderID: r.ID, RemindAt: key}); err != nil || n > 0 {
			if err != nil {
				return err
			}
			continue
		}
		n := Notice{UserID: r.UserID, WorkspaceID: r.WorkspaceID, Kind: Reminder, Task: &task,
			Title: "Reminder: " + task.Title, Body: about(task, now.In(loc), styleOf(r.UserID))}
		did, err := s.sendOnce(ctx, n, func(q store.Queries) (int64, error) {
			return q.ClaimReminderDelivery(ctx, sqlitedb.ClaimReminderDeliveryParams{ReminderID: r.ID, RemindAt: key, Now: now})
		})
		if err != nil {
			return err
		}
		sent = sent || did
	}

	tasks, err := s.tasks.DueBetween(ctx, earliest, now.AddDate(0, 0, 1).Format(time.DateOnly))
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Archived {
			continue
		}
		// Whose it is: the assignees, or with none its creator.
		people := t.Assignees
		if len(people) == 0 && t.CreatedBy != "" {
			people = []string{t.CreatedBy}
		}
		due := t.DueDate
		if t.DueTime != "" {
			due += " " + t.DueTime
		}
		for _, userID := range people {
			loc, err := zone(userID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			at, ok := DueMoment(t.DueDate, t.DueTime, loc)
			// A due date set after its moment had passed (a task made today
			// for today) is already known; it is not news.
			if !ok || !inWindow(at) || !t.DueSetAt.Before(at) {
				continue
			}
			if n, err := s.st.DueNoticeSent(ctx, sqlitedb.DueNoticeSentParams{TaskID: t.ID, UserID: userID, Due: due}); err != nil || n > 0 {
				if err != nil {
					return err
				}
				continue
			}
			title := t.Title + " is due"
			if t.DueTime == "" {
				title += " today"
			}
			task := t
			n := Notice{UserID: userID, WorkspaceID: t.WorkspaceID, Kind: Due, Task: &task,
				Title: title, Body: about(t, now.In(loc), styleOf(userID))}
			did, err := s.sendOnce(ctx, n, func(q store.Queries) (int64, error) {
				return q.ClaimDueNotice(ctx, sqlitedb.ClaimDueNoticeParams{TaskID: t.ID, UserID: userID, Due: due, Now: now})
			})
			if err != nil {
				return err
			}
			sent = sent || did
		}
	}
	if sent {
		s.jobs.Wake()
	}
	return nil
}

// sendOnce creates a notification unless claim, run in the same
// transaction, reports that it went out before. The claim is recorded
// even for someone who gets no notification (they turned that kind off,
// or cannot see the task), so they are not considered again. It reports
// whether a notification was made.
func (s *Service) sendOnce(ctx context.Context, n Notice, claim func(q store.Queries) (int64, error)) (bool, error) {
	p, err := s.plan(ctx, n)
	if err != nil {
		return false, err
	}
	made := false
	err = s.st.InTx(ctx, func(q store.Queries) error {
		claimed, err := claim(q)
		if err != nil || claimed == 0 || p == nil {
			return err
		}
		made = true
		return s.insert(ctx, q, p)
	})
	if err != nil {
		return false, err
	}
	if made && p.channels.App {
		s.poke(p.UserID)
	}
	return made, nil
}

// about is a task's due date and place, for a reminder or due notice:
// "Due today at 2:30 PM. In Home".
func about(t syncer.TaskBrief, local time.Time, style accounts.DateStyle) string {
	if t.DueDate == "" {
		return where(t)
	}
	return dueLine(t.DueDate, t.DueTime, local, style) + ". " + where(t)
}

// dueLine says when something is due, from the day it is where the reader
// is and the way they read dates: "Due today at 2:30 PM", "Due tomorrow",
// "Due Mon, Nov 2".
func dueLine(date, clock string, local time.Time, style accounts.DateStyle) string {
	today := local.Format(time.DateOnly)
	day := date
	if d, err := time.Parse(time.DateOnly, date); err == nil {
		day = style.Day(d, true, local)
	}
	switch date {
	case today:
		day = "today"
	case local.AddDate(0, 0, 1).Format(time.DateOnly):
		day = "tomorrow"
	case local.AddDate(0, 0, -1).Format(time.DateOnly):
		day = "yesterday"
	}
	line := "Due " + day
	if clock != "" {
		line += " at " + style.Time(clock)
	}
	return line
}

// handleCopyReminders gives the next task of a repeating series the
// reminders the one before had.
func (s *Service) handleCopyReminders(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	_, err := s.tasks.CopyReminders(ctx, r.ID)
	return err
}
