package accounts

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"time"

	"github.com/locodarwin/mila/server/internal/store"
)

// Preferences are how a user likes Mila to look and behave, stored in
// their settings under "preferences" and sent with the user, so every
// device they sign in on follows them. The web app keeps a copy for the
// moment before it has heard from the server.
type Preferences struct {
	Theme              string   `json:"theme"`
	Scheme             string   `json:"scheme"`
	AccentHue          *int     `json:"accentHue"`
	Density            string   `json:"density"`
	FontSize           string   `json:"fontSize"`
	WeekStart          string   `json:"weekStart"`
	DateFormat         string   `json:"dateFormat"`
	Clock              string   `json:"clock"`
	ShowCompleted      bool     `json:"showCompleted"`
	StartPage          string   `json:"startPage"`
	SidebarOrder       []string `json:"sidebarOrder"`
	SidebarHidden      []string `json:"sidebarHidden"`
	SingleKeyShortcuts bool     `json:"singleKeyShortcuts"`
	// Shortcuts holds the keys the person changed, by command id; the
	// web app knows the commands and their defaults, and a command left
	// out keeps its default keys.
	Shortcuts map[string][]string `json:"shortcuts"`
	// CustomTheme names one of the person's own themes (user_themes), or
	// a theme one of their workspaces offers (workspace_themes), or is
	// null for the built-in Theme alone. Only its shape is checked:
	// the theme may be saved after this, or deleted, and the app then
	// shows Theme, the built-in it was based on.
	CustomTheme *string `json:"customTheme"`
	// ThemeAnswers are the suggested workspace themes the person answered
	// (Use it, or No thanks), oldest first, so each is asked about once,
	// on every device.
	ThemeAnswers []string `json:"themeAnswers"`
	// TaskPanel is how the task panel lays out its sections: all showing,
	// in boxes, or as summary lines that open in place.
	TaskPanel string `json:"taskPanel"`
}

// The choices each preference allows, its default first.
var (
	themes      = []string{"standard", "calm", "playful", "contrast"}
	schemes     = []string{"system", "light", "dark"}
	densities   = []string{"comfortable", "dense"}
	fontSizes   = []string{"m", "s", "l", "xl", "xxl"}
	weekStarts  = []string{"auto", "sunday", "monday", "saturday"}
	dateFormats = []string{"auto", "month-day", "day-month", "iso"}
	clocks      = []string{"auto", "12", "24"}
	startPages  = []string{"inbox", "today", "upcoming", "projects"}
	taskPanels  = []string{"boxes", "summary"}
	// SidebarItems are the sidebar's parts that can be moved or hidden,
	// in their first order.
	SidebarItems = []string{"inbox", "today", "upcoming", "wiki", "time", "reports", "views", "projects"}
)

// DefaultPreferences are the preferences of someone who never changed one.
func DefaultPreferences() Preferences {
	return Preferences{
		Theme: themes[0], Scheme: schemes[0], Density: densities[0], FontSize: fontSizes[0],
		WeekStart: weekStarts[0], DateFormat: dateFormats[0], Clock: clocks[0],
		StartPage: startPages[0], SidebarOrder: slices.Clone(SidebarItems), SidebarHidden: []string{},
		SingleKeyShortcuts: true, Shortcuts: map[string][]string{}, ThemeAnswers: []string{},
		TaskPanel: taskPanels[0],
	}
}

// PreferencesOf reads a user's preferences: the defaults, with each stored
// value that is still valid on top.
func PreferencesOf(u store.User) Preferences {
	p := DefaultPreferences()
	var stored map[string]json.RawMessage
	if json.Unmarshal(SettingsOf(u)["preferences"], &stored) != nil {
		return p
	}
	for key, raw := range stored {
		_ = p.set(key, raw) // a value that no longer fits keeps the default
	}
	return p
}

// set changes one preference from its JSON, if the value is allowed.
func (p *Preferences) set(key string, raw json.RawMessage) error {
	oneOf := func(dst *string, allowed []string) error {
		var v string
		if json.Unmarshal(raw, &v) != nil || !slices.Contains(allowed, v) {
			return fail(Invalid, "%s must be one of %v.", key, allowed)
		}
		*dst = v
		return nil
	}
	flag := func(dst *bool) error {
		if json.Unmarshal(raw, dst) != nil {
			return fail(Invalid, "%s must be true or false.", key)
		}
		return nil
	}
	switch key {
	case "theme":
		return oneOf(&p.Theme, themes)
	case "scheme":
		return oneOf(&p.Scheme, schemes)
	case "density":
		return oneOf(&p.Density, densities)
	case "fontSize":
		return oneOf(&p.FontSize, fontSizes)
	case "weekStart":
		return oneOf(&p.WeekStart, weekStarts)
	case "dateFormat":
		return oneOf(&p.DateFormat, dateFormats)
	case "clock":
		return oneOf(&p.Clock, clocks)
	case "startPage":
		return oneOf(&p.StartPage, startPages)
	case "taskPanel":
		return oneOf(&p.TaskPanel, taskPanels)
	case "showCompleted":
		return flag(&p.ShowCompleted)
	case "singleKeyShortcuts":
		return flag(&p.SingleKeyShortcuts)
	case "accentHue":
		var hue *int
		if json.Unmarshal(raw, &hue) != nil || (hue != nil && (*hue < 0 || *hue > 359)) {
			return fail(Invalid, "accentHue must be a whole number from 0 to 359, or null.")
		}
		p.AccentHue = hue
		return nil
	case "customTheme":
		var id *string
		if json.Unmarshal(raw, &id) != nil || (id != nil && !themeIDPattern.MatchString(*id)) {
			return fail(Invalid, "customTheme must be a theme id, or null.")
		}
		p.CustomTheme = id
		return nil
	case "themeAnswers":
		var ids []string
		if json.Unmarshal(raw, &ids) != nil || len(ids) > maxThemeAnswers || slices.ContainsFunc(ids, func(id string) bool { return !themeIDPattern.MatchString(id) }) {
			return fail(Invalid, "themeAnswers must list up to %d theme ids.", maxThemeAnswers)
		}
		if ids == nil {
			ids = []string{}
		}
		p.ThemeAnswers = ids
		return nil
	case "sidebarOrder":
		var order []string
		if json.Unmarshal(raw, &order) != nil || !sidebarItems(order) {
			return fail(Invalid, "sidebarOrder must list sidebar items: %v.", SidebarItems)
		}
		p.SidebarOrder = completeOrder(order)
		return nil
	case "shortcuts":
		var shortcuts map[string][]string
		if json.Unmarshal(raw, &shortcuts) != nil || !validShortcuts(shortcuts) {
			return fail(Invalid, "shortcuts must map up to %d command ids to up to %d different key bindings each.", maxShortcuts, maxBindings)
		}
		// Null becomes empty, which the app expects.
		if shortcuts == nil {
			shortcuts = map[string][]string{}
		}
		for id, keys := range shortcuts {
			if keys == nil {
				shortcuts[id] = []string{}
			}
		}
		p.Shortcuts = shortcuts
		return nil
	case "sidebarHidden":
		var hidden []string
		if json.Unmarshal(raw, &hidden) != nil || !sidebarItems(hidden) {
			return fail(Invalid, "sidebarHidden must list sidebar items: %v.", SidebarItems)
		}
		if hidden == nil {
			hidden = []string{}
		}
		p.SidebarHidden = hidden
		return nil
	}
	return fail(Invalid, "%q is not a preference.", key)
}

// maxThemeAnswers bounds the suggestions a person's answers are kept for.
const maxThemeAnswers = 100

// How many commands may have their keys changed, and how many keys each
// may have.
const (
	maxShortcuts = 200
	maxBindings  = 4
)

var (
	// shortcutID is a command id as web/src/lib/keys/commands.ts writes them.
	shortcutID = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,47}$`)
	// binding is a chord ([Mod+][Alt+][Shift+]KEY) or two plain keys in
	// turn (KEY KEY), where KEY is one character or a key name such as
	// ArrowUp. web/src/lib/keys/chord.ts has the same grammar, and
	// shared/keybinding-vectors.json holds the two to it.
	binding = regexp.MustCompile(`^(?:(?:Mod\+)?(?:Alt\+)?(?:Shift\+)?` + bindingKey + `|` + bindingKey + ` ` + bindingKey + `)$`)
)

// bindingKey is one key: a character other than ASCII white space, or a
// key name.
const bindingKey = `(?:[^\t\n\v\f\r ]|[A-Z][A-Za-z0-9]{1,11})`

// validBinding reports whether a key binding is written as the app writes them.
func validBinding(b string) bool {
	return binding.MatchString(b)
}

// validShortcuts reports whether a shortcuts map is within its limits:
// known-looking ids, and a few different, well-formed bindings for each.
// Which commands exist and whether keys clash is the app's business.
func validShortcuts(m map[string][]string) bool {
	if len(m) > maxShortcuts {
		return false
	}
	for id, keys := range m {
		if !shortcutID.MatchString(id) || len(keys) > maxBindings {
			return false
		}
		for i, b := range keys {
			if !validBinding(b) || slices.Contains(keys[:i], b) {
				return false
			}
		}
	}
	return true
}

// completeOrder adds the items a saved order leaves out, such as one added
// since it was saved: each just after the item before it in the first
// order, or first.
func completeOrder(order []string) []string {
	for i, item := range SidebarItems {
		if slices.Contains(order, item) {
			continue
		}
		at := 0
		for j := i - 1; j >= 0; j-- {
			if k := slices.Index(order, SidebarItems[j]); k >= 0 {
				at = k + 1
				break
			}
		}
		order = slices.Insert(order, at, item)
	}
	return order
}

// sidebarItems reports whether a list names sidebar items, each once.
func sidebarItems(list []string) bool {
	for i, item := range list {
		if !slices.Contains(SidebarItems, item) || slices.Contains(list[:i], item) {
			return false
		}
	}
	return true
}

// SavePreferences changes the preferences named in the patch and leaves
// the rest alone, so two devices changing different ones offline both
// keep their change. Nothing is saved if any value is not allowed.
func (s *Service) SavePreferences(ctx context.Context, userID string, patch map[string]json.RawMessage) (Preferences, store.User, error) {
	var saved Preferences
	u, err := UpdateSettings(ctx, s.st, userID, func(set Settings) error {
		stored := map[string]json.RawMessage{}
		_ = json.Unmarshal(set["preferences"], &stored)
		saved = PreferencesOf(store.User{Settings: mustJSON(set)})
		for key, raw := range patch {
			if err := saved.set(key, raw); err != nil {
				return err
			}
			stored[key] = raw
		}
		raw, err := json.Marshal(stored)
		set["preferences"] = raw
		return err
	})
	if err != nil {
		return Preferences{}, store.User{}, err
	}
	s.events.SettingsChanged(userID)
	return saved, u, nil
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// DateStyle is how a user reads dates and times, for text the server
// writes such as emails and notifications. "auto" preferences follow what
// the user's device last reported (see SetTimezone); before any report,
// times are 24-hour and the month comes first.
type DateStyle struct {
	// Hour12 writes times as "2:30 PM" rather than "14:30".
	Hour12 bool
	// Order is "month-day", "day-month" or "iso".
	Order string
}

// DateStyleOf works out a user's DateStyle.
func DateStyleOf(u store.User) DateStyle {
	p := PreferencesOf(u)
	var device Locale
	_ = json.Unmarshal(SettingsOf(u)["locale"], &device)
	style := DateStyle{Order: "month-day"}
	switch {
	case p.Clock != "auto":
		style.Hour12 = p.Clock == "12"
	case device.Clock != "":
		style.Hour12 = device.Clock == "12"
	}
	switch {
	case p.DateFormat != "auto":
		style.Order = p.DateFormat
	case slices.Contains(dateOrders, device.DateOrder):
		style.Order = device.DateOrder
	}
	return style
}

var dateOrders = []string{"month-day", "day-month", "iso"}

// Time writes a clock time given as "15:04" the user's way; anything else
// comes back as it was.
func (d DateStyle) Time(clock string) string {
	t, err := time.Parse("15:04", clock)
	if err != nil || !d.Hour12 {
		return clock
	}
	return t.Format("3:04 PM")
}

// Day writes a date the user's way, with a short weekday if asked, and
// the year when it is not the year of now: "Mon, Nov 2", "Mon, 2 Nov",
// "2026-11-02".
func (d DateStyle) Day(day time.Time, weekday bool, now time.Time) string {
	if d.Order == "iso" {
		return day.Format(time.DateOnly)
	}
	layout := "Jan 2"
	if d.Order == "day-month" {
		layout = "2 Jan"
	}
	if day.Year() != now.Year() {
		if d.Order == "day-month" {
			layout += " 2006"
		} else {
			layout += ", 2006"
		}
	}
	if weekday {
		layout = "Mon, " + layout
	}
	return day.Format(layout)
}

// LongDay writes a date in full, for headings: "Monday, November 2", or
// "Monday 2 November".
func (d DateStyle) LongDay(day time.Time) string {
	switch d.Order {
	case "iso":
		return day.Format("Monday, 2006-01-02")
	case "day-month":
		return day.Format("Monday 2 January")
	}
	return day.Format("Monday, January 2")
}

// Locale is how the user's device writes dates and times, which is what
// "auto" preferences mean in text the server writes: Clock "12" or "24",
// DateOrder one of dateOrders, and WeekStart its first day of the week (0
// for Sunday to 6 for Saturday). Any part may be missing.
type Locale struct {
	Clock     string `json:"clock"`
	DateOrder string `json:"dateOrder"`
	WeekStart *int   `json:"weekStart,omitempty"`
}

// setLocale records what "auto" means on the user's device. A report
// without a week start keeps the one reported before (an app from before
// it reports none).
func setLocale(set Settings, l Locale) error {
	if l.Clock == "" && l.DateOrder == "" && l.WeekStart == nil {
		return nil
	}
	if (l.Clock != "" && l.Clock != "12" && l.Clock != "24") || (l.DateOrder != "" && !slices.Contains(dateOrders, l.DateOrder)) {
		return fail(Invalid, "clock must be 12 or 24 and dateOrder one of %v.", dateOrders)
	}
	if l.WeekStart != nil && (*l.WeekStart < 0 || *l.WeekStart > 6) {
		return fail(Invalid, "weekStart must be from 0 (Sunday) to 6 (Saturday).")
	}
	if l.WeekStart == nil {
		var before Locale
		_ = json.Unmarshal(set["locale"], &before)
		l.WeekStart = before.WeekStart
	}
	raw, err := json.Marshal(l)
	set["locale"] = raw
	return err
}

// WeekStartOf is the first day of a user's week, as their preference says,
// or for "auto" as their device last reported; before any report, Monday,
// as the app guesses when its device can't say.
func WeekStartOf(u store.User) time.Weekday {
	switch PreferencesOf(u).WeekStart {
	case "sunday":
		return time.Sunday
	case "monday":
		return time.Monday
	case "saturday":
		return time.Saturday
	}
	var device Locale
	if err := json.Unmarshal(SettingsOf(u)["locale"], &device); err == nil && device.WeekStart != nil && *device.WeekStart >= 0 && *device.WeekStart <= 6 {
		return time.Weekday(*device.WeekStart)
	}
	return time.Monday
}
