package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/store"
)

// fakeClock is a settable clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func setup(t *testing.T, d *db.DB) (*jobs.Runner, *store.Store, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}
	st := store.New(d)
	return jobs.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.now), st, clock
}

func TestRunsJobsByKind(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r, st, clock := setup(t, d)
		var got []string
		r.Handle("greet", func(_ context.Context, payload []byte) error {
			var p struct{ Name string }
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			got = append(got, p.Name)
			return nil
		})
		if err := r.Enqueue(ctx, "greet", map[string]string{"Name": "Dana"}); err != nil {
			t.Fatal(err)
		}
		// A job for later waits for its time.
		if err := jobs.Enqueue(ctx, st, "greet", map[string]string{"Name": "Lee"}, clock.now().Add(time.Hour), clock.now()); err != nil {
			t.Fatal(err)
		}
		if n, err := r.RunDue(ctx); err != nil || n != 1 {
			t.Fatalf("RunDue = %d, %v; want 1", n, err)
		}
		if n, _ := r.RunDue(ctx); n != 0 {
			t.Fatalf("a done job ran again (%d)", n)
		}
		clock.advance(time.Hour)
		if n, _ := r.RunDue(ctx); n != 1 {
			t.Fatalf("the later job did not run when due (%d)", n)
		}
		if len(got) != 2 || got[0] != "Dana" || got[1] != "Lee" {
			t.Fatalf("handled %v", got)
		}
	})
}

func TestRetriesWithBackoffThenGivesUp(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r, st, clock := setup(t, d)
		calls := 0
		r.Handle("flaky", func(context.Context, []byte) error {
			calls++
			return errors.New("the mail server is down")
		})
		if err := r.Enqueue(ctx, "flaky", nil); err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= jobs.MaxAttempts; attempt++ {
			if n, _ := r.RunDue(ctx); n != 1 {
				t.Fatalf("attempt %d: ran %d jobs", attempt, n)
			}
			if attempt < jobs.MaxAttempts {
				// Not before the backoff has passed.
				clock.advance(jobs.Backoff(attempt) - time.Second)
				if n, _ := r.RunDue(ctx); n != 0 {
					t.Fatalf("attempt %d: retried before its backoff", attempt)
				}
				clock.advance(time.Second)
			}
		}
		clock.advance(24 * time.Hour)
		if n, _ := r.RunDue(ctx); n != 0 || calls != jobs.MaxAttempts {
			t.Fatalf("a failed job kept running: %d calls", calls)
		}
		ids := listJobs(t, st)
		if len(ids) != 1 {
			t.Fatalf("jobs = %v", ids)
		}
		job, err := st.GetJob(ctx, ids[0])
		if err != nil {
			t.Fatal(err)
		}
		if job.DoneAt.Valid || job.LastError.String != "the mail server is down" || job.Attempts != jobs.MaxAttempts {
			t.Fatalf("failed job = %+v", job)
		}
	})
}

func TestRecoversAndForgetsOldJobs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r, st, clock := setup(t, d)
		fail := true
		r.Handle("once", func(context.Context, []byte) error {
			if fail {
				fail = false
				panic("boom")
			}
			return nil
		})
		if err := r.Enqueue(ctx, "once", nil); err != nil {
			t.Fatal(err)
		}
		// A panic is a failure like any other, and the retry succeeds.
		r.Pass(ctx)
		clock.advance(jobs.Backoff(1))
		r.Pass(ctx)
		job, _ := st.GetJob(ctx, listJobs(t, st)[0])
		if !job.DoneAt.Valid || job.LastError.Valid {
			t.Fatalf("job after a retry = %+v", job)
		}
		// A job with no handler fails rather than vanishing.
		if err := r.Enqueue(ctx, "unknown", nil); err != nil {
			t.Fatal(err)
		}
		r.Pass(ctx)
		// Done jobs go after a week, at the hourly tidy.
		clock.advance(7*24*time.Hour + time.Hour)
		r.Pass(ctx)
		left := listJobs(t, st)
		if len(left) != 1 {
			t.Fatalf("jobs left = %v", left)
		}
		if j, _ := st.GetJob(ctx, left[0]); j.Kind != "unknown" {
			t.Fatalf("kept %q", j.Kind)
		}
	})
}

func TestPeriodicWork(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r, _, clock := setup(t, d)
		runs := 0
		r.Every("count", time.Minute, func(context.Context) error { runs++; return nil })
		r.Pass(ctx)
		r.Pass(ctx)
		if runs != 1 {
			t.Fatalf("ran %d times in the first minute", runs)
		}
		clock.advance(time.Minute)
		r.Pass(ctx)
		if runs != 2 {
			t.Fatalf("ran %d times after a minute", runs)
		}
	})
}

func TestTwoRunnersNeverShareAJob(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r1, st, clock := setup(t, d)
		r2 := jobs.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.now)
		var mu sync.Mutex
		seen := map[int]int{}
		h := func(_ context.Context, payload []byte) error {
			var n int
			_ = json.Unmarshal(payload, &n)
			mu.Lock()
			seen[n]++
			mu.Unlock()
			return nil
		}
		r1.Handle("n", h)
		r2.Handle("n", h)
		for i := range 40 {
			if err := jobs.Enqueue(ctx, st, "n", i, clock.now(), clock.now()); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		for _, r := range []*jobs.Runner{r1, r2} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := r.RunDue(ctx); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if len(seen) != 40 {
			t.Fatalf("ran %d distinct jobs, want 40", len(seen))
		}
		for n, c := range seen {
			if c != 1 {
				t.Fatalf("job %d ran %d times", n, c)
			}
		}
	})
}

// Jobs that wait on other people's servers run beside the rest, a few at
// a time, so a server that never answers holds up only its own kind.
func TestConcurrentKindsDoNotHoldUpTheRest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r, st, _ := setup(t, d)
		release := make(chan struct{})
		var mu sync.Mutex
		inFlight, most := 0, 0
		r.HandleConcurrent("hang", 2, func(ctx context.Context, _ []byte) error {
			mu.Lock()
			inFlight++
			most = max(most, inFlight)
			mu.Unlock()
			defer func() {
				mu.Lock()
				inFlight--
				mu.Unlock()
			}()
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		quick := make(chan struct{})
		r.Handle("quick", func(context.Context, []byte) error {
			close(quick)
			return nil
		})
		ticks := 0
		r.Every("scan", time.Hour, func(context.Context) error {
			mu.Lock()
			ticks++
			mu.Unlock()
			return nil
		})
		for i := range 5 {
			if err := r.Enqueue(ctx, "hang", i); err != nil {
				t.Fatal(err)
			}
		}
		stopped := make(chan struct{})
		go func() {
			r.Run(ctx)
			close(stopped)
		}()
		waitFor(t, "two hanging jobs to start", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return inFlight == 2
		})
		if err := r.Enqueue(ctx, "quick", nil); err != nil {
			t.Fatal(err)
		}
		select {
		case <-quick:
		case <-time.After(5 * time.Second):
			t.Fatal("a job waited behind ones that hang")
		}
		mu.Lock()
		if ticks != 1 {
			t.Errorf("periodic work ran %d times, want 1", ticks)
		}
		mu.Unlock()

		close(release)
		waitFor(t, "every job to finish", func() bool {
			var n int
			_ = st.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE done_at IS NULL").Scan(&n)
			return n == 0
		})
		mu.Lock()
		if most != 2 {
			t.Errorf("at most %d ran at once, want 2", most)
		}
		mu.Unlock()
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after its context ended")
		}
	})
}

// Periodic work (the reminder scan) runs on time while a job takes long,
// such as an email to a mail server that does not answer.
func TestPeriodicWorkDoesNotWaitForJobs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r, _, clock := setup(t, d)
		release, started := make(chan struct{}), make(chan struct{})
		r.Handle("slow", func(ctx context.Context, _ []byte) error {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		})
		var mu sync.Mutex
		ticks := 0
		r.Every("scan", time.Minute, func(context.Context) error {
			mu.Lock()
			ticks++
			mu.Unlock()
			return nil
		})
		if err := r.Enqueue(ctx, "slow", nil); err != nil {
			t.Fatal(err)
		}
		stopped := make(chan struct{})
		go func() {
			r.Run(ctx)
			close(stopped)
		}()
		<-started
		waitFor(t, "the first scan", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return ticks == 1
		})
		clock.advance(time.Minute)
		r.Wake()
		waitFor(t, "a scan while a job takes long", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return ticks == 2
		})
		close(release)
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after its context ended")
		}
	})
}

// A job's outcome is recorded even when the runner stops while it runs, so
// it does not run again (an email sent twice) after a restart.
func TestOutcomeRecordedWhileStopping(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx, cancel := context.WithCancel(context.Background())
		r, st, _ := setup(t, d)
		started := make(chan struct{})
		r.Handle("send", func(context.Context, []byte) error {
			close(started)
			cancel() // the server stops while the message goes out
			return nil
		})
		if err := r.Enqueue(ctx, "send", nil); err != nil {
			t.Fatal(err)
		}
		stopped := make(chan struct{})
		go func() {
			r.Run(ctx)
			close(stopped)
		}()
		<-started
		<-stopped
		var n int
		if err := st.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM jobs WHERE done_at IS NULL").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("a job that did its work while the runner stopped was left to run again")
		}
	})
}

// RunDue, which tests use, waits for concurrent jobs and counts them.
func TestRunDueWaitsForConcurrentJobs(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		r, st, clock := setup(t, d)
		var mu sync.Mutex
		var got []int
		r.HandleConcurrent("send", 2, func(_ context.Context, payload []byte) error {
			var n int
			_ = json.Unmarshal(payload, &n)
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			got = append(got, n)
			mu.Unlock()
			if n == 4 {
				return errors.New("no answer")
			}
			return nil
		})
		for i := range 5 {
			if err := jobs.Enqueue(ctx, st, "send", i, clock.now(), clock.now()); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := r.RunDue(ctx); err != nil || n != 5 {
			t.Fatalf("RunDue = %d, %v; want 5", n, err)
		}
		mu.Lock()
		handled := len(got)
		mu.Unlock()
		if handled != 5 {
			t.Fatalf("handled %d jobs", handled)
		}
		// The failure is retried later, as for any job.
		if n, _ := r.RunDue(ctx); n != 0 {
			t.Fatalf("the failed job ran again at once (%d)", n)
		}
		clock.advance(jobs.Backoff(1))
		if n, _ := r.RunDue(ctx); n != 1 {
			t.Fatalf("the failed job was not retried (%d)", n)
		}
	})
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func listJobs(t *testing.T, st *store.Store) []string {
	t.Helper()
	rows, err := st.DB().QueryContext(context.Background(), "SELECT id FROM jobs ORDER BY created_at, id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// A slow job (an export) runs beside the others, one at a time, and
// holds up neither ordinary jobs nor another slow kind's turn for long.
func TestSlowLaneRunsBesideTheRest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r, st, _ := setup(t, d)
		release := make(chan struct{})
		var mu sync.Mutex
		inFlight, most, done := 0, 0, 0
		slow := func(ctx context.Context, _ []byte) error {
			mu.Lock()
			inFlight++
			most = max(most, inFlight)
			mu.Unlock()
			defer func() {
				mu.Lock()
				inFlight--
				done++
				mu.Unlock()
			}()
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		r.HandleSlow("slow.export", slow)
		r.HandleSlow("slow.import", slow)
		quick := make(chan struct{})
		r.Handle("quick", func(context.Context, []byte) error {
			close(quick)
			return nil
		})
		for _, kind := range []string{"slow.export", "slow.import", "slow.export"} {
			if err := r.Enqueue(ctx, kind, nil); err != nil {
				t.Fatal(err)
			}
		}
		stopped := make(chan struct{})
		go func() {
			r.Run(ctx)
			close(stopped)
		}()
		waitFor(t, "a slow job to start", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return inFlight == 1
		})
		if err := r.Enqueue(ctx, "quick", nil); err != nil {
			t.Fatal(err)
		}
		select {
		case <-quick:
		case <-time.After(5 * time.Second):
			t.Fatal("an ordinary job waited behind a slow one")
		}
		// A slow job is held long enough not to be taken again while it runs.
		var locked time.Time
		if err := st.DB().QueryRowContext(ctx, "SELECT locked_until FROM jobs WHERE kind = 'slow.export' AND locked_until IS NOT NULL LIMIT 1").Scan(&locked); err != nil {
			t.Fatal(err)
		}
		if held := locked.Sub(r.Now()); held < time.Hour {
			t.Errorf("a slow job is held for %v", held)
		}
		close(release)
		waitFor(t, "every slow job to finish", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return done == 3
		})
		mu.Lock()
		if most != 1 {
			t.Errorf("%d slow jobs ran at once, want 1", most)
		}
		mu.Unlock()
		cancel()
		<-stopped
	})
}

func TestSlowKindsNeedThePrefix(t *testing.T) {
	r := jobs.New(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	defer func() {
		if recover() == nil {
			t.Error("a slow kind without the prefix was taken")
		}
	}()
	r.HandleSlow("export", func(context.Context, []byte) error { return nil })
}
