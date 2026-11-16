package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/store"
)

// CheckHLC exposes checkHLC to the external tests.
var CheckHLC = checkHLC

// Restored lists every row of a workspace, deleted ones too, whoever may
// see them: what a restore wrote.
func (s *Service) Restored(ctx context.Context, workspaceID string) ([]Row, error) {
	d := s.st.DB()
	var out []Row
	for _, name := range syncOrder {
		rows, err := listAll(ctx, d, d.Dialect, entities[name], workspaceID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, r.snapshot(entities[name]))
		}
	}
	return out, nil
}

// Fingerprint sums up the synced entities as an older app would meet
// them: each entity's fields, their kinds and which are required.
func Fingerprint() string {
	names := slices.Sorted(maps.Keys(entities))
	h := sha256.New()
	for _, n := range names {
		_, _ = fmt.Fprintf(h, "%s\n", n)
		for _, f := range entities[n].fields {
			_, _ = fmt.Fprintf(h, "  %s %d required=%t enum=%v\n", f.name, f.kind, f.required, f.enum)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// The REST internals the external tests check.
var (
	StampAfter  = stampAfter
	KeyAfter    = keyAfter
	ValidateKey = validateKey
)

// SetNow sets the service's clock, which stamps REST writes.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

// BareDefaults makes one row of every entity holding only its required
// fields, so the migrations' DEFAULTs fill the rest, and lists every
// field whose stored value is not the def a REST create would give it.
func (s *Service) BareDefaults(ctx context.Context, workspaceID, userID string) ([]string, error) {
	d := s.st.DB()
	made := map[string]string{} // entity -> the ID of its bare row
	var wrong []string
	for _, name := range syncOrder {
		e := entities[name]
		id := uuid.NewString()
		cols := []string{"id", "workspace_id", "clocks", "created_at", "updated_at"}
		now := store.Now()
		args := []any{id, workspaceID, "{}", now, now}
		for _, f := range e.fields {
			if !f.required {
				continue
			}
			var v any
			switch f.kind {
			case kString, kText:
				v = "x"
			case kEnum:
				v = f.enum[0]
			case kTime:
				v = now
			case kRef:
				v = made[f.ref]
			case kSelf, kMember:
				v = userID
			case kInt:
				v = int64(1)
			case kDate:
				v = "2026-10-28"
			case kJSON:
				v = "1"
			case kBool:
				v = true
			}
			cols = append(cols, f.name)
			args = append(args, v)
		}
		if _, err := d.ExecContext(ctx, rebind(d.Dialect, "INSERT INTO "+e.table+" ("+strings.Join(cols, ", ")+") VALUES ("+marks(len(cols))+")"), args...); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		made[name] = id
		r, err := loadRow(ctx, d, d.Dialect, e, id)
		if err != nil {
			return nil, err
		}
		for i := range e.fields {
			f := &e.fields[i]
			def, ok := defaultOf(f)
			if !ok {
				continue
			}
			want, _ := json.Marshal(def)
			got, _ := json.Marshal(jsonValue(r.values[f.name]))
			if string(want) != string(got) {
				wrong = append(wrong, fmt.Sprintf("%s.%s: the database gives %s, def is %s", name, f.name, got, want))
			}
		}
	}
	return wrong, nil
}

// SetMaxHistory changes how many bytes of a page's history are kept, until
// the test ends.
func SetMaxHistory(t interface{ Cleanup(func()) }, n int64) {
	was := maxHistory
	maxHistory = n
	t.Cleanup(func() { maxHistory = was })
}
