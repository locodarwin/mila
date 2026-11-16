// Package jobs runs work outside requests: one runner goroutine polls the
// jobs table, claims what is due and hands it to the handler registered
// for its kind. A failing job is retried with growing gaps, then left
// failed with its last error. The runner also drives periodic work (the
// trash purge, the reminder and digest scans) from a loop of its own, so a
// long run of jobs never delays it.
//
// Jobs run one at a time, except kinds registered with HandleConcurrent:
// those wait on other people's servers (push services, the mail server),
// so they run a few at a time beside the rest, and a server that never
// answers holds up only its own kind. Slow work (exports, imports,
// backups) runs in the slow lane, one job at a time beside everything
// else, so a large export never delays a reminder.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	// MaxAttempts is how often a job runs before it is left failed.
	MaxAttempts = 6
	// lease is how long a claimed job is held; a runner that dies mid-job
	// lets it be claimed again after this.
	lease = 5 * time.Minute
	// firstRetry doubles after each failure: 30s, 1m, 2m, 4m, 8m.
	firstRetry = 30 * time.Second
	// keepDone is how long finished jobs stay, for looking into problems.
	keepDone = 7 * 24 * time.Hour
	// keepFailed is how long a job that gave up stays.
	keepFailed = 30 * 24 * time.Hour
	// recordFor is how long recording a job's outcome may take once the
	// runner is stopping.
	recordFor = 10 * time.Second
	// Poll is how often the runner looks for due jobs and periodic work
	// when nothing wakes it.
	Poll   = 30 * time.Second
	batch  = 20
	tidyAt = time.Hour
	// SlowLane starts the kind of every job in the slow lane.
	SlowLane = "slow."
	// slowLease is how long a slow job is held: they may take a while,
	// and one taken again while still running would do its work twice.
	slowLease = 6 * time.Hour
)

// Handler does one job. An error schedules a retry.
type Handler func(ctx context.Context, payload []byte) error

// lane runs one kind of job beside the others, up to workers at a time.
type lane struct {
	h       Handler
	workers int
	busy    int // guarded by Runner.mu
}

type periodic struct {
	name  string
	every time.Duration
	fn    func(ctx context.Context) error
	next  time.Time
}

// Runner claims and runs due jobs.
type Runner struct {
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	handlers map[string]Handler
	lanes    map[string]*lane
	// slow holds the slow lane's handlers by kind; slowBusy is whether
	// its one job is under way.
	slow     map[string]Handler
	slowBusy bool
	periodic []*periodic
	tidied   time.Time
	wake     chan struct{}
	// poke wakes the loop that starts concurrent jobs, and tock the one
	// that runs periodic work.
	poke chan struct{}
	tock chan struct{}
	// running counts concurrent jobs under way.
	running sync.WaitGroup
}

// New makes a runner. now is the clock; nil means store.Now.
func New(st *store.Store, log *slog.Logger, now func() time.Time) *Runner {
	if now == nil {
		now = store.Now
	}
	return &Runner{st: st, log: log, now: now, handlers: map[string]Handler{}, lanes: map[string]*lane{}, slow: map[string]Handler{},
		wake: make(chan struct{}, 1), poke: make(chan struct{}, 1), tock: make(chan struct{}, 1)}
}

// Now is the runner's clock, truncated as the database stores times.
func (r *Runner) Now() time.Time { return r.now().UTC().Truncate(time.Microsecond) }

// Handle registers the handler for a kind of job.
func (r *Runner) Handle(kind string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = h
}

// HandleConcurrent registers the handler for a kind of job that runs
// beside the others, up to workers at a time: work that waits on servers
// outside the operator's control, which must not hold up the rest.
func (r *Runner) HandleConcurrent(kind string, workers int, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lanes[kind] = &lane{h: h, workers: max(workers, 1)}
}

// HandleSlow registers the handler for a kind of slow job, whose kind
// starts with SlowLane. Slow jobs run one at a time in a lane of their
// own, held for hours rather than minutes, so the work that must be on
// time never waits behind them.
func (r *Runner) HandleSlow(kind string, h Handler) {
	if !strings.HasPrefix(kind, SlowLane) {
		panic("jobs: slow job kind " + kind + " must start with " + SlowLane)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slow[kind] = h
}

// concurrent reports whether a kind runs outside the serial loop.
func (r *Runner) concurrent(kind string) bool {
	if strings.HasPrefix(kind, SlowLane) {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lanes[kind] != nil
}

// Every runs fn about every d from the runner's periodic loop, first when
// it starts. Errors are logged; the next run comes as usual.
func (r *Runner) Every(name string, d time.Duration, fn func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.periodic = append(r.periodic, &periodic{name: name, every: d, fn: fn})
}

// Enqueue adds a job inside the caller's transaction, so it exists exactly
// when the change that caused it does. Call Wake once the transaction has
// committed to run it promptly.
func Enqueue(ctx context.Context, q store.Queries, kind string, payload any, runAt, now time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return q.InsertJob(ctx, sqlitedb.InsertJobParams{
		ID: uuid.Must(uuid.NewV7()).String(), Kind: kind, Payload: string(raw), RunAt: runAt, Now: now,
	})
}

// Enqueue adds a job due now and wakes the runner.
func (r *Runner) Enqueue(ctx context.Context, kind string, payload any) error {
	now := r.Now()
	if err := Enqueue(ctx, r.st, kind, payload, now, now); err != nil {
		return err
	}
	r.Wake()
	return nil
}

// Wake makes the runner look for due jobs and periodic work now rather
// than at its next poll.
func (r *Runner) Wake() {
	signal(r.wake)
	signal(r.poke)
	signal(r.tock)
}

func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// Run loops until ctx ends, then waits for the periodic work and the
// concurrent jobs under way.
func (r *Runner) Run(ctx context.Context) {
	var loops sync.WaitGroup
	loops.Add(2)
	go func() {
		defer loops.Done()
		r.feedLoop(ctx)
	}()
	go func() {
		defer loops.Done()
		r.periodicLoop(ctx)
	}()
	defer func() {
		loops.Wait()
		r.running.Wait()
	}()
	tick := time.NewTicker(Poll)
	defer tick.Stop()
	for {
		if _, err := r.runSerial(ctx); err != nil && ctx.Err() == nil {
			r.log.Warn("jobs: run due", "err", err)
		}
		r.tidy(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-r.wake:
		}
	}
}

// periodicLoop runs the periodic work as it comes due, apart from the
// jobs: a run of jobs that each wait on a server that does not answer
// would otherwise hold up the reminder scan for as long.
func (r *Runner) periodicLoop(ctx context.Context) {
	tick := time.NewTicker(Poll)
	defer tick.Stop()
	for {
		r.runPeriodic(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-r.tock:
		}
	}
}

// feedLoop starts concurrent jobs as they come due and as room frees up.
func (r *Runner) feedLoop(ctx context.Context) {
	tick := time.NewTicker(Poll)
	defer tick.Stop()
	for {
		if _, err := r.feed(ctx); err != nil && ctx.Err() == nil {
			r.log.Warn("jobs: start concurrent", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-r.poke:
		}
	}
}

// Pass does everything due once: periodic work, then jobs until none are
// due, concurrent ones included, then the hourly tidy. Tests call it in
// place of Run.
func (r *Runner) Pass(ctx context.Context) {
	r.runPeriodic(ctx)
	if _, err := r.RunDue(ctx); err != nil && ctx.Err() == nil {
		r.log.Warn("jobs: run due", "err", err)
	}
	r.tidy(ctx)
}

// tidy deletes old jobs, once an hour.
func (r *Runner) tidy(ctx context.Context) {
	now := r.Now()
	if now.Sub(r.tidied) >= tidyAt {
		r.tidied = now
		n, err := r.st.DeleteOldJobs(ctx, sqlitedb.DeleteOldJobsParams{
			DoneBefore: sql.NullTime{Time: now.Add(-keepDone), Valid: true}, MaxAttempts: MaxAttempts, FailedBefore: now.Add(-keepFailed),
		})
		if err != nil {
			r.log.Warn("jobs: delete old", "err", err)
		} else if n > 0 {
			r.log.Debug("jobs: deleted old jobs", "jobs", n)
		}
	}
}

func (r *Runner) runPeriodic(ctx context.Context) {
	r.mu.Lock()
	due := []*periodic{}
	now := r.Now()
	for _, p := range r.periodic {
		if !now.Before(p.next) {
			p.next = now.Add(p.every)
			due = append(due, p)
		}
	}
	r.mu.Unlock()
	for _, p := range due {
		if err := p.fn(ctx); err != nil && ctx.Err() == nil {
			r.log.Warn("periodic work failed", "work", p.name, "err", err)
		}
	}
}

// RunDue runs due jobs until none are left, waiting for the concurrent
// ones, and reports how many ran. Tests call it; Run starts concurrent
// jobs without waiting for them.
func (r *Runner) RunDue(ctx context.Context) (int, error) {
	ran := 0
	for ctx.Err() == nil {
		started, err := r.feed(ctx)
		ran += started
		if err != nil {
			r.running.Wait()
			return ran, err
		}
		n, err := r.runSerial(ctx)
		ran += n
		r.running.Wait()
		if err != nil || started+n == 0 {
			return ran, err
		}
	}
	return ran, ctx.Err()
}

// runSerial runs the due jobs of every kind but the concurrent and slow
// ones, one
// at a time, walking the due list once, and reports how many ran.
func (r *Runner) runSerial(ctx context.Context) (int, error) {
	ran := 0
	var afterAt time.Time
	afterID := ""
	for ctx.Err() == nil {
		rows, err := r.st.ListDueJobs(ctx, sqlitedb.ListDueJobsParams{
			MaxAttempts: MaxAttempts, Now: r.Now(), AfterRunAt: afterAt, AfterID: afterID, MaxRows: batch,
		})
		if err != nil || len(rows) == 0 {
			return ran, err
		}
		for _, row := range rows {
			afterAt, afterID = row.RunAt, row.ID
			if r.concurrent(row.Kind) {
				continue
			}
			job, err := r.claim(ctx, row.ID, lease)
			if err != nil {
				return ran, err
			}
			if job == nil {
				continue // taken by another runner in between
			}
			ran++
			r.mu.Lock()
			h := r.handlers[job.Kind]
			r.mu.Unlock()
			if err := r.run(ctx, job, h); err != nil {
				return ran, err
			}
		}
	}
	return ran, ctx.Err()
}

// feed starts due jobs of the concurrent kinds as far as each has room,
// and the next slow job when the slow lane is free, and reports how many
// it started.
func (r *Runner) feed(ctx context.Context) (int, error) {
	r.mu.Lock()
	kinds := make([]string, 0, len(r.lanes))
	for k := range r.lanes {
		kinds = append(kinds, k)
	}
	r.mu.Unlock()
	sort.Strings(kinds)
	started := 0
	for _, kind := range kinds {
		r.mu.Lock()
		l := r.lanes[kind]
		room := l.workers - l.busy
		r.mu.Unlock()
		if room <= 0 {
			continue
		}
		ids, err := r.st.ListDueJobsOfKind(ctx, sqlitedb.ListDueJobsOfKindParams{
			Kind: kind, MaxAttempts: MaxAttempts, Now: r.Now(), MaxRows: int64(room),
		})
		if err != nil {
			return started, err
		}
		for _, id := range ids {
			job, err := r.claim(ctx, id, lease)
			if err != nil {
				return started, err
			}
			if job == nil {
				continue
			}
			started++
			r.mu.Lock()
			l.busy++
			r.mu.Unlock()
			r.running.Add(1)
			go func() {
				defer func() {
					r.mu.Lock()
					l.busy--
					r.mu.Unlock()
					r.running.Done()
					signal(r.poke)
				}()
				if err := r.run(ctx, job, l.h); err != nil && ctx.Err() == nil {
					r.log.Warn("jobs: record result", "kind", job.Kind, "id", job.ID, "err", err)
				}
			}()
		}
	}
	n, err := r.feedSlow(ctx)
	return started + n, err
}

// feedSlow starts the next due slow job if the slow lane is free.
func (r *Runner) feedSlow(ctx context.Context) (int, error) {
	r.mu.Lock()
	idle := !r.slowBusy && len(r.slow) > 0
	r.mu.Unlock()
	if !idle {
		return 0, nil
	}
	ids, err := r.st.ListDueJobsOfLane(ctx, sqlitedb.ListDueJobsOfLaneParams{
		Pattern: SlowLane + "%", MaxAttempts: MaxAttempts, Now: r.Now(), MaxRows: 1,
	})
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	job, err := r.claim(ctx, ids[0], slowLease)
	if err != nil || job == nil {
		return 0, err
	}
	r.mu.Lock()
	r.slowBusy = true
	h := r.slow[job.Kind]
	r.mu.Unlock()
	r.running.Add(1)
	go func() {
		defer func() {
			r.mu.Lock()
			r.slowBusy = false
			r.mu.Unlock()
			r.running.Done()
			signal(r.poke)
		}()
		if err := r.run(ctx, job, h); err != nil && ctx.Err() == nil {
			r.log.Warn("jobs: record result", "kind", job.Kind, "id", job.ID, "err", err)
		}
	}()
	return 1, nil
}

// claim takes a due job for this runner, holding it for hold; nil means
// someone else has it.
func (r *Runner) claim(ctx context.Context, id string, hold time.Duration) (*sqlitedb.Job, error) {
	now := r.Now()
	job, err := r.st.ClaimJob(ctx, sqlitedb.ClaimJobParams{
		ID: id, MaxAttempts: MaxAttempts,
		LockedUntil: sql.NullTime{Time: now.Add(hold), Valid: true},
		Now:         sql.NullTime{Time: now, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// run runs a claimed job with its handler and records how it went. The
// outcome is recorded even when the runner is stopping, or a job that did
// its work (an email that went out) would run again after a restart.
func (r *Runner) run(ctx context.Context, job *sqlitedb.Job, h Handler) error {
	id := job.ID
	var runErr error
	if h == nil {
		runErr = fmt.Errorf("no handler for jobs of kind %q", job.Kind)
	} else {
		runErr = safely(ctx, h, []byte(job.Payload))
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordFor)
	defer cancel()
	if runErr == nil {
		return r.st.FinishJob(ctx, sqlitedb.FinishJobParams{ID: id, Now: sql.NullTime{Time: r.Now(), Valid: true}})
	}
	if job.Attempts >= MaxAttempts {
		r.log.Error("job failed for good", "kind", job.Kind, "id", id, "attempts", job.Attempts, "err", runErr)
	} else {
		r.log.Warn("job failed; will retry", "kind", job.Kind, "id", id, "attempts", job.Attempts, "err", runErr)
	}
	return r.st.RetryJob(ctx, sqlitedb.RetryJobParams{
		ID: id, RunAt: r.Now().Add(Backoff(int(job.Attempts))),
		LastError: sql.NullString{String: truncate(runErr.Error(), 2000), Valid: true},
	})
}

// Backoff is the wait before the next try after a job's nth failure.
func Backoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	return firstRetry << (n - 1)
}

// safely runs a handler, turning a panic into an error so one bad job
// cannot stop the runner.
func safely(ctx context.Context, h Handler, payload []byte) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return h(ctx, payload)
}

// truncate cuts an error's text, which may quote an outside server, to at
// most n bytes, on a character's edge and as valid UTF-8 for Postgres.
func truncate(s string, n int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
