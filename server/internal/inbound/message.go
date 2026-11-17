// Package inbound is email to task: reading a mailbox,
// telling which of its mail is addressed to someone's private address, and making
// tasks of what they accept. Nothing here replies to anyone.
package inbound

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	// Mail arrives in every character set there is.
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

const (
	// maxMessage is the largest message read: two attachments at their limit,
	// which base64 makes a third larger.
	maxMessage = 64 << 20
	// maxFile is the largest attachment kept, as for one uploaded.
	maxFile = 25 << 20
	// signatureImage is the size under which a picture in the text of a message
	// is taken for part of a signature.
	signatureImage = 100 << 10
	// maxPlainRead and maxHTMLRead bound the text read of one message, all its
	// parts together. HTML is less because it costs far more to parse.
	maxPlainRead = 4 << 20
	maxHTMLRead  = 2 << 20
	// maxHeaderRead is how much of a message's headers is read to decide about it.
	maxHeaderRead = 256 << 10
	// maxSubjectRead keeps the work of tidying a subject in proportion.
	maxSubjectRead = 4 << 10
	// maxFiles is how many files of one message are kept.
	maxFiles = 20
	// A message is refused before any parsing for more structure than mail has:
	// the parser has no limit of its own on how deep parts nest or how many there
	// are, and each level costs it memory and every read a pass through them all.
	maxBoundaries    = 32
	maxBoundaryLines = 5000
	maxLines         = 2 << 20
)

// ErrTooComplex says a message has more structure than an email should.
var ErrTooComplex = errors.New("too complex to read")

// checkStructure looks over a whole message for what would make parsing it costly.
func checkStructure(raw []byte) error {
	if len(raw) > maxMessage {
		return ErrTooComplex
	}
	if bytes.Count(raw, []byte("\n")) > maxLines {
		return ErrTooComplex
	}
	if countFold(raw, "boundary") > maxBoundaries {
		return ErrTooComplex
	}
	if bytes.Count(raw, []byte("\n--")) > maxBoundaryLines {
		return ErrTooComplex
	}
	return nil
}

// countFold counts a lowercase word in any case, without copying a large message.
func countFold(raw []byte, word string) int {
	n := 0
	for i := 0; i+len(word) <= len(raw); i++ {
		if raw[i]|0x20 == word[0] && bytes.EqualFold(raw[i:i+len(word)], []byte(word)) {
			n++
			i += len(word) - 1
		}
	}
	return n
}

// Attachment is a file in a message.
type Attachment struct {
	Name, Type string
	Data       []byte
	// Size is its length when too large to keep, with no Data.
	Size int64
}

// Message is what is needed of an email.
type Message struct {
	// From is the sender's one address, lowercased; empty if it has none or many.
	From      string
	MessageID string
	Subject   string
	Date      time.Time
	// Text is the message as plain text, from its HTML if it has nothing else.
	Text        string
	Attachments []Attachment
	// MoreFiles is how many files were past the number kept.
	MoreFiles int
	// Size is the whole message's length, as far as known.
	Size int64
	// TooLarge says the message is not to be read.
	TooLarge bool

	header mail.Header
}

// clean makes text safe to store: valid UTF-8 with no NUL, which Postgres refuses.
func clean(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "")
}

// Parse reads an email. A part it cannot read is left out rather than losing the rest.
func Parse(r io.Reader) (*Message, error) {
	mr, err := mail.CreateReader(r)
	if err != nil && !message.IsUnknownCharset(err) {
		return nil, err
	}
	if mr == nil {
		return nil, errors.New("not an email")
	}
	defer func() { _ = mr.Close() }()
	m := &Message{header: mr.Header}
	// One From header holding one address. With more, which one a provider checked
	// is anyone's guess.
	froms := mr.Header.FieldsByKey("From")
	count := 0
	for froms.Next() {
		count++
	}
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) == 1 && count == 1 {
		m.From = strings.ToLower(clean(strings.TrimSpace(from[0].Address)))
	}
	m.MessageID, _ = mr.Header.MessageID()
	m.MessageID = clean(m.MessageID)
	if s, err := mr.Header.Subject(); err == nil || message.IsUnknownCharset(err) {
		if len(s) > maxSubjectRead {
			s = s[:maxSubjectRead]
		}
		m.Subject = clean(s)
	}
	if d, err := mr.Header.Date(); err == nil {
		m.Date = d
	}
	var plain, rich strings.Builder
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if message.IsUnknownCharset(err) && part != nil {
				continue
			}
			break // What came before it is still worth having.
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			kind, _, _ := h.ContentType()
			name := ""
			if _, params, err := h.ContentDisposition(); err == nil {
				name = params["filename"]
			}
			if name == "" {
				_, params, _ := h.ContentType()
				name = params["name"]
			}
			switch {
			case kind == "text/plain" && name == "":
				readText(&plain, part.Body, maxPlainRead)
			case kind == "text/html" && name == "":
				readText(&rich, part.Body, maxHTMLRead)
			case strings.HasPrefix(kind, "image/"):
				if a, ok := readFile(name, kind, part.Body); ok && (a.Size > 0 || len(a.Data) > signatureImage) {
					m.keep(a)
				}
			default:
				if a, ok := readFile(name, kind, part.Body); ok {
					m.keep(a)
				}
			}
		case *mail.AttachmentHeader:
			kind, _, _ := h.ContentType()
			name, _ := h.Filename()
			if a, ok := readFile(name, kind, part.Body); ok {
				m.keep(a)
			}
		}
	}
	m.Text = clean(plain.String())
	if strings.TrimSpace(m.Text) == "" {
		m.Text = clean(htmlToText(rich.String()))
	}
	return m, nil
}

// keep holds on to a file, up to the number kept.
func (m *Message) keep(a Attachment) {
	if len(m.Attachments) == maxFiles {
		m.MoreFiles++
		return
	}
	m.Attachments = append(m.Attachments, a)
}

// readText adds a part's text, up to limit for all of them together.
func readText(to *strings.Builder, r io.Reader, limit int64) {
	room := limit - int64(to.Len())
	if room <= 0 {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r, room))
	if err != nil && len(body) == 0 {
		return
	}
	if to.Len() > 0 {
		to.WriteString("\n\n")
	}
	to.Write(body)
}

// readFile reads a file, keeping its size only if too large to keep. A file's
// name comes from whoever sent it, so it is cleaned where it is stored.
func readFile(name, kind string, r io.Reader) (Attachment, bool) {
	a := Attachment{Name: clean(name), Type: kind}
	if a.Name == "" {
		a.Name = "file"
	}
	data, err := io.ReadAll(io.LimitReader(r, maxFile+1))
	if err != nil && len(data) == 0 {
		return a, false
	}
	if len(data) > maxFile {
		n, _ := io.Copy(io.Discard, r)
		a.Size = int64(len(data)) + n
		return a, true
	}
	a.Data = data
	return a, true
}

// htmlToText reads HTML as text. Nothing of it is ever shown as HTML.
func htmlToText(source string) string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.Join(strings.Fields(n.Data), " ")
			if text != "" {
				if ends := b.String(); ends != "" && !strings.HasSuffix(ends, "\n") && !strings.HasSuffix(ends, " ") {
					b.WriteByte(' ')
				}
				b.WriteString(text)
			}
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "head", "title", "meta":
				return
			case "br":
				b.WriteByte('\n')
				return
			case "hr":
				b.WriteString("\n\n")
				return
			case "li":
				b.WriteString("\n- ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "p", "div", "tr", "table", "ul", "ol", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
			b.WriteString("\n\n")
		case "a":
			// Where a link leads is lost otherwise, and is often the point.
			for _, at := range n.Attr {
				if at.Key == "href" && (strings.HasPrefix(at.Val, "https://") || strings.HasPrefix(at.Val, "http://")) {
					b.WriteString(" (" + at.Val + ")")
				}
			}
		}
	}
	walk(doc)
	return blankLines.ReplaceAllString(strings.TrimSpace(b.String()), "\n\n")
}

var blankLines = regexp.MustCompile(`\n[ \t]*(\n[ \t]*)+`)

// Automatic says the mail was sent by a machine, not forwarded by a person.
func (m *Message) Automatic() bool {
	h := m.header
	if v := strings.ToLower(strings.TrimSpace(h.Get("Auto-Submitted"))); v != "" && v != "no" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(h.Get("Precedence"))) {
	case "bulk", "list", "junk", "auto_reply":
		return true
	}
	return h.Get("List-Id") != "" || h.Get("List-Unsubscribe") != "" || h.Get("X-Autoreply") != "" || h.Get("X-Autorespond") != ""
}

// Spam says the mailbox's provider or a filter before it called this spam.
func (m *Message) Spam() bool {
	h := m.header
	return strings.EqualFold(strings.TrimSpace(h.Get("X-Spam-Flag")), "yes") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.Get("X-Spam-Status"))), "yes")
}

// Forged says the sender's domain checks failed, as the receiving server saw it.
// Its header comes first, and anything below it may be the sender's own, so a
// sender can add nothing to get past this. No results at all pass: not every
// server writes them. DMARC decides where it ran; without it, SPF failing,
// softly or not, is forgery unless a DKIM signature of the From address's own
// domain passed: anyone can sign with a domain of their own.
func (m *Message) Forged() bool {
	header := m.header.Get("Authentication-Results")
	results := authResults(header)
	switch results["dmarc"] {
	case "fail":
		return true
	case "pass":
		return false
	}
	if spf := results["spf"]; spf != "fail" && spf != "softfail" {
		return false
	}
	_, domain, _ := strings.Cut(m.From, "@")
	return !signedBy(header, domain)
}

var dkimDomain = regexp.MustCompile(`header\.d\s*=\s*([a-z0-9.-]+)`)

// signedBy says a DKIM signature passed for the domain, or one it is part
// of or that is part of it (relaxed alignment, as DMARC has it).
func signedBy(header, domain string) bool {
	if domain == "" {
		return false
	}
	for _, part := range strings.Split(quotedOrComment.ReplaceAllString(strings.ToLower(header), ""), ";") {
		found := authMethod.FindStringSubmatch(part)
		if found == nil || found[1] != "dkim" || found[2] != "pass" {
			continue
		}
		if d := dkimDomain.FindStringSubmatch(part); d != nil {
			signer := strings.TrimSuffix(d[1], ".")
			if signer == domain || strings.HasSuffix(domain, "."+signer) || strings.HasSuffix(signer, "."+domain) {
				return true
			}
		}
	}
	return false
}

var (
	quotedOrComment = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|\([^()]*\)`)
	authMethod      = regexp.MustCompile(`^\s*(spf|dkim|dmarc)\s*=\s*([a-z]+)`)
)

// authResults reads an Authentication-Results header: each check's first result,
// taken only where a check begins. What a sender chose, such as the address the
// mail claims to be from, is often quoted further along and must not count.
func authResults(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(quotedOrComment.ReplaceAllString(strings.ToLower(header), ""), ";") {
		if found := authMethod.FindStringSubmatch(part); found != nil {
			if _, seen := out[found[1]]; !seen {
				out[found[1]] = found[2]
			}
		}
	}
	return out
}

// routeHeaders are where a recipient's address can be found, what the servers on
// the way said first: mail sent by Bcc names it only there, and its To may name
// someone else's address entirely.
var routeHeaders = []string{"Delivered-To", "X-Original-To", "Envelope-To", "X-Envelope-To", "X-Forwarded-To", "To", "Cc", "Bcc", "Resent-To", "Received"}

// RecipientsMatching finds the first token a message is addressed to, in the
// order of routeHeaders.
func (m *Message) RecipientsMatching(pattern *regexp.Regexp) string {
	for _, name := range routeHeaders {
		fields := m.header.FieldsByKey(name)
		for fields.Next() {
			if found := pattern.FindStringSubmatch(fields.Value()); found != nil {
				return strings.ToLower(found[1])
			}
		}
	}
	return ""
}

// parseHeaders reads the headers of a message of size bytes, however much of
// them was fetched. A header cut off where the limit fell is dropped.
func parseHeaders(raw []byte, size int64) (*Message, error) {
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		raw = raw[:i]
	} else if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		raw = raw[:i]
	} else if i := bytes.LastIndexByte(raw, '\n'); i >= 0 && len(raw) >= maxHeaderRead {
		raw = raw[:i]
	}
	// Without its parts, a multipart message's headers read as an empty message.
	m, err := Parse(bytes.NewReader(append(bytes.TrimRight(raw, "\r\n"), "\r\n\r\n"...)))
	if err != nil {
		return nil, err
	}
	m.Size = size
	m.TooLarge = size > maxMessage
	m.Text = ""
	m.Attachments = nil
	return m, nil
}

// titleOf is a task's title for a message: its subject without what forwarding
// and replying put before it, or the first line of the message, or whom it is from.
func titleOf(m *Message, maxRunes int) string {
	title := strings.Join(strings.Fields(m.Subject), " ")
	for {
		cut := replyPrefix.ReplaceAllString(title, "")
		if cut == title {
			break
		}
		title = cut
	}
	if title == "" {
		for _, line := range strings.Split(m.Text, "\n") {
			if line = strings.Join(strings.Fields(line), " "); line != "" {
				title = line
				break
			}
		}
	}
	if title == "" {
		title = "Email from " + m.From
	}
	if utf8.RuneCountInString(title) > maxRunes {
		title = strings.TrimSpace(string([]rune(title)[:maxRunes]))
	}
	return title
}

var replyPrefix = regexp.MustCompile(`(?i)^\s*(fwd?|re)\s*:\s*`)
