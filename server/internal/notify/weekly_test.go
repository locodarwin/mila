package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/reports"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestWeeklyReport(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		member, guest := w.users["member"], w.users["guest"]
		// Dana is in Chicago, her week starting as her device's (Monday,
		// none reported); Lee is in UTC, and his device starts it on Sunday.
		_, err := accounts.UpdateSettings(w.ctx, w.st, member, func(s accounts.Settings) error {
			s["timezone"] = json.RawMessage(`"America/Chicago"`)
			return nil
		})
		must(t, err)
		_, err = accounts.UpdateSettings(w.ctx, w.st, guest, func(s accounts.Settings) error {
			s["locale"] = json.RawMessage(`{"clock":"24","dateOrder":"day-month","weekStart":0}`)
			return nil
		})
		must(t, err)
		for _, who := range []string{member, guest} {
			_, _, err = w.svc.SavePrefs(w.ctx, who, notify.Prefs{Digest: notify.Digest{Hour: 7}}, &notify.Weekly{Enabled: true})
			must(t, err)
		}
		// A save from an app older than the weekly report leaves it on.
		p, _, err := w.svc.SavePrefs(w.ctx, member, notify.Prefs{Digest: notify.Digest{Hour: 7}}, nil)
		must(t, err)
		if !p.Weekly.Enabled {
			t.Fatal("a save without weekly turned it off")
		}

		day := func(d, h int) time.Time { return time.Date(2026, 11, d, h, 0, 0, 0, time.UTC) }
		task := func(role string, at time.Time, project, title string, extra map[string]any) string {
			id := uuid.NewString()
			fields := map[string]any{"title": title, "sort_key": "a0", "created_by": w.users[role], "project_id": project}
			for k, v := range extra {
				fields[k] = v
			}
			w.pushAt(role, at, "task", id, fields)
			return id
		}
		private := uuid.NewString()
		w.pushAt("owner", time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), "project", private, map[string]any{"name": "Private", "sort_key": "a1"})
		rent := task("member", time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), w.project, "Pay rent", nil)
		w.pushAt("member", day(4, 18), "task", rent, map[string]any{"completed_at": day(4, 18).Format(time.RFC3339)})
		gate := task("member", day(3, 18), w.project, "Fix the gate", map[string]any{"due_date": "2026-11-05"})
		secret := task("owner", day(3, 18), private, "Owner's secret", nil)
		w.pushAt("owner", day(5, 18), "task", secret, map[string]any{"completed_at": day(5, 18).Format(time.RFC3339)})
		w.pushAt("member", day(4, 19), "time_entry", uuid.NewString(), map[string]any{"task_id": rent, "user_id": member, "day": "2026-11-04", "minutes": 90})
		w.pushAt("owner", day(5, 19), "time_entry", uuid.NewString(), map[string]any{"task_id": secret, "user_id": w.users["owner"], "day": "2026-11-05", "minutes": 30})
		// A second workspace of Dana's where nothing happened sends nothing.
		quiet := uuid.NewString()
		must(t, w.st.CreateWorkspace(w.ctx, sqlitedb.CreateWorkspaceParams{ID: quiet, Name: "Quiet", Slug: "quiet-" + quiet[:8], Now: store.Now()}))
		must(t, w.st.CreateMembership(w.ctx, sqlitedb.CreateMembershipParams{WorkspaceID: quiet, UserID: member, Role: "owner", Now: store.Now()}))
		w.run()
		w.mail.take()

		send := func(at time.Time) []string {
			t.Helper()
			w.clock = at
			must(t, w.svc.SendWeekly(w.ctx))
			w.run()
			var out []string
			for _, m := range w.mail.take() {
				out = append(out, m.To+"\n"+m.Subject+"\n"+m.Text)
			}
			return out
		}
		// Lee's comes on Sunday at 7:00 UTC, about November 1 to 7.
		if sent := send(day(8, 6)); len(sent) != 0 {
			t.Fatalf("before the hour: %v", sent)
		}
		sent := send(day(8, 7))
		if len(sent) != 1 || !strings.HasPrefix(sent[0], "guest-") {
			t.Fatalf("on Sunday: %v", sent)
		}
		for _, line := range []string{
			"Team: your week, 1 to 7 Nov\n",
			"Here is last week in Team, 1 to 7 Nov, against the week before.",
			"- 1 task done, up from 0\n",
			"- Home: 1 done, 1 open, 1 overdue\n  " + base + "/projects/" + w.project + "?ws=" + w.ws + "\n",
			"Time logged\n\n- You: 0m\n\n",
			"See more in Reports: " + base + "/reports?ws=" + w.ws + "\n",
			"It counts what you can see in Team.",
			"/settings/notifications",
		} {
			if !strings.Contains(sent[0], line) {
				t.Fatalf("Lee's lacks %q:\n%s", line, sent[0])
			}
		}
		for _, absent := range []string{"Everyone", "Private", "secret", "Needs you"} {
			if strings.Contains(sent[0], absent) {
				t.Fatalf("Lee's has %q:\n%s", absent, sent[0])
			}
		}

		// Dana's on Monday at 7:00 in Chicago, 13:00 UTC, about November 2 to 8.
		if sent := send(day(9, 12)); len(sent) != 0 {
			t.Fatalf("before Dana's hour: %v", sent)
		}
		sent = send(day(9, 13))
		if len(sent) != 1 || !strings.HasPrefix(sent[0], "member-") {
			t.Fatalf("on Monday: %v", sent)
		}
		for _, line := range []string{
			"Team: your week, Nov 2 to 8\n",
			"Hi Dana,\n\nHere is last week in Team, Nov 2 to 8, against the week before.\n\n",
			"- 2 tasks done, up from 0\n",
			"- 2 tasks made, up from 0\n",
			"- 1 task overdue as the week ended, up from 0\n",
			"Projects\n\n- Home: 1 done, 1 open, 1 overdue\n",
			"- Private: 1 done, 0 open\n",
			"Time logged\n\n- You: 1h 30m\n- Everyone: 2h\n",
			"Needs you\n\n- Fix the gate: 4 days overdue\n  " + base + "/?task=" + gate + "&ws=" + w.ws + "\n",
		} {
			if !strings.Contains(sent[0], line) {
				t.Fatalf("Dana's lacks %q:\n%s", line, sent[0])
			}
		}
		if strings.Contains(sent[0], "median") {
			t.Fatalf("a median with nothing that went in progress:\n%s", sent[0])
		}
		// Once a week, though the hour goes on.
		if sent := send(day(9, 14)); len(sent) != 0 {
			t.Fatalf("a second one: %v", sent)
		}
		var weeks int
		row := w.st.DB().QueryRowContext(w.ctx, "SELECT COUNT(*) FROM weekly_reports WHERE user_id = '"+member+"'")
		must(t, row.Scan(&weeks))
		if weeks != 2 {
			t.Fatalf("recorded %d weeks for Dana, want Team's and Quiet's", weeks)
		}
		// The owner never turned it on.
		w.clock = day(16, 13)
		must(t, w.svc.SendWeekly(w.ctx))
		w.run()
		for _, m := range w.mail.take() {
			if strings.HasPrefix(m.To, "owner-") {
				t.Fatal("the owner got one")
			}
		}
	})
}

// flakyWeekly is sync whose weekly read fails while down, counting reads.
type flakyWeekly struct {
	*syncer.Service
	mu    sync.Mutex
	reads int
	down  bool
}

func (f *flakyWeekly) WeeklyInput(ctx context.Context, userID, workspaceID string) (reports.Input, error) {
	f.mu.Lock()
	f.reads++
	down := f.down
	f.mu.Unlock()
	if down {
		return reports.Input{}, errors.New("the database is busy")
	}
	return f.Service.WeeklyInput(ctx, userID, workspaceID)
}

func (f *flakyWeekly) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *flakyWeekly) set(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

// The minute's pass only queues weekly reports: each person's is read and
// written in a job of its own, beside the reminder scan, and one that
// fails is tried again after a growing wait, not every minute.
func TestWeeklyReportsAreJobsOfTheirOwn(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		member := w.users["member"]
		_, _, err := w.svc.SavePrefs(w.ctx, member, notify.Prefs{Digest: notify.Digest{Hour: 7}}, &notify.Weekly{Enabled: true})
		must(t, err)
		done := time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)
		w.pushAt("member", done, "task", uuid.NewString(), map[string]any{"title": "Pay rent", "sort_key": "a0", "created_by": member,
			"project_id": w.project, "completed_at": done.Format(time.RFC3339)})
		w.run()
		w.mail.take()

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		runner := jobs.New(w.st, log, func() time.Time { return w.clock })
		flaky := &flakyWeekly{Service: w.sync, down: true}
		svc := notify.New(w.st, log, runner, w.mail, flaky, w.poker, notify.Config{BaseURL: base, Email: true})
		// pass is a minute's look, then the jobs due.
		pass := func(at time.Time) []mail.Message {
			t.Helper()
			w.clock = at
			must(t, svc.SendWeekly(w.ctx))
			if _, err := runner.RunDue(w.ctx); err != nil {
				t.Fatal(err)
			}
			return w.mail.take()
		}
		monday := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)
		w.clock = monday
		must(t, svc.SendWeekly(w.ctx))
		if n := flaky.count(); n != 0 {
			t.Fatalf("the minute's pass read the workspace %d times", n)
		}
		if sent := pass(monday); flaky.count() != 1 || len(sent) != 0 {
			t.Fatalf("first try: %d reads, sent %v", flaky.count(), sent)
		}
		// Not again a minute on, as before, but after its wait.
		for _, step := range []struct {
			after time.Duration
			reads int
		}{{20 * time.Second, 1}, {40 * time.Second, 2}, {80 * time.Second, 2}} {
			if sent := pass(monday.Add(step.after)); flaky.count() != step.reads || len(sent) != 0 {
				t.Fatalf("%s on: %d reads, want %d; sent %v", step.after, flaky.count(), step.reads, sent)
			}
		}
		flaky.set(false)
		sent := pass(monday.Add(3 * time.Minute))
		if flaky.count() != 3 || len(sent) != 1 || sent[0].To != "member-"+member[:8]+"@example.com" || !strings.Contains(sent[0].Text, "1 task done") {
			t.Fatalf("once it works: %d reads, sent %v", flaky.count(), sent)
		}
		if sent := pass(monday.Add(10 * time.Minute)); len(sent) != 0 || flaky.count() != 3 {
			t.Fatalf("a second one: %d reads, sent %v", flaky.count(), sent)
		}
	})
}
