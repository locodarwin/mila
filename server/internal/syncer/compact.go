package syncer

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

const (
	// LogKeep is how long the change log keeps a change. A device that
	// has not pulled for longer bootstraps again; its unsent edits are
	// pushed first and are in the copy it gets.
	LogKeep = 30 * 24 * time.Hour
	// opsKeep is how long an applied op's ID is remembered, so a retry
	// changes nothing twice. An op retried later still changes nothing
	// where its row is as it left it, since the merge takes only newer
	// clocks; it is kept longer than the log for devices that come back
	// after a long time with a push whose answer they never got.
	opsKeep = 90 * 24 * time.Hour
)

// Compacted is what one compaction removed.
type Compacted struct {
	Changes, Ops, Batches int64
}

// Compact deletes the changes logged before now less LogKeep, the op IDs
// older than opsKeep, and what batches changed from once older than the
// trash. Each workspace's log floor is raised in the same transaction as
// its delete, so a pull never skips a change without being told to
// bootstrap.
func (s *Service) Compact(ctx context.Context, now time.Time) (Compacted, error) {
	var out Compacted
	ids, err := s.st.ListAllWorkspaceIDs(ctx)
	if err != nil {
		return out, err
	}
	before := now.Add(-LogKeep).UTC()
	for _, ws := range ids {
		err := s.st.InTx(ctx, func(q store.Queries) error {
			floor, err := q.CompactionFloor(ctx, sqlitedb.CompactionFloorParams{WorkspaceID: ws, Before: before})
			if err != nil || floor == 0 {
				return err
			}
			if err := q.RaiseLogFloor(ctx, sqlitedb.RaiseLogFloorParams{ID: ws, Floor: floor}); err != nil {
				return err
			}
			n, err := q.DeleteChangesThrough(ctx, sqlitedb.DeleteChangesThroughParams{WorkspaceID: ws, Seq: floor})
			out.Changes += n
			return err
		})
		if err != nil {
			return out, err
		}
	}
	n, err := s.st.DeleteOldAppliedOps(ctx, now.Add(-opsKeep).UTC())
	out.Ops = n
	if err != nil {
		return out, err
	}
	// A batch can be undone as long as the trash keeps what it deleted.
	out.Batches, err = s.st.DeleteOldBatchChanges(ctx, now.Add(-TrashDays*24*time.Hour).UTC())
	return out, err
}

// Epoch is the instance's sync epoch, made on first use. Bootstrap hands
// it out and every pull sends it back; it changes when the database is
// restored from a backup, which makes every device bootstrap again.
func (s *Service) Epoch(ctx context.Context) (string, error) {
	s.epochMu.Lock()
	defer s.epochMu.Unlock()
	if s.epoch != "" {
		return s.epoch, nil
	}
	v, err := s.st.InitInstanceSetting(ctx, store.SyncEpochKey, uuid.Must(uuid.NewV7()).String())
	if err != nil {
		return "", err
	}
	s.epoch = v
	return v, nil
}

// CheckEpoch fails with Gone unless a device's epoch is the instance's:
// its copy came from a database that has since been restored, or from
// before epochs existed, and it must bootstrap again.
func (s *Service) CheckEpoch(ctx context.Context, deviceEpoch string) error {
	current, err := s.Epoch(ctx)
	if err != nil {
		return err
	}
	if deviceEpoch != current {
		return apperr.New(apperr.Gone, "This device's copy is out of step with the server and will be reloaded.")
	}
	return nil
}

// firstSchema is the version of a push that names none: apps from before
// the version gate (M5) sync the first version's entities, and so do
// edits they left in an outbox.
const firstSchema = 1

// CheckSchema fails with UpdateRequired unless a push comes from an app
// that syncs this server's version of the entities. It is checked before
// any op, so a refused push changes nothing and the device keeps every
// op to send once it has reloaded.
func CheckSchema(version int) error {
	if version == 0 {
		version = firstSchema
	}
	if version != Schema {
		return apperr.New(apperr.UpdateRequired, "A new version of Mila is available. Reload to keep syncing; your changes are kept until then.")
	}
	return nil
}
