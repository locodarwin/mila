// Package store is the one door to the database. Queries live in
// queries/queries.sql; sqlc generates them once per dialect (sqlitedb,
// pgdb), and gen writes the adapter that presents Postgres through the
// sqlitedb types, so callers see a single Queries interface.
package store

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f ../../sqlc.yaml
//go:generate go run ./gen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/store/pgdb"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Queries is every query the server can make.
type Queries = sqlitedb.Querier

// Row and parameter types, named once for callers.
type (
	User      = sqlitedb.User
	Passkey   = sqlitedb.Passkey
	Session   = sqlitedb.Session
	Workspace = sqlitedb.Workspace
	Invite    = sqlitedb.Invite
)

// SyncEpochKey is the instance setting holding the sync epoch: a value
// that changes whenever the database goes back in time (a restore), so
// every device starts again from a fresh copy rather than trusting change
// numbers the restored database may hand out a second time.
const SyncEpochKey = "sync_epoch"

// ErrNotFound is returned in place of sql.ErrNoRows.
var ErrNotFound = sql.ErrNoRows

// Now is the current time as the database should store it: UTC, at the
// microsecond precision Postgres keeps, so values round-trip exactly.
func Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// NullNow is Now for parameters that set a nullable time column.
func NullNow() sql.NullTime { return sql.NullTime{Time: Now(), Valid: true} }

// Store runs queries against the database, alone or in a transaction.
type Store struct {
	Queries
	db *db.DB
}

// New returns the Store for d's dialect.
func New(d *db.DB) *Store {
	return &Store{Queries: queriesFor(d.Dialect, d.DB), db: d}
}

func queriesFor(dialect db.Dialect, conn sqlitedb.DBTX) Queries {
	if dialect == db.Postgres {
		return pgQueries{pgdb.New(conn)}
	}
	return sqlitedb.New(conn)
}

// InTx runs fn in a transaction, committing if it returns nil.
func (s *Store) InTx(ctx context.Context, fn func(q Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(queriesFor(s.db.Dialect, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

// InTxConn is InTx for callers that also run their own SQL in the
// transaction, such as the sync package's field-by-field row writes.
func (s *Store) InTxConn(ctx context.Context, fn func(q Queries, tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(queriesFor(s.db.Dialect, tx), tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

// DB returns the underlying database, for read-only SQL of its own.
func (s *Store) DB() *db.DB { return s.db }

// InstanceSetting returns a setting's value, and false if it is unset.
func (s *Store) InstanceSetting(ctx context.Context, key string) (string, bool, error) {
	v, err := s.GetInstanceSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// SetInstanceSetting creates or replaces a setting.
func (s *Store) SetInstanceSetting(ctx context.Context, key, value string) error {
	return s.UpsertInstanceSetting(ctx, sqlitedb.UpsertInstanceSettingParams{Key: key, Value: value, UpdatedAt: Now()})
}

// InitInstanceSetting stores value only if the key is unset, and returns
// whichever value is stored afterward. Safe against concurrent callers.
func (s *Store) InitInstanceSetting(ctx context.Context, key, value string) (string, error) {
	err := s.InsertInstanceSettingIfAbsent(ctx, sqlitedb.InsertInstanceSettingIfAbsentParams{Key: key, Value: value, UpdatedAt: Now()})
	if err != nil {
		return "", err
	}
	return s.GetInstanceSetting(ctx, key)
}

// ResetMember makes a member's devices load a workspace afresh, after
// their access to its projects changed: what they pulled before may hold
// rows they no longer see, and rows they now see were never sent. It
// raises the workspace's change number and records it for the member, so
// a pull from before it answers 410 and the device bootstraps again, even
// one that was offline when it happened. Run it in the transaction that
// changes the access.
func ResetMember(ctx context.Context, q Queries, workspaceID, userID string) error {
	seq, err := q.BumpChangeSeq(ctx, workspaceID)
	if err != nil {
		return err
	}
	return q.SetMemberResetSeq(ctx, sqlitedb.SetMemberResetSeqParams{WorkspaceID: workspaceID, UserID: userID, ResetSeq: seq})
}
