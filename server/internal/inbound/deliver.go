package inbound

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
	"github.com/locodarwin/mila/server/internal/ydoc"
)

// What became of a message, as Settings, Email to task lists it.
const (
	Made            = "made"
	RefusedSender   = "refused_sender"
	RefusedAuto     = "refused_automatic"
	RefusedSpam     = "refused_spam"
	RefusedForged   = "refused_forged"
	RefusedLimit    = "refused_limit"
	RefusedPlace    = "refused_place"
	RefusedTooLarge = "refused_too_large"
	Failed          = "failed"
)

const (
	// maxPerDay is how many tasks one address makes in a day.
	maxPerDay = 50
	// maxBytesPerDay is how much mail one address takes in a day, so that a stolen
	// address cannot fill the disk with files.
	maxBytesPerDay = 500 << 20
	// maxRefusedListed is how many refusals one address lists in a day, so junk
	// cannot fill the list.
	maxRefusedListed = 20
	maxSubject       = 200
	maxFromShown     = 254
	maxTitle         = 500
	maxNotes         = 100_000
	keepMail         = 30 * 24 * time.Hour
)

// Notices tells a person their mail became a task.
type Notices interface {
	Send(ctx context.Context, list ...notify.Notice) error
}

// Service makes tasks of mail.
type Service struct {
	st      *store.Store
	log     *slog.Logger
	sync    *syncer.Service
	files   *files.Store
	notices Notices
	pattern *regexp.Regexp
	now     func() time.Time
}

// AddressPattern finds a person's token in a header, from the address the
// server's settings say theirs look like.
func AddressPattern(template string) *regexp.Regexp {
	before, after, _ := strings.Cut(template, "{token}")
	return regexp.MustCompile(`(?i)(?:^|[<\s,;:"'(])` + regexp.QuoteMeta(before) + `([a-z2-7]{26})` + regexp.QuoteMeta(after) + `(?:$|[>\s,;"')])`)
}

// New makes the service, for addresses that look like template.
func New(st *store.Store, log *slog.Logger, sync *syncer.Service, fs *files.Store, notices Notices, template string) *Service {
	return &Service{st: st, log: log, sync: sync, files: fs, notices: notices, pattern: AddressPattern(template), now: store.Now}
}

// screened is a message that has passed every check, ready to be read in full.
type screened struct {
	addr sqlitedb.InboundAddress
	user store.User
	id   string
	// resuming says its task was made, by a delivery cut short before it finished.
	resuming bool
}

// Deliver deals with one message read in full, all at once. It returns whether the
// message named one of Mila's addresses at all. An error means trying again later
// may help: a problem with this message itself is recorded and is no error.
func (s *Service) Deliver(ctx context.Context, m *Message) (bool, error) {
	ok, named, err := s.Screen(ctx, m)
	if ok == nil || err != nil {
		return named, err
	}
	return named, s.accept(ctx, ok, m)
}

// Screen decides about a message from its headers alone, so that mail from a
// stranger never costs more than reading those. It returns what to go on with if
// the message should be read and made a task of, and whether it named an address.
func (s *Service) Screen(ctx context.Context, m *Message) (*screened, bool, error) {
	token := m.RecipientsMatching(s.pattern)
	if token == "" {
		return nil, false, nil
	}
	addr, err := s.st.GetInboundAddressByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		// A deleted address and one that never existed are the same to a sender.
		return nil, true, nil
	}
	if err != nil {
		return nil, true, err
	}
	user, err := s.st.GetUser(ctx, addr.UserID)
	if err != nil {
		return nil, true, err
	}
	since := s.now().Add(-24 * time.Hour)
	refuse := func(outcome string) (*screened, bool, error) {
		return nil, true, s.refuse(ctx, addr, m, outcome)
	}
	member, err := s.member(ctx, addr)
	if err != nil {
		return nil, true, err
	}
	switch {
	case !member:
		return nil, true, nil
	case !accepts(addr, user, m.From):
		return refuse(RefusedSender)
	case m.Automatic():
		return refuse(RefusedAuto)
	case m.Spam():
		return refuse(RefusedSpam)
	case m.Forged():
		return refuse(RefusedForged)
	case m.TooLarge:
		return refuse(RefusedTooLarge)
	}
	if addr.ProjectID.Valid {
		n, err := s.st.CountLiveProject(ctx, sqlitedb.CountLiveProjectParams{ID: addr.ProjectID.String, WorkspaceID: addr.WorkspaceID})
		if err != nil {
			return nil, true, err
		}
		if n == 0 {
			return refuse(RefusedPlace)
		}
	}
	// The same message always makes the same task, so one delivered again, or
	// read again after a restart, finds it there, even in the trash. One with no
	// line in the list was cut short, and is finished now.
	ok := &screened{addr: addr, user: user, id: taskID(addr.ID, m)}
	if _, there, err := s.sync.Task(ctx, ok.id); err != nil {
		return nil, true, err
	} else if there {
		finished, err := s.st.CountInboundMailForTask(ctx, sql.NullString{String: ok.id, Valid: true})
		if err != nil {
			return nil, true, err
		}
		if finished > 0 {
			return nil, true, nil
		}
		ok.resuming = true
		return ok, true, nil
	}
	made, err := s.st.CountInboundMadeSince(ctx, sqlitedb.CountInboundMadeSinceParams{AddressID: addr.ID, Since: since})
	if err != nil {
		return nil, true, err
	}
	taken, err := s.st.SumInboundBytesSince(ctx, sqlitedb.SumInboundBytesSinceParams{AddressID: addr.ID, Since: since})
	if err != nil {
		return nil, true, err
	}
	if made >= maxPerDay || taken+m.Size > maxBytesPerDay {
		return refuse(RefusedLimit)
	}
	return ok, true, nil
}

// refuse lists a refusal, up to the number listed in a day.
func (s *Service) refuse(ctx context.Context, addr sqlitedb.InboundAddress, m *Message, outcome string) error {
	n, err := s.st.CountInboundRefusedSince(ctx, sqlitedb.CountInboundRefusedSinceParams{AddressID: addr.ID, Since: s.now().Add(-24 * time.Hour)})
	if err != nil {
		return err
	}
	if n >= maxRefusedListed {
		return nil
	}
	return s.record(ctx, addr, m, outcome, "")
}

// accept makes the task of a message that passed Screen, read in full now.
func (s *Service) accept(ctx context.Context, ok *screened, m *Message) error {
	if err := s.makeTask(ctx, ok, m); err != nil {
		if ctx.Err() != nil || apperr.KindOf(err) == 0 {
			return err
		}
		// Refused by the rules for tasks, which trying again will not change.
		s.log.Warn("email to task: a task was refused", "err", err)
		return s.record(ctx, ok.addr, m, Failed, "")
	}
	// Written last, so that a delivery cut short before it is finished the next time.
	return s.record(ctx, ok.addr, m, Made, ok.id)
}

func (s *Service) member(ctx context.Context, addr sqlitedb.InboundAddress) (bool, error) {
	ws, err := s.st.GetWorkspace(ctx, addr.WorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if ws.DeletedAt.Valid {
		return false, nil
	}
	return s.sync.CanEdit(ctx, addr.UserID, addr.WorkspaceID)
}

func accepts(addr sqlitedb.InboundAddress, user store.User, from string) bool {
	if from == "" {
		return false
	}
	if strings.EqualFold(from, user.Email) {
		return true
	}
	var listed []string
	_ = json.Unmarshal([]byte(addr.Senders), &listed)
	return slices.Contains(listed, from)
}

// taskID is a message's task, the same for the same message at the same address.
func taskID(addressID string, m *Message) string {
	key := m.MessageID
	if key == "" {
		key = m.From + "\x00" + m.Subject + "\x00" + m.Date.UTC().Format(time.RFC3339)
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(addressID+"\x00"+key)).String()
}

// makeTask makes a message's task and all that goes with it. Every step can be
// done again, so a delivery cut short can be finished by running it once more.
func (s *Service) makeTask(ctx context.Context, ok *screened, m *Message) error {
	ctx = syncer.WithEmail(ctx)
	addr, id := ok.addr, ok.id
	title := titleOf(m, maxTitle)
	var left []string
	var kept []Attachment
	room := s.files.Room() == nil
	for _, a := range m.Attachments {
		switch {
		case a.Size > 0:
			left = append(left, fmt.Sprintf("Not kept, larger than 25 MB: %s (%d MB)", files.CleanName(a.Name), a.Size>>20))
		case !room:
			left = append(left, "Not kept, no room on the server: "+files.CleanName(a.Name))
		default:
			kept = append(kept, a)
		}
	}
	if m.MoreFiles > 0 {
		left = append(left, fmt.Sprintf("Not kept, past the %d files an email can bring: %d more", maxFiles, m.MoreFiles))
	}
	if !ok.resuming {
		fields := map[string]any{"title": title, "notes": notesOf(m, ok.user, left, s.now())}
		if addr.ProjectID.Valid {
			fields["project_id"] = addr.ProjectID.String
			status, err := s.sync.StartingStatus(ctx, addr.ProjectID.String)
			if err != nil {
				return err
			}
			if status != "" {
				fields["status_id"] = status
			}
		}
		if _, _, err := s.sync.Write(ctx, addr.UserID, addr.WorkspaceID, syncer.Write{Entity: "task", ID: id, Mode: syncer.Create, Fields: rawFields(fields)}); err != nil {
			return err
		}
	}
	if addr.AssignSelf {
		if _, _, err := s.sync.Write(ctx, addr.UserID, addr.WorkspaceID, syncer.Write{
			Entity: "task_assignee", Mode: syncer.Upsert, Fields: rawFields(map[string]any{"task_id": id, "user_id": addr.UserID}),
		}); err != nil {
			if ctx.Err() != nil || apperr.KindOf(err) == 0 {
				return err
			}
			s.log.Warn("email to task: not assigned", "err", err)
		}
	}
	for i, a := range kept {
		if err := s.attach(ctx, addr, id, i, a); err != nil {
			if ctx.Err() != nil || apperr.KindOf(err) == 0 && !errors.Is(err, files.ErrTooLarge) {
				return err
			}
			s.log.Warn("email to task: a file was not kept", "err", err)
		}
	}
	if s.notices != nil {
		if task, there, err := s.sync.Task(ctx, id); err == nil && there {
			if err := s.notices.Send(ctx, notify.Notice{
				UserID: addr.UserID, WorkspaceID: addr.WorkspaceID, Kind: notify.EmailTask, Task: &task,
				Title: "Made a task from your email: " + title, Body: m.From,
			}); err != nil {
				s.log.Warn("email to task: no notification", "err", err)
			}
		}
	}
	return nil
}

// attach keeps one file of a message, under an ID made from its task and place
// among the files, so that finishing a delivery cut short keeps it once.
func (s *Service) attach(ctx context.Context, addr sqlitedb.InboundAddress, taskID string, index int, a Attachment) error {
	id := uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "%s\x00file\x00%d", taskID, index)).String()
	if _, err := s.sync.Attachment(ctx, addr.UserID, addr.WorkspaceID, id); err == nil {
		return nil
	} else if apperr.KindOf(err) != apperr.NotFound {
		return err
	}
	key, size, head, err := s.files.Save(bytes.NewReader(a.Data), maxFile)
	if err != nil {
		return err
	}
	_, err = s.sync.CreateAttachment(ctx, addr.UserID, addr.WorkspaceID, syncer.NewAttachment{
		ID: id, TaskID: taskID, Filename: files.CleanName(a.Name),
		Mime: files.MediaType(a.Type, head), StorageKey: key, Size: size,
	})
	return err
}

func rawFields(fields map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(fields))
	for k, v := range fields {
		out[k], _ = json.Marshal(v)
	}
	return out
}

// notesOf is a task's notes for a message, as Markdown that says only what the
// message said: whatever marks it holds are text. A line first says who it was
// from and when, in the person's own way of writing dates.
func notesOf(m *Message, user store.User, left []string, now time.Time) string {
	at := m.Date
	if at.IsZero() {
		at = now
	}
	local := at.In(accounts.Location(user))
	style := accounts.DateStyleOf(user)
	lead := fmt.Sprintf("By email from %s, %s at %s", m.From, style.Day(local, false, now), style.Time(local.Format("15:04")))
	foot := ""
	for _, line := range left {
		foot += "\n\n" + line
	}
	text := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\r", "\n")
	build := func(body string) string {
		return ydoc.Markdown(paragraphs(lead + "\n\n" + strings.TrimSpace(body) + foot))
	}
	notes := build(text)
	// Marks are escaped, so the limit is on the Markdown and not the message.
	if utf8.RuneCountInString(notes) <= maxNotes {
		return notes
	}
	const cut = "\n\nThe rest of this email was too long to keep here."
	runes := []rune(text)
	for keep := maxNotes - maxNotes/8; keep > 0; keep -= maxNotes / 8 {
		if keep < len(runes) {
			if notes = build(string(runes[:keep]) + cut); utf8.RuneCountInString(notes) <= maxNotes {
				return notes
			}
		}
	}
	return ydoc.Markdown(paragraphs(lead + cut))
}

// paragraphs makes a document of plain text: paragraphs at blank lines, and the
// line breaks in each kept.
func paragraphs(text string) *ydoc.Node {
	doc := ydoc.Doc()
	for _, para := range blankLines.Split(text, -1) {
		if strings.TrimSpace(para) == "" {
			continue
		}
		p := &ydoc.Node{Type: "paragraph"}
		for i, line := range strings.Split(para, "\n") {
			if i > 0 {
				p.Content = append(p.Content, &ydoc.Node{Type: "hardBreak"})
			}
			// Spaces before a line would make a block of it: a heading or a fence.
			if line = strings.TrimSpace(line); line != "" {
				p.Content = append(p.Content, &ydoc.Node{Type: "text", Text: line})
			}
		}
		doc.Content = append(doc.Content, p)
	}
	return doc
}

func (s *Service) record(ctx context.Context, addr sqlitedb.InboundAddress, m *Message, outcome, task string) error {
	cut := func(text string, n int) string {
		text = strings.Join(strings.Fields(text), " ")
		if utf8.RuneCountInString(text) > n {
			return string([]rune(text)[:n])
		}
		return text
	}
	params := sqlitedb.InsertInboundMailParams{
		ID: uuid.NewString(), UserID: addr.UserID, WorkspaceID: addr.WorkspaceID, AddressID: addr.ID,
		FromAddress: cut(m.From, maxFromShown), Subject: cut(m.Subject, maxSubject), Outcome: outcome, Now: s.now(),
		Size: m.Size,
	}
	if task != "" {
		params.TaskID = sql.NullString{String: task, Valid: true}
	}
	return s.st.InsertInboundMail(ctx, params)
}

// Prune forgets mail older than the list shows.
func (s *Service) Prune(ctx context.Context) error {
	return s.st.PurgeInboundMail(ctx, s.now().Add(-keepMail))
}
