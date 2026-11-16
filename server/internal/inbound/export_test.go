package inbound

import (
	"context"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// CheckOnce is one check of the mailbox, as Run makes.
func (r *Reader) CheckOnce(ctx context.Context) error { return r.check(ctx) }

// PositionParams is how far a test says Mila has read.
func PositionParams(raw string) sqlitedb.UpsertInstanceSettingParams {
	return sqlitedb.UpsertInstanceSettingParams{Key: positionKey, Value: raw, UpdatedAt: store.Now()}
}

// DeliverRaw deals with a whole message as a mailbox would hand it over.
func (r *Reader) DeliverRaw(ctx context.Context, raw []byte) (bool, error) {
	head := raw
	if len(head) > maxHeaderRead {
		head = head[:maxHeaderRead]
	}
	return r.deliverParts(ctx, head, int64(len(raw)), func() ([]byte, error) { return raw, nil })
}

// Limits a test needs to reach.
const (
	MaxFiles       = maxFiles
	MaxMessage     = maxMessage
	MaxBoundaries  = maxBoundaries
	MaxRefusedDay  = maxRefusedListed
	MaxBytesPerDay = maxBytesPerDay
)

// FolderOf is the folder a file:// URL names.
var FolderOf = folderOf

// Expire forgets the access token kept, so the next check asks Microsoft for one.
func (m *Microsoft) Expire() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.access = ""
}
