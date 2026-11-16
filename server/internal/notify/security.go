package notify

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/mail"
	"github.com/locodarwin/mila/server/internal/store"
)

// NewDeviceSignIn emails someone that their account was signed in to
// from a browser it had not used before. It goes out whatever their
// notification settings, as security mail does, through the mail lane so
// it is retried; with no email set up there is no one to tell.
func (s *Service) NewDeviceSignIn(ctx context.Context, user store.User, ev accounts.SecurityEvent) {
	if !s.email {
		return
	}
	local := ev.CreatedAt.In(accounts.Location(user))
	style := accounts.DateStyleOf(user)
	when := fmt.Sprintf("%s at %s (%s)", style.Day(local, true, local), style.Time(local.Format("15:04")), accounts.Timezone(user))
	device := DeviceName(ev.UserAgent)
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\nYour Mila account was signed in to from a browser it has not used before:\n\n", user.Name)
	fmt.Fprintf(&b, "  %s\n  %s\n", device, when)
	if ev.Ip != "" {
		fmt.Fprintf(&b, "  from the address %s\n", ev.Ip)
	}
	b.WriteString("\nIf this was you, there is nothing to do.\n\n")
	b.WriteString("If it was not, open Settings, Security in Mila and sign that device out, then remove any passkey you do not recognize. Anyone who can read your email can sign in by email link, so check that account too.\n\n")
	b.WriteString(s.baseURL + "/settings/security\n")
	err := QueueMail(ctx, s.st, mail.Message{
		To: user.Email, Subject: "Mila: new sign-in on " + device, Text: b.String(),
	}, s.now())
	if err != nil {
		s.log.Warn("notify: new device", "err", err)
		return
	}
	s.jobs.Wake()
}

// PasskeyAdded tells someone a passkey was added to their account,
// whatever their notification settings, through the mail lane: anyone
// who adds one can sign in with it for good.
func (s *Service) PasskeyAdded(ctx context.Context, user store.User, ev accounts.SecurityEvent) {
	if !s.email {
		return
	}
	local := ev.CreatedAt.In(accounts.Location(user))
	style := accounts.DateStyleOf(user)
	when := fmt.Sprintf("%s at %s (%s)", style.Day(local, true, local), style.Time(local.Format("15:04")), accounts.Timezone(user))
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\nA passkey named \"%s\" was added to your Mila account on %s", user.Name, ev.Detail, when)
	if ev.UserAgent != "" {
		fmt.Fprintf(&b, ", from %s", DeviceName(ev.UserAgent))
	}
	b.WriteString(".\n\nIf this was you, there is nothing to do.\n\n")
	b.WriteString("If it was not, remove that passkey in Settings, Security in Mila, and sign out the devices you do not know.\n\n")
	b.WriteString(s.baseURL + "/settings/security\n")
	err := QueueMail(ctx, s.st, mail.Message{
		To: user.Email, Subject: "Mila: a passkey was added to your account", Text: b.String(),
	}, s.now())
	if err != nil {
		s.log.Warn("notify: passkey added", "err", err)
		return
	}
	s.jobs.Wake()
}

// EmailChanged tells the address an account had that the account now
// uses another, whatever their notification settings, through the mail
// lane. Whoever changed it had to open a link sent to the new address, so
// the old one is the place to hear of a change the owner did not make.
func (s *Service) EmailChanged(ctx context.Context, user store.User, oldEmail string, ev accounts.SecurityEvent) {
	if !s.email {
		return
	}
	local := ev.CreatedAt.In(accounts.Location(user))
	style := accounts.DateStyleOf(user)
	when := fmt.Sprintf("%s at %s (%s)", style.Day(local, true, local), style.Time(local.Format("15:04")), accounts.Timezone(user))
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\nThe email address of your Mila account changed from %s to %s on %s.\n\n", user.Name, oldEmail, user.Email, when)
	b.WriteString("Sign-in links and email from Mila go to the new address from now on.\n\n")
	b.WriteString("If you did not make this change, someone else may be signed in to your account. On a device where you are still signed in, change the address back in Settings, Profile, and sign out the devices you do not know in Settings, Security. If you are signed out everywhere, ask your Mila admin for help.\n\n")
	b.WriteString(s.baseURL + "/settings/security\n")
	err := QueueMail(ctx, s.st, mail.Message{
		To: oldEmail, Subject: "Mila: your email address changed", Text: b.String(),
	}, s.now())
	if err != nil {
		s.log.Warn("notify: email changed", "err", err)
		return
	}
	s.jobs.Wake()
}

var (
	browsers = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`Edg/`), "Edge"},
		{regexp.MustCompile(`OPR/`), "Opera"},
		{regexp.MustCompile(`Firefox/`), "Firefox"},
		{regexp.MustCompile(`Chrome/`), "Chrome"},
		{regexp.MustCompile(`Safari/`), "Safari"},
	}
	systems = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`iPhone`), "iPhone"},
		{regexp.MustCompile(`iPad`), "iPad"},
		{regexp.MustCompile(`Android`), "Android"},
		{regexp.MustCompile(`CrOS`), "ChromeOS"},
		{regexp.MustCompile(`Mac OS X|Macintosh`), "macOS"},
		{regexp.MustCompile(`Windows`), "Windows"},
		{regexp.MustCompile(`Linux`), "Linux"},
	}
)

// DeviceName is a short name for a browser from its user agent, such as
// "Firefox on Linux": deviceName in web/src/lib/device.ts, which names
// the same browsers in the app.
func DeviceName(userAgent string) string {
	var browser, system string
	for _, b := range browsers {
		if b.re.MatchString(userAgent) {
			browser = b.name
			break
		}
	}
	for _, sys := range systems {
		if sys.re.MatchString(userAgent) {
			system = sys.name
			break
		}
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "an unknown device"
}
