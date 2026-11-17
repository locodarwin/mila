package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/reports"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// The weekly report's lists stop at these, with a line saying how many
// more there are.
const (
	weeklyProjects = 8
	weeklyNeeds    = 10
)

// SendWeekly queues the weekly report
// for everyone who has it on, on the first day of their week from their
// digest hour: one for each workspace they are in, about the week before.
// Each is claimed in the transaction that queues it, so it goes once. It
// runs every minute, beside the reminder scan, so it only queues: reading
// a workspace for the report is the job's (handleWeekly), which a failure
// tries again after a growing wait.
func (s *Service) SendWeekly(ctx context.Context) error {
	users, err := s.st.ListUsers(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	queued := false
	for _, u := range users {
		prefs := PrefsOf(u)
		if !prefs.Weekly.Enabled || u.Email == "" {
			continue
		}
		local := now.In(accounts.Location(u))
		if local.Weekday() != accounts.WeekStartOf(u) {
			continue
		}
		if h := local.Hour(); h < prefs.Digest.Hour || h >= prefs.Digest.Hour+digestWindow {
			continue
		}
		today := local.Format(time.DateOnly)
		week := local.AddDate(0, 0, -7).Format(time.DateOnly)
		workspaces, err := s.st.ListWorkspacesForUser(ctx, u.ID)
		if err != nil {
			return err
		}
		for _, ws := range workspaces {
			last, err := s.st.GetWeeklyReport(ctx, sqlitedb.GetWeeklyReportParams{UserID: u.ID, WorkspaceID: ws.ID})
			switch {
			case err == nil && last.Week >= week:
				continue
			case err != nil && !errors.Is(err, sql.ErrNoRows):
				return err
			}
			err = s.st.InTx(ctx, func(q store.Queries) error {
				n, err := q.ClaimWeeklyReport(ctx, sqlitedb.ClaimWeeklyReportParams{UserID: u.ID, WorkspaceID: ws.ID, Week: week, Now: now})
				if err != nil || n == 0 {
					return err
				}
				queued = true
				return jobs.Enqueue(ctx, q, jobWeekly, weeklyJob{UserID: u.ID, WorkspaceID: ws.ID, Today: today}, now, now)
			})
			if err != nil {
				return err
			}
		}
	}
	if queued {
		s.jobs.Wake()
	}
	return nil
}

// weeklyJob is one person's weekly report for one workspace, claimed for
// the week before Today.
type weeklyJob struct {
	UserID      string `json:"userId"`
	WorkspaceID string `json:"workspaceId"`
	Today       string `json:"today"`
}

// handleWeekly writes one weekly report and queues it to be sent, unless
// the week was quiet, or the person has turned it off or left the
// workspace since it was queued.
func (s *Service) handleWeekly(ctx context.Context, payload []byte) error {
	var j weeklyJob
	if err := json.Unmarshal(payload, &j); err != nil {
		return err
	}
	u, err := s.st.GetUser(ctx, j.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !PrefsOf(u).Weekly.Enabled || u.Email == "" {
		return nil
	}
	workspaces, err := s.st.ListWorkspacesForUser(ctx, u.ID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(workspaces, func(ws sqlitedb.ListWorkspacesForUserRow) bool { return ws.ID == j.WorkspaceID })
	if i < 0 {
		return nil
	}
	now := s.now()
	m, err := s.weekly(ctx, u, workspaces[i], j.Today, now.In(accounts.Location(u)))
	if err != nil || m == nil {
		return err
	}
	if err := jobs.Enqueue(ctx, s.st, jobMail, m, now, now); err != nil {
		return err
	}
	s.jobs.Wake()
	return nil
}

// weekly writes a person's weekly report for a workspace, or nil for a
// quiet week. It counts only what they can see; the time everyone logged
// is for members, since a guest sees only some of it.
func (s *Service) weekly(ctx context.Context, u store.User, ws sqlitedb.ListWorkspacesForUserRow, today string, local time.Time) (*mail.Message, error) {
	in, err := s.tasks.WeeklyInput(ctx, u.ID, ws.ID)
	if err != nil {
		return nil, err
	}
	in.Today, in.Zone = today, local.Location()
	r := reports.Weekly(in)
	if r.Quiet {
		return nil, nil
	}
	style := accounts.DateStyleOf(u)
	span := weekSpan(style, r.Week, local)
	q := url.Values{"ws": {ws.ID}}
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\nHere is last week in %s, %s, against the week before.\n\n", u.Name, ws.Name, span)
	fmt.Fprintf(&b, "- %s, %s\n", count(r.Last.Done, "task done", "tasks done"), against(r.Last.Done, r.Prior.Done))
	fmt.Fprintf(&b, "- %s, %s\n", count(r.Last.Made, "task made", "tasks made"), against(r.Last.Made, r.Prior.Made))
	fmt.Fprintf(&b, "- %s as the week ended, %s\n", count(r.Last.Overdue, "task overdue", "tasks overdue"), against(r.Last.Overdue, r.Prior.Overdue))
	if r.Last.Median != nil {
		line := "- A median cycle time of " + sayDays(*r.Last.Median)
		switch {
		case r.Prior.Median == nil:
		case sayDays(*r.Prior.Median) == sayDays(*r.Last.Median):
			line += ", the same as the week before"
		case *r.Last.Median < *r.Prior.Median:
			line += ", down from " + sayDays(*r.Prior.Median)
		default:
			line += ", up from " + sayDays(*r.Prior.Median)
		}
		b.WriteString(line + "\n")
	}

	if len(r.Projects) > 0 {
		b.WriteString("\nProjects\n\n")
		for i, p := range r.Projects {
			if i == weeklyProjects {
				fmt.Fprintf(&b, "- and %s\n", count(len(r.Projects)-i, "more project", "more projects"))
				break
			}
			line := fmt.Sprintf("- %s: %d done, %d open", p.Name, p.Done, p.Open)
			if p.Overdue > 0 {
				line += fmt.Sprintf(", %d overdue", p.Overdue)
			}
			fmt.Fprintf(&b, "%s\n  %s/projects/%s?%s\n", line, s.baseURL, url.PathEscape(p.ID), q.Encode())
		}
	}

	if r.Everyone > 0 {
		b.WriteString("\nTime logged\n\n")
		fmt.Fprintf(&b, "- You: %s\n", Duration(r.Mine))
		if ws.Role != "guest" {
			fmt.Fprintf(&b, "- Everyone: %s\n", Duration(r.Everyone))
		}
	}

	if len(r.Needs) > 0 {
		b.WriteString("\nNeeds you\n\n")
		for i, n := range r.Needs {
			if i == weeklyNeeds {
				fmt.Fprintf(&b, "- and %d more\n", len(r.Needs)-i)
				break
			}
			var why []string
			if n.Late != nil {
				why = append(why, count(*n.Late, "day", "days")+" overdue")
			}
			if n.Waiting > 0 {
				why = append(why, "waiting on "+count(n.Waiting, "task", "tasks"))
			}
			if n.Over != nil {
				why = append(why, Duration(*n.Over)+" past its estimate")
			}
			fmt.Fprintf(&b, "- %s: %s\n  %s\n", n.Task.Title, strings.Join(why, ", "), s.Link(ws.ID, n.Task.ID))
		}
	}

	fmt.Fprintf(&b, "\nSee more in Reports: %s/reports?%s\n", s.baseURL, q.Encode())
	fmt.Fprintf(&b, "\nIt counts what you can see in %s.\n", ws.Name)
	b.WriteString(s.footer())
	return &mail.Message{To: u.Email, Subject: ws.Name + ": your week, " + span, Text: b.String()}, nil
}

// count writes a number with its noun: "1 task done", "3 tasks done".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// against says how a week's figure compares with the week before's.
func against(now, before int) string {
	switch {
	case now > before:
		return fmt.Sprintf("up from %d", before)
	case now < before:
		return fmt.Sprintf("down from %d", before)
	}
	return "the same as the week before"
}

// sayDays writes a length of time in days as the Reports page does
// (reports.ts sayDays): hours under a day, tenths under ten days.
func sayDays(days float64) string {
	if days < 1 {
		hours := int(math.Round(days * 24))
		switch {
		case hours < 1:
			return "under an hour"
		case hours == 1:
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	n := math.Round(days)
	if days < 10 {
		n = math.Round(days*10) / 10
	}
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%g days", n)
}

// weekSpan writes a week the person's way: "Nov 2 to 8", "Oct 26 to
// Nov 1", "2 to 8 Nov" or "2026-11-02 to 2026-11-08".
func weekSpan(style accounts.DateStyle, p reports.Period, now time.Time) string {
	from, errFrom := time.Parse(time.DateOnly, p.From)
	to, errTo := time.Parse(time.DateOnly, p.To)
	if errFrom != nil || errTo != nil || style.Order == "iso" {
		return p.From + " to " + p.To
	}
	if from.Month() != to.Month() || from.Year() != now.Year() {
		return style.Day(from, false, now) + " to " + style.Day(to, false, now)
	}
	if style.Order == "day-month" {
		return fmt.Sprintf("%d to %s", from.Day(), style.Day(to, false, now))
	}
	return fmt.Sprintf("%s to %d", style.Day(from, false, now), to.Day())
}
