package syncer_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// A page keeps its newest 500 versions, and of those no more than
// maxHistory bytes, however often it is written whole; the newest stays
// even when it alone is larger.
func TestAPagesHistoryIsBounded(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newWorld(t, d)
		start := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
		now := start
		w.svc.SetNow(func() time.Time { return now })
		project, _ := w.project("House")
		main := syncer.MainPageID(project)
		w.push("owner", op("page", main, hlcAt(now, "a"), newPage(project, nil, "")))
		write := func(i int, body string) {
			t.Helper()
			now = start.Add(time.Duration(i) * time.Second)
			wantStatus(t, w.push("owner", op("page", main, hlcAt(now, "a"), map[string]any{"body": body}))[0], "applied")
		}
		history := func() []syncer.PageVersion {
			t.Helper()
			v, err := w.svc.PageVersions(context.Background(), w.users["owner"], w.ws, main)
			must(t, err)
			return v
		}
		for i := range 520 {
			write(i, fmt.Sprintf("Version %d", i))
		}
		var kept int
		must(t, d.QueryRowContext(context.Background(), rebind(d, "SELECT COUNT(*) FROM page_versions WHERE page_id = ?"), main).Scan(&kept))
		if v := history(); kept != 500 || v[0].Body != "Version 519" || v[499].Body != "Version 20" {
			t.Fatalf("kept %d, newest %q, oldest %q", kept, v[0].Body, v[len(v)-1].Body)
		}

		syncer.SetMaxHistory(t, 10_000)
		big := strings.Repeat("x", 3_000)
		for i := range 5 {
			write(600+i, fmt.Sprintf("%s %d", big, i))
		}
		v := history()
		if len(v) != 3 || !strings.HasSuffix(v[0].Body, " 4") || !strings.HasSuffix(v[2].Body, " 2") {
			t.Fatalf("with 10 KB of history kept %d versions, newest ending %q", len(v), v[0].Body[len(v[0].Body)-2:])
		}
		write(700, strings.Repeat("y", 20_000))
		if v := history(); len(v) != 1 || len(v[0].Body) != 20_000 {
			t.Fatalf("a version larger than the whole cap: kept %d", len(v))
		}
	})
}
