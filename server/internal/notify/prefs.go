package notify

import (
	"context"
	"encoding/json"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/store"
)

// Kind is what a notification is about.
type Kind string

const (
	Mention  Kind = "mention"
	Assigned Kind = "assigned"
	Due      Kind = "due"
	Reminder Kind = "reminder"
	Comment  Kind = "comment"
	// PageComment: someone wrote in a thread on a wiki page that the person
	// is already in.
	PageComment Kind = "page_comment"
	// Unblocked: a task someone is assigned to was waiting for one that
	// is now done.
	Unblocked      Kind = "unblocked"
	InviteAccepted Kind = "invite_accepted"
	// ExportReady and ImportDone answer something the person asked for
	// (Inform); they are not in Kinds, since there is nothing to choose.
	ExportReady Kind = "export_ready"
	ImportDone  Kind = "import_done"
	// EmailTask tells someone their email became a task.
	EmailTask Kind = "email_task"
	// Automation is what a rule says: its Notify change, or that it couldn't go on.
	Automation Kind = "automation"
	// WebhookStopped tells who made a webhook that Mila stopped it for failing.
	WebhookStopped Kind = "webhook_stopped"
)

// Kinds lists every kind, in the order settings show them.
var Kinds = []Kind{Mention, Assigned, Comment, PageComment, Unblocked, Due, Reminder, InviteAccepted, EmailTask, Automation}

// Channels says where one kind of notification goes.
type Channels struct {
	App   bool `json:"app"`
	Push  bool `json:"push"`
	Email bool `json:"email"`
}

// Digest is the daily email: whether it goes out, and at what local hour.
type Digest struct {
	Enabled bool `json:"enabled"`
	Hour    int  `json:"hour"`
}

// Weekly is the weekly report by email: whether it goes out. It goes on
// the first day of the person's week, at the digest's hour.
type Weekly struct {
	Enabled bool `json:"enabled"`
}

// Prefs are a user's notification preferences, stored in their settings
// under "notifications".
type Prefs struct {
	Kinds  map[Kind]Channels `json:"kinds"`
	Digest Digest            `json:"digest"`
	Weekly Weekly            `json:"weekly"`
}

// alwaysInApp kinds cannot leave the in-app list, which is their record.
var alwaysInApp = map[Kind]bool{Mention: true, Reminder: true}

// AlwaysInApp reports whether a kind always shows in the app.
func AlwaysInApp(k Kind) bool { return alwaysInApp[k] }

// Defaults are the preferences of someone who never changed them.
func Defaults() Prefs {
	return Prefs{
		Kinds: map[Kind]Channels{
			Mention:        {App: true, Push: true, Email: true},
			Assigned:       {App: true, Push: true},
			Due:            {App: true, Push: true, Email: true},
			Reminder:       {App: true, Push: true, Email: true},
			Comment:        {App: true},
			PageComment:    {App: true},
			Unblocked:      {App: true, Push: true},
			InviteAccepted: {App: true, Email: true},
			EmailTask:      {App: true},
			Automation:     {App: true, Push: true},
		},
		Digest: Digest{Enabled: false, Hour: 7},
	}
}

// PrefsOf reads a user's preferences: the defaults, with whatever they
// stored on top.
func PrefsOf(u store.User) Prefs {
	p := Defaults()
	raw, ok := accounts.SettingsOf(u)["notifications"]
	if !ok {
		return p
	}
	var stored Prefs
	if err := json.Unmarshal(raw, &stored); err != nil {
		return p
	}
	for _, k := range Kinds {
		if c, ok := stored.Kinds[k]; ok {
			p.Kinds[k] = c
		}
	}
	if stored.Digest.Hour >= 0 && stored.Digest.Hour <= 23 {
		p.Digest = stored.Digest
	}
	p.Weekly = stored.Weekly
	return p.normalized()
}

func (p Prefs) normalized() Prefs {
	for k := range alwaysInApp {
		c := p.Kinds[k]
		c.App = true
		p.Kinds[k] = c
	}
	return p
}

// Prefs returns a user's notification preferences.
func (s *Service) Prefs(ctx context.Context, userID string) (Prefs, store.User, error) {
	u, err := s.st.GetUser(ctx, userID)
	if err != nil {
		return Prefs{}, u, err
	}
	return PrefsOf(u), u, nil
}

// SavePrefs replaces a user's preferences. Kinds left out keep their
// current channels, and with weekly nil the weekly report stays as it was
// (an app from before it sends none); mentions and reminders always stay
// in the app.
func (s *Service) SavePrefs(ctx context.Context, userID string, p Prefs, weekly *Weekly) (Prefs, store.User, error) {
	if p.Digest.Hour < 0 || p.Digest.Hour > 23 {
		return Prefs{}, store.User{}, apperr.New(apperr.Invalid, "The digest hour must be from 0 to 23.")
	}
	for k := range p.Kinds {
		if !known(k) {
			return Prefs{}, store.User{}, apperr.New(apperr.Invalid, "%q is not a kind of notification.", k)
		}
	}
	var saved Prefs
	u, err := accounts.UpdateSettings(ctx, s.st, userID, func(set accounts.Settings) error {
		current := PrefsOf(store.User{Settings: mustJSON(set)})
		for k, c := range p.Kinds {
			current.Kinds[k] = c
		}
		current.Digest = p.Digest
		if weekly != nil {
			current.Weekly = *weekly
		}
		saved = current.normalized()
		raw, err := json.Marshal(saved)
		set["notifications"] = raw
		return err
	})
	return saved, u, err
}

func known(k Kind) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
