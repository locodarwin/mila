package ownership_test

import (
	"crypto/rand"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/ownership"
)

// An import's files may take twice what its upload does, whatever the
// upload limit: a small zip of a file that compresses to nothing brings
// in none of it, and a file that does not compress comes in.
func TestImportFilesAreBoundByTheUpload(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		f := newFixture(t, d, 64<<20)
		owner := f.users["owner"]
		f.fill()
		z := f.export("owner", ownership.JSON)
		name := "attachments/" + f.id("file") + "/notes.txt"
		stored := func() int64 {
			var n int64
			must(t, filepath.WalkDir(filepath.Join(f.dir, "attachments"), func(_ string, e fs.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return err
				}
				info, err := e.Info()
				n += info.Size()
				return err
			}))
			return n
		}
		report := func(im ownership.Import) ownership.Report {
			t.Helper()
			if im.Status != "ready" {
				t.Fatalf("import %+v", im)
			}
			var r ownership.Report
			must(t, json.Unmarshal([]byte(im.Report), &r))
			return r
		}

		zeros := zipBytes(t, z, map[string]string{name: strings.Repeat("\x00", 20<<20)})
		if len(zeros) > 256<<10 {
			t.Fatalf("the test zip is %d KB", len(zeros)>>10)
		}
		before := stored()
		im, err := f.importZip(owner, zeros, false)
		must(t, err)
		if r := report(im); r.Files != 0 || r.MissingFiles != 1 {
			t.Errorf("a %d KB zip of 20 MB of zeros brought in %d files, left out %d", len(zeros)>>10, r.Files, r.MissingFiles)
		}
		if grew := stored() - before; grew > 1<<20 {
			t.Errorf("a %d KB zip wrote %d KB of files", len(zeros)>>10, grew>>10)
		}

		noise := make([]byte, 2<<20)
		_, err = rand.Read(noise)
		must(t, err)
		im, err = f.importZip(owner, zipBytes(t, z, map[string]string{name: string(noise)}), false)
		must(t, err)
		if r := report(im); r.Files != 1 || r.MissingFiles != 0 {
			t.Errorf("2 MB that does not compress: brought in %d files, left out %d", r.Files, r.MissingFiles)
		}
	})
}
