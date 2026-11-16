package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	// Time zone names work even where the system has no zone database,
	// such as a distroless container.
	_ "time/tzdata"

	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
)

// Settings is a user's settings JSON, by key. Each part of the server
// owns its own keys and leaves the others as they are.
type Settings map[string]json.RawMessage

// SettingsOf reads a user's settings; anything unreadable counts as empty.
func SettingsOf(u store.User) Settings {
	s := Settings{}
	if err := json.Unmarshal([]byte(u.Settings), &s); err != nil || s == nil {
		return Settings{}
	}
	return s
}

// UpdateSettings changes a user's settings in a transaction that holds
// their row, so two changes to different keys never undo each other.
func UpdateSettings(ctx context.Context, st *store.Store, userID string, change func(Settings) error) (store.User, error) {
	var u store.User
	err := st.InTx(ctx, func(q store.Queries) error {
		// Locked before reading: on Postgres a plain read would let two
		// changes start from the same settings and the later write win.
		if err := q.LockUser(ctx, userID); err != nil {
			return err
		}
		var err error
		if u, err = q.GetUser(ctx, userID); err != nil {
			return err
		}
		s := SettingsOf(u)
		if err := change(s); err != nil {
			return err
		}
		raw, err := json.Marshal(s)
		if err != nil {
			return err
		}
		u.Settings = string(raw)
		return q.UpdateUserSettings(ctx, sqlitedb.UpdateUserSettingsParams{ID: userID, Settings: u.Settings, Now: store.Now()})
	})
	return u, err
}

// Timezone is the IANA time zone a user's device last reported, or UTC.
func Timezone(u store.User) string {
	var name string
	if raw, ok := SettingsOf(u)["timezone"]; ok && json.Unmarshal(raw, &name) == nil {
		if _, err := loadZone(name); err == nil {
			return name
		}
	}
	return "UTC"
}

// Location is Timezone as a location for time arithmetic.
func Location(u store.User) *time.Location {
	loc, err := loadZone(Timezone(u))
	if err != nil {
		return time.UTC
	}
	return loc
}

func loadZone(name string) (*time.Location, error) {
	if name == "" || strings.EqualFold(name, "local") {
		return nil, fail(Invalid, "%q is not a time zone.", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fail(Invalid, "%q is not a time zone.", name)
	}
	return loc, nil
}

// SetTimezone records the time zone of the user's device, such as
// "America/Chicago", for reminders, due notices, the digest and the weekly
// report, and, when given, how the device writes dates and times (Locale),
// which is what the "auto" date preferences mean in emails and
// notifications.
func (s *Service) SetTimezone(ctx context.Context, userID, name string, l Locale) (store.User, error) {
	if _, err := loadZone(name); err != nil {
		return store.User{}, err
	}
	return UpdateSettings(ctx, s.st, userID, func(set Settings) error {
		raw, _ := json.Marshal(name)
		set["timezone"] = raw
		return setLocale(set, l)
	})
}
