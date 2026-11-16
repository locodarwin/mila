package ownership

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/accounts"
	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Limits on what an import reads from its zip, so a hostile one cannot
// fill the memory or the disk.
const (
	maxEntries  = 200_000
	maxManifest = 1 << 20
	maxContent  = 512 << 20
	// workspace.json unpacks to at most contentRatio times what it takes
	// in the zip (Mila's own exports zip about 8.5 to 1), but never less
	// than minContent; and its rows may take at most memRatio times that
	// in memory, as memCost reckons it (Mila's own reckon at under 6 times
	// their JSON), so a small zip cannot unpack into gigabytes.
	contentRatio    = 32
	minContent      = 4 << 20
	memRatio        = 4
	maxAttachment   = 25 << 20 // as an upload's
	attachmentRatio = 2        // all files together, as a multiple of the upload
	// An export's rows and history entries together, at most one per
	// minRowBytes of the upload (Mila's own exports zip a row into about
	// 95 bytes), but never fewer than minRows: a few kilobytes of zip
	// cannot unpack into gigabytes of rows.
	minRowBytes = 16
	minRows     = 10_000
)

// rowLimit is how many rows and history entries an upload of size bytes
// may hold.
func rowLimit(size int64) int {
	return int(min(max(size/minRowBytes, minRows), syncer.MaxRestoreRows))
}

// ErrTooLarge means an import's upload passed the size limit.
var ErrTooLarge = files.ErrTooLarge

// Import is one import as its owner sees it.
type Import = sqlitedb.ListImportsRow

// Report is what an import made of its export, stored with it as JSON.
type Report struct {
	syncer.RestoreReport
	// Files counts the attachment files brought in, and MissingFiles the
	// attachments left out because the zip had no file for them, or the
	// file was larger than an import of its size may bring in.
	Files        int `json:"files"`
	MissingFiles int `json:"missingFiles"`
}

// StartImport takes a JSON export's zip and queues its import into a new
// workspace for the user, keeping its IDs or giving every row new ones.
// The zip is checked enough to refuse the wrong kind of file at once.
func (s *Service) StartImport(ctx context.Context, userID string, upload io.Reader, fileName string, keepIDs bool) (Import, error) {
	// One upload at a time per person, held from before the upload is
	// read: the count of imports below sees only those already queued, so
	// uploads sent together would all pass it.
	if _, busy := s.uploading.LoadOrStore(userID, true); busy {
		return Import{}, apperr.New(apperr.Conflict, "An import is on its way already. Wait for it to finish.")
	}
	defer s.uploading.Delete(userID)
	if err := files.CheckRoom(s.imports); err != nil {
		return Import{}, err
	}
	n, err := s.st.CountUnfinishedImports(ctx, userID)
	if err != nil {
		return Import{}, err
	}
	if n > 0 {
		return Import{}, apperr.New(apperr.Conflict, "An import is on its way already. Wait for it to finish.")
	}
	id := uuid.Must(uuid.NewV7()).String()
	tmp, err := os.CreateTemp(s.imports, id+".tmp-*")
	if err != nil {
		return Import{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	size, err := io.Copy(tmp, io.LimitReader(upload, s.max+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Import{}, err
	}
	if size > s.max {
		return Import{}, ErrTooLarge
	}
	if _, err := readManifest(tmp.Name()); err != nil {
		return Import{}, err
	}
	if err := os.Rename(tmp.Name(), s.importPath(id)); err != nil {
		return Import{}, err
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || len(fileName) > 255 {
		fileName = "export.zip"
	}
	now := s.now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.InsertImport(ctx, sqlitedb.InsertImportParams{ID: id, UserID: userID, FileName: fileName, KeepIds: keepIDs, Now: now}); err != nil {
			return err
		}
		return jobs.Enqueue(ctx, q, jobImport, ref{ID: id}, now, now)
	})
	if err != nil {
		_ = os.Remove(s.importPath(id))
		return Import{}, err
	}
	s.jobs.Wake()
	return Import{ID: id, UserID: userID, FileName: fileName, KeepIds: keepIDs, Status: "queued", Report: "{}", CreatedAt: now}, nil
}

// Imports lists the user's imports, newest first.
func (s *Service) Imports(ctx context.Context, userID string) ([]Import, error) {
	return s.st.ListImports(ctx, userID)
}

// readManifest opens a zip and checks it is a JSON export this Mila reads.
func readManifest(name string) (*Manifest, error) {
	z, _, err := openZip(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = z.Close() }()
	return manifestOf(&z.Reader)
}

// openZip opens an uploaded zip, once its entries are counted, and gives
// its size.
func openZip(name string) (*zip.ReadCloser, int64, error) {
	notZip := apperr.New(apperr.Invalid, "That is not a zip file. Choose the zip a JSON export made.")
	f, err := os.Open(name)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	n, err := countEntries(f, info.Size(), maxEntries)
	_ = f.Close()
	if err != nil {
		return nil, 0, notZip
	}
	if n > maxEntries {
		return nil, 0, apperr.New(apperr.Invalid, "The zip holds too many files to be a Mila export.")
	}
	z, err := zip.OpenReader(name)
	if err != nil {
		return nil, 0, notZip
	}
	return z, info.Size(), nil
}

func manifestOf(z *zip.Reader) (*Manifest, error) {
	if len(z.File) > maxEntries {
		return nil, apperr.New(apperr.Invalid, "The zip holds too many files to be a Mila export.")
	}
	var m Manifest
	err := readJSON(z, "manifest.json", func(*zip.File) int64 { return maxManifest }, func(r io.Reader, _ int64) error { return json.NewDecoder(r).Decode(&m) })
	if err != nil || m.Format != FormatName {
		return nil, apperr.New(apperr.Invalid, "This zip is not a Mila JSON export (it has no Mila manifest.json). CSV and Markdown exports cannot be imported.")
	}
	if m.Version > FormatVersion || m.Version < 1 {
		return nil, apperr.New(apperr.Invalid, "This export was made by a newer Mila (format %d). Update this Mila to import it.", m.Version)
	}
	return &m, nil
}

// contentLimit is how many bytes workspace.json may unpack to, from what it
// takes in an upload of size bytes.
func contentLimit(f *zip.File, size int64) int64 {
	return min(maxContent, max(minContent, min(int64(f.CompressedSize64), size)*contentRatio))
}

// readJSON decodes one file of the zip, reading at most limit bytes.
func readJSON(z *zip.Reader, name string, limitOf func(*zip.File) int64, decode func(r io.Reader, limit int64) error) error {
	for _, f := range z.File {
		if f.Name != name {
			continue
		}
		limit := limitOf(f)
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		lr := &io.LimitedReader{R: r, N: limit + 1}
		if err := decode(lr, limit); err != nil {
			if lr.N <= 0 {
				return apperr.New(apperr.Invalid, "%s is larger than an import reads.", name)
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("no %s", name)
}

// decodeContent reads workspace.json a row at a time, so it can stop
// once the rows and history entries pass limit, or what they would take
// in memory passes mem bytes, before they fill the memory.
func decodeContent(r io.Reader, limit int, mem int64, c *Content) error {
	cr := &rowCap{r: r, max: maxRowBytes, mem: mem}
	dec := json.NewDecoder(cr)
	if tok, err := dec.Token(); err != nil {
		return err
	} else if tok != json.Delim('{') {
		return errors.New("workspace.json is not an object")
	}
	n := 0
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok {
		case "rows":
			err = decodeList(dec, cr, &c.Rows, &n, limit)
		case "activity":
			err = decodeList(dec, cr, &c.Activity, &n, limit)
		case "pageVersions":
			err = decodeList(dec, cr, &c.Versions, &n, limit)
		case "people":
			err = decodeList(dec, cr, &c.People, &n, limit)
		default:
			err = skipValue(dec)
		}
		if errors.Is(err, errRowTooBig) {
			return apperr.New(apperr.Invalid, "The export holds a row larger than any Mila writes. Choose the zip a JSON export made.")
		}
		if errors.Is(err, errTooDense) {
			return apperr.New(apperr.Invalid, "The export holds more than a zip of its size could. Choose the zip a JSON export made.")
		}
		if err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

// maxRowBytes is the most JSON one row of an export may take. Mila's rows
// are a few kilobytes at most; without a cap, one row of a small zip
// could unpack into a string of hundreds of megabytes in memory.
const maxRowBytes = 8 << 20

var (
	errRowTooBig = errors.New("a row is too large")
	errTooDense  = errors.New("the rows would take too much memory")
)

// rowCap is the reader under the decoder: it fails once the decoder has
// read more than max bytes past where the row it is on began (mark, the
// decoder's offset there), or once what it has read would take more than
// mem bytes decoded.
type rowCap struct {
	r       io.Reader
	n, mark int64
	max     int64
	mem     int64
	cost    memCost
}

func (c *rowCap) Read(p []byte) (int, error) {
	left := c.max - (c.n - c.mark)
	if left < 0 {
		return 0, errRowTooBig
	}
	if c.cost.n > c.mem {
		return 0, errTooDense
	}
	// No more than the row may still take, so the next read is checked.
	if int64(len(p)) > left+1 {
		p = p[:left+1]
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.cost.add(p[:n])
	return n, err
}

// memCost is about what JSON takes once decoded into Go values, bytes
// and all: each value is an interface (16 bytes), a string's text is
// allocated apart, an array is a slice, and an object a map, whose
// first entries alone take some 300 bytes; all far more than the few
// bytes they take as JSON. It reads the JSON a
// piece at a time, keeping its place inside strings.
type memCost struct {
	n              int64
	inString, skip bool
}

func (m *memCost) add(p []byte) {
	m.n += int64(len(p))
	for _, c := range p {
		switch {
		case m.skip:
			m.skip = false
		case m.inString:
			switch c {
			case '\\':
				m.skip = true
			case '"':
				m.inString = false
			}
		case c == '"':
			m.inString = true
			m.n += 24
		case c == ',':
			m.n += 16
		case c == ':':
			m.n += 32
		case c == '{':
			m.n += 320
		case c == '[':
			m.n += 40
		}
	}
}

// decodeList reads a JSON array (or null) into list an element at a time,
// counting each against limit in n.
func decodeList[T any](dec *json.Decoder, cr *rowCap, list *[]T, n *int, limit int) error {
	tok, err := dec.Token()
	if err != nil || tok == nil {
		return err
	}
	if tok != json.Delim('[') {
		return errors.New("expected a list")
	}
	for dec.More() {
		if *n++; *n > limit {
			if limit >= syncer.MaxRestoreRows {
				return apperr.New(apperr.Invalid, "The export holds more than %d rows, more than one import takes.", limit)
			}
			return apperr.New(apperr.Invalid, "The export holds more rows than a zip of its size could. Choose the zip a JSON export made.")
		}
		cr.mark = dec.InputOffset()
		var v T
		if err := dec.Decode(&v); err != nil {
			return err
		}
		*list = append(*list, v)
	}
	_, err = dec.Token()
	return err
}

// skipValue reads past one JSON value without keeping it.
func skipValue(dec *json.Decoder) error {
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
		if depth == 0 {
			return nil
		}
	}
}

func (s *Service) handleImport(ctx context.Context, payload []byte) error {
	var r ref
	if err := json.Unmarshal(payload, &r); err != nil {
		return err
	}
	im, err := s.st.GetImport(ctx, r.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && im.Status != "queued" && im.Status != "running") {
		_ = os.Remove(s.importPath(r.ID))
		return nil
	}
	if err != nil {
		return err
	}
	// A run cut short by a stop leaves a hidden workspace; start again
	// from nothing.
	if im.WorkspaceID.Valid {
		if err := s.st.PurgeWorkspace(ctx, im.WorkspaceID.String); err != nil {
			return err
		}
	}
	// Still running means the last run never finished, not even to say
	// it was stopped: the server died under it (the import may be what
	// killed it). It is not tried again.
	if im.Status == "running" {
		_ = os.Remove(s.importPath(im.ID))
		return s.st.FailImport(ctx, sqlitedb.FailImportParams{
			ID: im.ID, Report: "{}", Now: sql.NullTime{Time: s.now(), Valid: true},
			Error: sql.NullString{String: "The server stopped partway through this import. Try it again, or tell your admin if it happens again.", Valid: true},
		})
	}
	var rep Report
	wsID, name, err := s.runImport(ctx, im, &rep)
	if err != nil && ctx.Err() != nil {
		// The server is stopping: queue it again, upload and all, for the
		// job's retry after the restart (which removes the workspace).
		if qerr := s.st.RequeueImport(context.WithoutCancel(ctx), im.ID); qerr != nil {
			s.log.Warn("queue an import again", "import", im.ID, "err", qerr)
		}
		return ctx.Err()
	}
	defer func() { _ = os.Remove(s.importPath(im.ID)) }()
	raw, _ := json.Marshal(rep)
	now := s.now()
	if err != nil {
		if wsID != "" {
			if perr := s.st.PurgeWorkspace(ctx, wsID); perr != nil {
				s.log.Warn("remove a failed import's workspace", "import", im.ID, "err", perr)
			}
		}
		if apperr.KindOf(err) == 0 {
			s.log.Error("import failed", "import", im.ID, "err", err)
		}
		return s.st.FailImport(ctx, sqlitedb.FailImportParams{
			ID: im.ID, Report: string(raw), Now: sql.NullTime{Time: now, Valid: true},
			Error: sql.NullString{String: message(err, "The import failed on the server. Try again, or tell your admin."), Valid: true},
		})
	}
	// The person may have deleted their account meanwhile, taking the
	// import's record with it; then the workspace goes too.
	if _, err := s.st.GetImport(ctx, im.ID); errors.Is(err, sql.ErrNoRows) {
		return s.st.PurgeWorkspace(ctx, wsID)
	} else if err != nil {
		return err
	}
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.CreateMembership(ctx, sqlitedb.CreateMembershipParams{WorkspaceID: wsID, UserID: im.UserID, Role: string(authz.Owner), Now: now}); err != nil {
			return err
		}
		if err := q.ShowWorkspace(ctx, sqlitedb.ShowWorkspaceParams{ID: wsID, Now: now}); err != nil {
			return err
		}
		return q.FinishImport(ctx, sqlitedb.FinishImportParams{ID: im.ID, Report: string(raw), Now: sql.NullTime{Time: now, Valid: true}})
	})
	if err != nil {
		_ = s.st.PurgeWorkspace(ctx, wsID)
		return err
	}
	s.events.MembershipChanged(im.UserID)
	tasks := rep.Restored["task"]
	body := fmt.Sprintf("%d %s and %d %s came in.", tasks, plural(tasks, "task", "tasks"), rep.Restored["project"], plural(rep.Restored["project"], "project", "projects"))
	if left := sum(rep.Dropped) + rep.MissingFiles; left > 0 {
		body += fmt.Sprintf(" %d %s left out; Settings, Import and export, says why.", left, plural(left, "thing was", "things were"))
	}
	if err := s.tell.Inform(ctx, im.UserID, wsID, notify.ImportDone, "Imported "+name, body); err != nil {
		s.log.Warn("tell about import", "import", im.ID, "err", err)
	}
	return nil
}

// runImport restores the upload into a new, hidden workspace and returns
// it, with its name. On failure the workspace (if made) is the caller's to
// remove.
func (s *Service) runImport(ctx context.Context, im sqlitedb.Import, rep *Report) (string, string, error) {
	z, size, err := openZip(s.importPath(im.ID))
	if err != nil {
		return "", "", err
	}
	defer func() { _ = z.Close() }()
	m, err := manifestOf(&z.Reader)
	if err != nil {
		return "", "", err
	}

	name := strings.TrimSpace(m.Workspace.Name)
	if name == "" || len([]rune(name)) > 100 {
		name = "Imported workspace"
	}
	wsID := uuid.Must(uuid.NewV7()).String()
	now := s.now()
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.CreateWorkspace(ctx, sqlitedb.CreateWorkspaceParams{ID: wsID, Name: name, Slug: accounts.SlugFor(name), IsPersonal: false, Now: now}); err != nil {
			return err
		}
		// Out of sight until it is complete.
		if err := q.DeleteWorkspace(ctx, sqlitedb.DeleteWorkspaceParams{ID: wsID, Now: sql.NullTime{Time: now, Valid: true}}); err != nil {
			return err
		}
		return q.StartImport(ctx, sqlitedb.StartImportParams{ID: im.ID, WorkspaceID: sql.NullString{String: wsID, Valid: true}})
	})
	if err != nil {
		return "", "", err
	}

	// Running from here: if the server dies while this reads, the import
	// is not tried again.
	var content Content
	err = readJSON(&z.Reader, "workspace.json", func(f *zip.File) int64 { return contentLimit(f, size) }, func(r io.Reader, limit int64) error {
		return decodeContent(r, rowLimit(size), limit*memRatio, &content)
	})
	if err != nil {
		if apperr.KindOf(err) != 0 {
			return wsID, name, err
		}
		return wsID, name, apperr.New(apperr.Invalid, "The export's workspace.json is missing or damaged.")
	}

	// Attachment files come in first, under new keys; an attachment whose
	// file is not in the zip is left out.
	entries := map[string]*zip.File{}
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "attachments/") && !f.FileInfo().IsDir() {
			parts := strings.SplitN(strings.TrimPrefix(f.Name, "attachments/"), "/", 2)
			if len(parts) == 2 && entries[parts[0]] == nil {
				entries[parts[0]] = f
			}
		}
	}
	// Files together may take twice what the upload does, which an export
	// of Mila's own (files stored, not compressed) never needs; a zip of
	// files that compress to nothing writes no more than that. The disk is
	// checked before each one, as an upload's is.
	budget := size * attachmentRatio
	rows := content.Rows[:0]
	for _, row := range content.Rows {
		if row.Entity == "attachment" || row.Entity == "page_attachment" {
			f := entries[row.ID]
			if f == nil || int64(f.UncompressedSize64) > maxAttachment || int64(f.UncompressedSize64) > budget {
				rep.MissingFiles++
				continue
			}
			if err := s.files.Room(); err != nil {
				return wsID, name, err
			}
			key, size, err := s.saveFile(f)
			if errors.Is(err, files.ErrTooLarge) || errors.Is(err, zip.ErrChecksum) || errors.Is(err, zip.ErrAlgorithm) || errors.Is(err, zip.ErrFormat) {
				rep.MissingFiles++
				continue
			}
			if err != nil {
				return wsID, name, err
			}
			budget -= size
			fields := map[string]any{}
			for k, v := range row.Fields {
				fields[k] = v
			}
			fields["storage_key"], fields["size"] = key, size
			row.Fields = fields
			rep.Files++
		}
		rows = append(rows, row)
	}

	restored, err := s.sync.Restore(ctx, wsID, rows, content.Activity, syncer.RestoreOptions{
		FreshIDs: !im.KeepIds, Owner: im.UserID, Exporter: m.ExportedBy.ID,
		From: m.Workspace.ID, Versions: content.Versions,
		// Nothing in an export proves who wrote what (anyone can make
		// one), so work credited to someone else becomes the importer's,
		// or naming a person would let anyone put words in their mouth and
		// learn their email from the export of the result. A deleted
		// account's work stays theirs: it shows only as "Deleted user".
		Keep: func(ctx context.Context, id string) (bool, error) {
			u, err := s.st.GetUser(ctx, id)
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return err == nil && u.DeletedAt.Valid, err
		},
	})
	rep.RestoreReport = restored
	return wsID, name, err
}

// saveFile copies one file out of the zip into the attachment store.
func (s *Service) saveFile(f *zip.File) (string, int64, error) {
	r, err := f.Open()
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = r.Close() }()
	key, size, _, err := s.files.Save(r, maxAttachment)
	return key, size, err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}
