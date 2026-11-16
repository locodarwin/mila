package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// The backup status lives in instance_setting under these keys, as RFC
// 3339 times and the error's text, for the admin page.
const (
	keyLastOK      = "backup.last_ok_at"
	keyLastError   = "backup.last_error"
	keyLastErrorAt = "backup.last_error_at"
	// keySlot is the nightly slot last queued, so a restart neither skips
	// a night nor backs up twice.
	keySlot = "backup.nightly_slot"
)

// jobBackup is a backup made by the job runner, in the slow lane.
const jobBackup = jobs.SlowLane + "backup"

// checkEvery is how often the server looks whether a nightly backup is
// due.
const checkEvery = 5 * time.Minute

// Config is how the service runs.
type Config struct {
	// Dir holds the backups.
	Dir string
	// Nightly turns the nightly backup on.
	Nightly bool
	// Hour is the hour of the day, UTC, from which the nightly backup is
	// due.
	Hour int
	// BaseURL is the instance's address, for the failure email.
	BaseURL string
	// Mail queues an email; nil sends none.
	Mail func(ctx context.Context, m mail.Message) error
}

// Service makes backups for the server and the commands, and keeps their
// status.
type Service struct {
	st     *store.Store
	log    *slog.Logger
	cfg    Config
	runner *jobs.Runner
	now    func() time.Time
}

// New builds the service. With a runner, it registers the backup job and,
// if nightly backups are on, the check that queues them; the commands
// pass nil.
func New(st *store.Store, log *slog.Logger, runner *jobs.Runner, cfg Config) *Service {
	s := &Service{st: st, log: log, cfg: cfg, runner: runner, now: store.Now}
	if runner != nil {
		s.now = runner.Now
		runner.HandleSlow(jobBackup, s.handle)
		if cfg.Nightly {
			runner.Every("backup schedule", checkEvery, s.Schedule)
		}
	}
	return s
}

// Dir is where the backups are.
func (s *Service) Dir() string { return s.cfg.Dir }

// Nightly reports whether the nightly backup is on, and its hour (UTC).
func (s *Service) Nightly() (bool, int) { return s.cfg.Nightly, s.cfg.Hour }

// Slot is the nightly backup a moment falls after: hour:00 UTC that day,
// or the day before if that is still to come.
func Slot(now time.Time, hour int) time.Time {
	now = now.UTC()
	slot := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if now.Before(slot) {
		slot = slot.AddDate(0, 0, -1)
	}
	return slot
}

// Schedule queues the nightly backup once its slot has come and it has
// not been queued for that slot yet: on time, or as soon as the server
// is back if it was off. The first time it runs it only notes the slot,
// so a new instance's first backup is the next night's.
func (s *Service) Schedule(ctx context.Context) error {
	slot := Slot(s.now(), s.cfg.Hour)
	last, ok, err := s.st.InstanceSetting(ctx, keySlot)
	if err != nil {
		return err
	}
	if !ok {
		return s.st.SetInstanceSetting(ctx, keySlot, slot.Format(time.RFC3339))
	}
	if at, err := time.Parse(time.RFC3339, last); err == nil && !at.Before(slot) {
		return nil
	}
	now := s.now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := jobs.Enqueue(ctx, q, jobBackup, request{Kind: Nightly}, now, now); err != nil {
			return err
		}
		return q.UpsertInstanceSetting(ctx, sqlitedb.UpsertInstanceSettingParams{Key: keySlot, Value: slot.Format(time.RFC3339), UpdatedAt: now})
	})
	if err != nil {
		return err
	}
	s.runner.Wake()
	return nil
}

type request struct {
	Kind Kind `json:"kind"`
}

// Request queues a backup to be made now in the slow lane: the admin
// page's "Back up now".
func (s *Service) Request(ctx context.Context) error {
	if s.runner == nil {
		return errors.New("backup: no job runner")
	}
	return s.runner.Enqueue(ctx, jobBackup, request{Kind: Manual})
}

// handle makes a queued backup. Like every slow job it records its own
// failure, in the backup status and an email, and is not retried. One cut
// short by the server stopping is no failure: it fails the job instead,
// so the runner makes the backup again after the restart.
func (s *Service) handle(ctx context.Context, payload []byte) error {
	var r request
	if err := json.Unmarshal(payload, &r); err != nil || !r.Kind.valid() {
		return fmt.Errorf("backup job: bad payload %q", payload)
	}
	if _, err := s.Run(ctx, r.Kind); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// Run makes a backup now, prunes old ones, and records the outcome. The
// first failure after a success emails the instance admins; later ones
// in the same run of failures only update the status.
func (s *Service) Run(ctx context.Context, kind Kind) (Info, error) {
	info, err := Snapshot(ctx, s.st.DB(), s.cfg.Dir, kind, s.now())
	if err != nil && ctx.Err() != nil {
		// Stopped, not broken: nothing to report.
		s.log.Info("backup stopped before it finished", "err", err)
		return Info{}, err
	}
	if err != nil {
		s.failed(ctx, err)
		return Info{}, err
	}
	s.log.Info("backed up the database", "file", filepath.Join(s.cfg.Dir, info.Name), "bytes", info.Size)
	if removed, err := Prune(s.cfg.Dir); err != nil {
		s.log.Warn("prune backups", "err", err)
	} else if len(removed) > 0 {
		s.log.Info("removed old backups", "files", len(removed))
	}
	if err := s.st.SetInstanceSetting(ctx, keyLastOK, info.At.Format(time.RFC3339)); err != nil {
		s.log.Warn("record backup status", "err", err)
	}
	return info, nil
}

func (s *Service) failed(ctx context.Context, cause error) {
	s.log.Error("backup failed", "err", cause)
	ctx = context.WithoutCancel(ctx)
	st, err := s.Status(ctx)
	if err != nil {
		s.log.Warn("read backup status", "err", err)
		return
	}
	// A run of failures has begun if the last thing that happened was a
	// success, or nothing has yet.
	first := st.LastErrorAt == nil || (st.LastOKAt != nil && !st.LastOKAt.Before(*st.LastErrorAt))
	now := s.now()
	for key, value := range map[string]string{keyLastError: cause.Error(), keyLastErrorAt: now.Format(time.RFC3339)} {
		if err := s.st.SetInstanceSetting(ctx, key, value); err != nil {
			s.log.Warn("record backup status", "err", err)
		}
	}
	if first {
		if err := s.tellAdmins(ctx, cause, now); err != nil {
			s.log.Warn("email about the failed backup", "err", err)
		}
	}
}

// tellAdmins emails every instance admin that a backup failed.
func (s *Service) tellAdmins(ctx context.Context, cause error, at time.Time) error {
	if s.cfg.Mail == nil {
		return nil
	}
	admins, err := s.st.ListInstanceAdmins(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, u := range admins {
		local := at.In(accounts.Location(u))
		style := accounts.DateStyleOf(u)
		when := fmt.Sprintf("%s at %s (%s)", style.Day(local, true, local), style.Time(local.Format("15:04")), accounts.Timezone(u))
		var b strings.Builder
		fmt.Fprintf(&b, "Mila could not back up its database on %s.\n\n", when)
		fmt.Fprintf(&b, "The error was: %s\n\n", cause)
		b.WriteString("Backups go to " + s.cfg.Dir + " on the server. Check that the disk has room and the folder can be written to, then run \"mila backup now\" there to try again.\n\n")
		b.WriteString("You will not get another email about this until a backup has worked again.\n")
		if s.cfg.BaseURL != "" {
			b.WriteString("\n" + strings.TrimSuffix(s.cfg.BaseURL, "/") + "\n")
		}
		errs = append(errs, s.cfg.Mail(ctx, mail.Message{To: u.Email, Subject: "Mila: a backup failed", Text: b.String()}))
	}
	return errors.Join(errs...)
}

// Status is the backup status the admin page shows.
type Status struct {
	LastOKAt    *time.Time `json:"lastOkAt"`
	LastError   string     `json:"lastError"`
	LastErrorAt *time.Time `json:"lastErrorAt"`
	// Recent is every backup kept, newest first.
	Recent []Info `json:"recent"`
}

// Status reads the backup status and lists the backups.
func (s *Service) Status(ctx context.Context) (Status, error) {
	var st Status
	read := func(key string) (string, error) {
		v, _, err := s.st.InstanceSetting(ctx, key)
		return v, err
	}
	when := func(key string) (*time.Time, error) {
		v, err := read(key)
		if err != nil || v == "" {
			return nil, err
		}
		// A value that does not read is no time at all.
		if t, perr := time.Parse(time.RFC3339, v); perr == nil {
			return &t, nil
		}
		return nil, nil
	}
	var err error
	if st.LastOKAt, err = when(keyLastOK); err != nil {
		return st, err
	}
	if st.LastErrorAt, err = when(keyLastErrorAt); err != nil {
		return st, err
	}
	if st.LastError, err = read(keyLastError); err != nil {
		return st, err
	}
	// A directory that cannot be read is what the last error is about;
	// the rest of the status still shows.
	if st.Recent, err = List(s.cfg.Dir); err != nil {
		s.log.Warn("list backups", "err", err)
	}
	if st.Recent == nil {
		st.Recent = []Info{}
	}
	return st, nil
}
