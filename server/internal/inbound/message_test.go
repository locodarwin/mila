package inbound

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, raw string) *Message {
	t.Helper()
	m, err := Parse(strings.NewReader(strings.ReplaceAll(raw, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestReadingAPlainMessage(t *testing.T) {
	m := parse(t, `From: Dana Reyes <Dana@Example.COM>
To: mila+abc@example.com
Subject: =?UTF-8?B?Q2Fmw6kgb3JkZXI=?=
Message-ID: <one@example.com>
Date: Tue, 3 Nov 2026 09:12:00 -0800
Content-Type: text/plain; charset=iso-8859-1
Content-Transfer-Encoding: quoted-printable

Caf=E9 on Tuesday.
`)
	if m.From != "dana@example.com" || m.Subject != "Café order" || m.MessageID != "one@example.com" {
		t.Fatalf("%+v", m)
	}
	if strings.TrimSpace(m.Text) != "Café on Tuesday." {
		t.Fatalf("text %q", m.Text)
	}
}

func TestAMessageOfOnlyHTMLIsReadAsText(t *testing.T) {
	m := parse(t, `From: a@example.com
Content-Type: text/html; charset=utf-8

<html><head><title>No</title><style>p{color:red}</style></head>
<body><p>First <b>line</b>.</p><script>alert(1)</script>
<ul><li>One</li><li>Two</li></ul><a href="https://example.com/x">See this</a><a href="javascript:alert(1)">Not this</a></body></html>
`)
	for _, want := range []string{"First line .", "- One", "- Two", "See this (https://example.com/x)", "Not this"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("no %q in %q", want, m.Text)
		}
	}
	for _, not := range []string{"alert(1)", "color:red", "<", "No\n", "javascript"} {
		if strings.Contains(m.Text, not) {
			t.Errorf("%q in %q", not, m.Text)
		}
	}
}

func TestPlainTextIsPreferredAndPartsAreSorted(t *testing.T) {
	small := base64.StdEncoding.EncodeToString(make([]byte, 2000))
	big := base64.StdEncoding.EncodeToString(make([]byte, signatureImage+1))
	m := parse(t, `From: a@example.com
Content-Type: multipart/mixed; boundary=b

--b
Content-Type: multipart/alternative; boundary=c

--c
Content-Type: text/plain

The words.
--c
Content-Type: text/html

<p>Not these.</p>
--c--
--b
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

`+small+`
--b
Content-Type: image/jpeg; name=IMG_1.jpg
Content-Disposition: inline; filename=IMG_1.jpg
Content-Transfer-Encoding: base64

`+big+`
--b
Content-Type: application/pdf
Content-Disposition: attachment; filename="../../etc/invoice.pdf"
Content-Transfer-Encoding: base64

JVBERi0=
--b--
`)
	if strings.TrimSpace(m.Text) != "The words." {
		t.Fatalf("text %q", m.Text)
	}
	var names []string
	for _, a := range m.Attachments {
		names = append(names, a.Name)
	}
	// The small picture in the text is a signature's; the large one is a photo.
	if got := strings.Join(names, ","); got != "IMG_1.jpg,../../etc/invoice.pdf" {
		t.Fatalf("attachments %s", got)
	}
}

func TestAFileTooLargeIsNamedAndNotKept(t *testing.T) {
	big := base64.StdEncoding.EncodeToString(make([]byte, maxFile+10))
	m := parse(t, `From: a@example.com
Content-Type: multipart/mixed; boundary=b

--b
Content-Type: text/plain

Hello.
--b
Content-Type: video/mp4
Content-Disposition: attachment; filename=walk.mov
Content-Transfer-Encoding: base64

`+big+`
--b--
`)
	if len(m.Attachments) != 1 || m.Attachments[0].Data != nil || m.Attachments[0].Size != maxFile+10 {
		t.Fatalf("%d attachments, size %d", len(m.Attachments), m.Attachments[0].Size)
	}
}

func TestNULsAndBadBytesCannotReachTheDatabase(t *testing.T) {
	m := parse(t, "From: a@example.com\nSubject: a\x00b\xff\nContent-Type: text/plain\n\nc\x00d\xff\n")
	for _, s := range []string{m.Subject, m.Text} {
		if strings.Contains(s, "\x00") || !strings.Contains(s, "�") && strings.Contains(s, "\xff") {
			t.Fatalf("%q", s)
		}
	}
}

func TestTwoSendersIsNoSender(t *testing.T) {
	if m := parse(t, "From: a@example.com, b@example.com\n\nHello.\n"); m.From != "" {
		t.Fatalf("from %q", m.From)
	}
}

func TestWhatIsTakenForAMachine(t *testing.T) {
	cases := map[string]bool{
		"Auto-Submitted: auto-replied": true,
		"Auto-Submitted: no":           false,
		"Precedence: bulk":             true,
		"List-Id: <news.example.com>":  true,
		"List-Unsubscribe: <mailto:x>": true,
		"X-Autoreply: yes":             true,
		"X-Other: yes":                 false,
	}
	for header, want := range cases {
		if got := parse(t, "From: a@example.com\n"+header+"\n\nHi\n").Automatic(); got != want {
			t.Errorf("%s: %v", header, got)
		}
	}
}

func TestWhatIsTakenForSpam(t *testing.T) {
	cases := map[string]bool{
		"X-Spam-Flag: YES":             true,
		"X-Spam-Status: Yes, score=9":  true,
		"X-Spam-Status: No, score=0.1": false,
	}
	for header, want := range cases {
		if got := parse(t, "From: a@example.com\n"+header+"\n\nHi\n").Spam(); got != want {
			t.Errorf("%s: %v", header, got)
		}
	}
}

func TestSenderChecksTrustOnlyTheReceivingServer(t *testing.T) {
	cases := []struct {
		name, headers string
		want          bool
	}{
		{"none", "", false},
		{"pass", "Authentication-Results: mx.example; spf=pass; dkim=pass; dmarc=pass\n", false},
		{"dmarc failed", "Authentication-Results: mx.example; dmarc=fail\n", true},
		{"spf failed alone", "Authentication-Results: mx.example; spf=fail\n", true},
		// A forward fails SPF and keeps its signature.
		{"spf failed, signed", "Authentication-Results: mx.example; spf=fail; dkim=pass header.d=example.com\n", false},
		{"spf failed, signed by a subdomain", "Authentication-Results: mx.example; spf=fail; dkim=pass header.d=mail.example.com\n", false},
		// Anyone can sign with a domain of their own.
		{"spf failed, signed by someone else", "Authentication-Results: mx.example; spf=fail; dkim=pass header.d=evil.example\n", true},
		{"spf failed, a signature naming no domain", "Authentication-Results: mx.example; spf=fail; dkim=pass\n", true},
		{"spf softfailed alone", "Authentication-Results: mx.example; spf=softfail; dmarc=none\n", true},
		{"dmarc passed", "Authentication-Results: mx.example; spf=softfail; dmarc=pass\n", false},
		// The receiving server's comes first, whatever the sender added below it.
		{"a sender's claim below", "Authentication-Results: mx.example; dmarc=fail\nAuthentication-Results: me; dmarc=pass\n", true},
	}
	for _, c := range cases {
		if got := parse(t, "From: a@example.com\n"+c.headers+"\nHi\n").Forged(); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

const token = "abcdefghijklmnopqrstuvwxyz"

func TestFindingAnAddress(t *testing.T) {
	plus := AddressPattern("mila+{token}@example.com")
	catchAll := AddressPattern("{token}@in.example.com")
	cases := []struct {
		name    string
		pattern *regexp.Regexp
		headers string
		want    string
	}{
		{"to", plus, "To: Mila <mila+" + token + "@example.com>\n", token},
		{"bare", plus, "To: mila+" + token + "@example.com\n", token},
		{"among others", plus, "To: x@example.org, \"M\" <mila+" + token + "@example.com>, y@example.org\n", token},
		{"cc", plus, "To: x@example.org\nCc: mila+" + token + "@example.com\n", token},
		{"upper case", plus, "To: MILA+" + strings.ToUpper(token) + "@EXAMPLE.COM\n", token},
		{"bcc, by a server's header", plus, "To: x@example.org\nDelivered-To: mila+" + token + "@example.com\n", token},
		{"received", plus, "To: x@example.org\nReceived: from a by b for <mila+" + token + "@example.com>; Tue\n", token},
		{"catch-all", catchAll, "To: " + token + "@in.example.com\n", token},
		{"another domain", plus, "To: mila+" + token + "@example.org\n", ""},
		{"too short", plus, "To: mila+abc@example.com\n", ""},
		{"too long", plus, "To: mila+" + token + "a@example.com\n", ""},
		{"not a token's letters", plus, "To: mila+" + strings.Repeat("1", 26) + "@example.com\n", ""},
		{"nothing of ours", plus, "To: x@example.org\n", ""},
	}
	for _, c := range cases {
		m := parse(t, "From: a@example.com\n"+c.headers+"\nHi\n")
		if got := m.RecipientsMatching(c.pattern); got != c.want {
			t.Errorf("%s: %q", c.name, got)
		}
	}
	// The message itself counts for nothing: quoting an address isn't sending to it.
	m := parse(t, "From: a@example.com\nTo: x@example.org\n\nmila+"+token+"@example.com\n")
	if got := m.RecipientsMatching(plus); got != "" {
		t.Errorf("found in the body: %q", got)
	}
}

func TestATasksTitle(t *testing.T) {
	cases := []struct{ subject, text, want string }{
		{"Fwd: Re: FW:  Invoice 4521", "", "Invoice 4521"},
		{"Re:Fwd:Plan", "", "Plan"},
		{"Regarding the roast", "", "Regarding the roast"},
		{"  line\tone  ", "", "line one"},
		{"", "\n\n  Crack in the seal\nSent from my phone", "Crack in the seal"},
		{"Fwd:", "Body", "Body"},
		{"", "", "Email from a@example.com"},
	}
	for _, c := range cases {
		if got := titleOf(&Message{Subject: c.subject, Text: c.text, From: "a@example.com"}, 500); got != c.want {
			t.Errorf("%q: %q", c.subject, got)
		}
	}
	if got := titleOf(&Message{Subject: strings.Repeat("é", 600)}, 500); len([]rune(got)) != 500 {
		t.Errorf("%d", len([]rune(got)))
	}
}

func TestSenderChecksReadOnlyWhereACheckBegins(t *testing.T) {
	cases := []struct {
		name, results string
		want          bool
	}{
		{"a sender's words in quotes", `mx; spf=fail smtp.mailfrom="dkim=pass"@evil.example`, true},
		{"a sender's words after a semicolon in quotes", `mx; spf=fail smtp.mailfrom="x;dkim=pass"@evil.example`, true},
		{"in a comment", `mx; spf=fail (dkim=pass); dmarc=none`, true},
		{"a real signature", `mx; spf=fail smtp.mailfrom=a@example.com; dkim=pass header.d=example.com`, false},
		{"a signature's domain in quotes", `mx; spf=fail; dkim=pass header.i="x header.d=example.com" header.d=evil.example`, true},
		{"upper case", `mx; DMARC=FAIL`, true},
		{"the first result of a check counts", `mx; dmarc=fail; dmarc=pass`, true},
	}
	for _, c := range cases {
		if got := parse(t, "From: a@example.com\nAuthentication-Results: "+c.results+"\n\nHi\n").Forged(); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestTwoFromHeadersIsNoSender(t *testing.T) {
	if m := parse(t, "From: listed@example.com\nFrom: other@example.org\n\nHi\n"); m.From != "" {
		t.Fatalf("from %q", m.From)
	}
}

// Bcc: To names someone else's address; a server's own header says whose this is.
func TestWhatServersSayComesBeforeWhatASenderWrote(t *testing.T) {
	plus := AddressPattern("mila+{token}@example.com")
	other := strings.Repeat("b", 26)
	m := parse(t, "From: a@example.com\nTo: mila+"+other+"@example.com\nDelivered-To: mila+"+token+"@example.com\n\nHi\n")
	if got := m.RecipientsMatching(plus); got != token {
		t.Fatalf("%q", got)
	}
}

func TestTextHasALimitAcrossAllItsParts(t *testing.T) {
	var b strings.Builder
	b.WriteString("From: a@example.com\nContent-Type: multipart/mixed; boundary=x\n\n")
	line := strings.Repeat("<br>", 200_000)
	for range 20 {
		b.WriteString("--x\nContent-Type: text/html\n\n" + line + "\n")
	}
	b.WriteString("--x--\n")
	started := time.Now()
	m := parse(t, b.String())
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("took %v", took)
	}
	if len(m.Text) > maxHTMLRead {
		t.Fatalf("%d bytes of text", len(m.Text))
	}
}

func TestALongSubjectIsCutBeforeItIsTidied(t *testing.T) {
	m := parse(t, "From: a@example.com\nSubject: "+strings.Repeat("Re:", 300_000)+"Hello\n\nHi\n")
	started := time.Now()
	_ = titleOf(m, 500)
	if took := time.Since(started); took > time.Second {
		t.Fatalf("took %v", took)
	}
}
