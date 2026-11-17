// Package ownership lets people take their data with them and bring it
// back: exports of a workspace as JSON (complete, with attachments, and
// restorable), CSV and Markdown, and imports of a JSON export into a new
// workspace. Both are slow jobs (jobs.SlowLane), so a large one never
// delays a reminder; the export zips and the uploads waiting to be
// imported live in folders of their own beside the attachments, and
// expire.
package ownership

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/apperr"
	"github.com/locodarwin/mila/server/internal/authz"
	"github.com/locodarwin/mila/server/internal/files"
	"github.com/locodarwin/mila/server/internal/jobs"
	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/store"
	"github.com/locodarwin/mila/server/internal/store/sqlitedb"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// Formats of export.
const (
	JSON     = "json"
	CSV      = "csv"
	Markdown = "markdown"
)

// Formats lists the formats of export.
var Formats = []string{JSON, CSV, Markdown}

const (
	// ExportLifetime is how long a finished export can be downloaded.
	ExportLifetime = 7 * 24 * time.Hour
	// keepImports is how long the record of a finished import stays.
	keepImports = 30 * 24 * time.Hour
	// maxUnfinished is how many exports (or imports) one person may have
	// waiting or running at once.
	maxUnfinished = 3
	// maxKept is how many exports one person may have on the way or ready
	// at once: each is a copy of a workspace, its files included, kept for
	// a week.
	maxKept = 10
	// grace spares a file that may be being written from the sweep.
	grace = time.Hour

	jobExport = jobs.SlowLane + "export"
	jobImport = jobs.SlowLane + "import"
)

// Informer tells someone in the app that what they asked for is done.
type Informer interface {
	Inform(ctx context.Context, userID, workspaceID string, kind notify.Kind, title, body string) error
}

// Events hears about workspaces an import adds.
type Events interface {
	MembershipChanged(userIDs ...string)
}

// Config is where the service keeps its files and how large an upload
// it takes.
type Config struct {
	// DataDir holds the exports and imports folders.
	DataDir string
	// ImportMax is the largest upload an import accepts, in bytes.
	ImportMax int64
}

// Service makes exports and runs imports.
type Service struct {
	st      *store.Store
	log     *slog.Logger
	sync    *syncer.Service
	files   *files.Store
	jobs    *jobs.Runner
	tell    Informer
	events  Events
	exports string // the folder of export zips
	imports string // the folder of uploads waiting to be imported
	max     int64
	// uploading holds the people with an import upload being read.
	uploading sync.Map
}

// New builds the service, making its folders, and registers its jobs and
// its hourly tidy with the runner.
func New(st *store.Store, log *slog.Logger, sync *syncer.Service, attachments *files.Store, runner *jobs.Runner,
	tell Informer, events Events, cfg Config) (*Service, error) {
	s := &Service{st: st, log: log, sync: sync, files: attachments, jobs: runner, tell: tell, events: events,
		exports: filepath.Join(cfg.DataDir, "exports"), imports: filepath.Join(cfg.DataDir, "imports"), max: cfg.ImportMax}
	for _, dir := range []string{s.exports, s.imports} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	runner.HandleSlow(jobExport, s.handleExport)
	runner.HandleSlow(jobImport, s.handleImport)
	runner.Every("exports", time.Hour, s.Tidy)
	return s, nil
}

// ImportMax is the largest upload an import accepts, in bytes.
func (s *Service) ImportMax() int64 { return s.max }

func (s *Service) now() time.Time { return s.jobs.Now() }

// Export is one export as its owner sees it.
type Export = sqlitedb.ListExportsRow

// StartExport queues an export of what the user sees of a workspace.
func (s *Service) StartExport(ctx context.Context, userID, workspaceID, format string) (Export, error) {
	if !known(format) {
		return Export{}, apperr.New(apperr.Invalid, "%q is not a format Mila exports; choose json, csv or markdown.", format)
	}
	ws, err := s.member(ctx, userID, workspaceID, authz.ExportWorkspace)
	if err != nil {
		return Export{}, err
	}
	n, err := s.st.CountUnfinishedExports(ctx, userID)
	if err != nil {
		return Export{}, err
	}
	if n >= maxUnfinished {
		return Export{}, apperr.New(apperr.Conflict, "You have %d exports on the way already. Wait for one to finish.", n)
	}
	now := s.now()
	kept, err := s.st.CountKeptExports(ctx, sqlitedb.CountKeptExportsParams{UserID: userID, Now: now})
	if err != nil {
		return Export{}, err
	}
	if kept >= maxKept {
		return Export{}, apperr.New(apperr.Conflict, "You have %d exports kept already. Delete one you have downloaded, or wait for one to expire.", kept)
	}
	if err := files.CheckRoom(s.exports); err != nil {
		return Export{}, err
	}
	ext := map[string]string{JSON: "json", CSV: "csv", Markdown: "markdown"}[format]
	e := Export{
		ID: uuid.Must(uuid.NewV7()).String(), UserID: userID, WorkspaceID: workspaceID, Format: format, Status: "queued",
		FileName:  fmt.Sprintf("mila-%s-%s-%s.zip", fileSlug(ws.Name), ext, now.Format("2006-01-02")),
		CreatedAt: now, ExpiresAt: now.Add(ExportLifetime), WorkspaceName: ws.Name,
	}
	err = s.st.InTx(ctx, func(q store.Queries) error {
		if err := q.InsertExport(ctx, sqlitedb.InsertExportParams{
			ID: e.ID, UserID: userID, WorkspaceID: workspaceID, Format: format, FileName: e.FileName, Now: now, ExpiresAt: e.ExpiresAt,
		}); err != nil {
			return err
		}
		return jobs.Enqueue(ctx, q, jobExport, ref{ID: e.ID}, now, now)
	})
	if err != nil {
		return Export{}, err
	}
	s.jobs.Wake()
	return e, nil
}

// Exports lists the user's exports, newest first.
func (s *Service) Exports(ctx context.Context, userID string) ([]Export, error) {
	return s.st.ListExports(ctx, userID)
}

// OpenExport opens a finished export's zip for its owner.
func (s *Service) OpenExport(ctx context.Context, userID, id string) (*os.File, sqlitedb.Export, error) {
	e, err := s.st.GetExport(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (e.UserID != userID || e.ExpiresAt.Before(s.now()))) {
		return nil, e, apperr.New(apperr.NotFound, "No such export. Exports can be downloaded for 7 days.")
	}
	if err != nil {
		return nil, e, err
	}
	if e.Status != "ready" {
		return nil, e, apperr.New(apperr.Conflict, "This export is not ready yet.")
	}
	// Made while they were a member; no longer theirs once they are not.
	if _, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: e.WorkspaceID, UserID: userID}); errors.Is(err, sql.ErrNoRows) {
		return nil, e, apperr.New(apperr.NotFound, "No such export. Exports can be downloaded only by members of the workspace.")
	} else if err != nil {
		return nil, e, err
	}
	f, err := os.Open(s.exportPath(e.ID))
	return f, e, err
}

// DeleteExport removes one of the user's exports and its file.
func (s *Service) DeleteExport(ctx context.Context, userID, id string) error {
	n, err := s.st.DeleteExport(ctx, sqlitedb.DeleteExportParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(apperr.NotFound, "No such export.")
	}
	if err := os.Remove(s.exportPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Warn("remove export file", "export", id, "err", err)
	}
	return nil
}

func (s *Service) exportPath(id string) string { return filepath.Join(s.exports, id+".zip") }
func (s *Service) importPath(id string) string { return filepath.Join(s.imports, id+".zip") }

// member loads a workspace the user belongs to, checking the role allows
// the action. Anyone else gets NotFound.
func (s *Service) member(ctx context.Context, userID, workspaceID string, action authz.Action) (store.Workspace, error) {
	ws, err := s.st.GetWorkspace(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return ws, apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return ws, err
	}
	m, err := s.st.GetMembership(ctx, sqlitedb.GetMembershipParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		return ws, apperr.New(apperr.NotFound, "No such workspace.")
	}
	if err != nil {
		return ws, err
	}
	if !authz.Can(authz.Role(m.Role), action) {
		return ws, apperr.New(apperr.Forbidden, "Your role in this workspace does not allow that.")
	}
	return ws, nil
}

// Tidy removes exports past their lifetime, old import records, and any
// file in the two folders that no unexpired record names. The runner does
// it hourly.
func (s *Service) Tidy(ctx context.Context) error {
	now := s.now()
	if _, err := s.st.DeleteExpiredExports(ctx, now); err != nil {
		return err
	}
	if _, err := s.st.DeleteOldImports(ctx, sql.NullTime{Time: now.Add(-keepImports), Valid: true}); err != nil {
		return err
	}
	exports, err := s.st.ListExportIDs(ctx)
	if err != nil {
		return err
	}
	if err := sweep(s.exports, exports); err != nil {
		return err
	}
	imports, err := s.st.ListImportIDs(ctx)
	if err != nil {
		return err
	}
	return sweep(s.imports, imports)
}

// sweep deletes the files in dir that none of ids names (by "<id>.zip" or
// a temporary file of that ID), sparing those written in the last hour.
func sweep(dir string, ids []string) error {
	keep := map[string]bool{}
	for _, id := range ids {
		keep[id] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		id := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "."), ".zip")
		if i := strings.Index(id, ".tmp"); i >= 0 {
			id = id[:i]
		}
		info, err := e.Info()
		if err != nil || keep[id] || time.Since(info.ModTime()) < grace {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	return nil
}

type ref struct {
	ID string `json:"id"`
}

func known(format string) bool {
	for _, f := range Formats {
		if f == format {
			return true
		}
	}
	return false
}

// message is what a person is told about a failure: the error's own words
// when it is meant for them, otherwise a plain apology (the details go to
// the log).
func message(err error, fallback string) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return fallback
}
