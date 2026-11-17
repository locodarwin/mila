package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/datagen"
	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// The server's budgets, on a generated
// 5,000-task workspace: the API's 95th percentile under 50 ms, and the
// server's part of a first sync (the bootstrap) well inside the 2 s the
// whole first sync may take. They run when MILA_BUDGETS is set (`make
// budgets`, and CI on both databases), since they take a while. Shared
// CI runners hold them at a multiple of the budget (MILA_BUDGET_SCALE, as
// the browser's budgets do); `make budgets` holds them to the budget.
const (
	apiP95Budget    = 50 * time.Millisecond
	bootstrapBudget = time.Second
	budgetTasks     = 5000
	// Requests per kind, and how many are in flight at once.
	budgetRounds      = 200
	budgetConcurrency = 4
)

func TestBudgets(t *testing.T) {
	if os.Getenv("MILA_BUDGETS") == "" {
		t.Skip("set MILA_BUDGETS=1 to measure the server's budgets")
	}
	scale := 1.0
	if s := os.Getenv("MILA_BUDGET_SCALE"); s != "" {
		var err error
		if scale, err = strconv.ParseFloat(s, 64); err != nil || scale < 1 {
			t.Fatalf("MILA_BUDGET_SCALE %q: want a number of at least 1", s)
		}
	}
	apiP95Budget := time.Duration(float64(apiP95Budget) * scale)
	bootstrapBudget := time.Duration(float64(bootstrapBudget) * scale)
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		gen, err := datagen.Generate(ctx, s.st, s.sync, datagen.Spec{Email: "shawn@example.com", Tasks: budgetTasks, Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		ws := gen.WorkspaceID
		t.Logf("generated %d rows", gen.Ops)
		// A second workspace as large, so a query that scans every
		// workspace's rows rather than one's shows in the REST lists.
		other, err := datagen.Generate(ctx, s.st, s.sync, datagen.Spec{Email: "dana@example.com", Tasks: budgetTasks, Seed: 2})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("generated %d rows in a second workspace", other.Ops)

		// The bootstrap, as the browser asks for it (gzip, which Go's
		// client asks for and undoes by itself).
		var snap struct {
			Seq   int64  `json:"seq"`
			Epoch string `json:"epoch"`
			Rows  []struct {
				Entity string         `json:"entity"`
				ID     string         `json:"id"`
				Fields map[string]any `json:"fields"`
			} `json:"rows"`
		}
		var times []time.Duration
		for range 5 {
			start := time.Now()
			resp, err := owner.http.Get(owner.base + "/workspaces/" + ws + "/sync/bootstrap?" + syncQ)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 || !resp.Uncompressed {
				t.Fatalf("bootstrap = %d, compressed %v", resp.StatusCode, resp.Uncompressed)
			}
			if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			times = append(times, time.Since(start))
		}
		slices.Sort(times)
		boot := times[len(times)/2]
		size := bootstrapWireSize(t, owner, ws)
		t.Logf("bootstrap: %d rows, median %v (of %v), %.1f MB as JSON, %.2f MB gzipped",
			len(snap.Rows), boot.Round(time.Millisecond), times, float64(jsonSize(t, owner, ws))/1e6, float64(size)/1e6)
		if boot > bootstrapBudget {
			t.Errorf("bootstrap took %v, budget %v", boot, bootstrapBudget)
		}

		var tasks []string
		for _, r := range snap.Rows {
			if r.Entity == "task" {
				tasks = append(tasks, r.ID)
			}
		}
		seq := snap.Seq
		node := uuid.NewString()[:8]
		var mu sync.Mutex
		clock := time.Now().UnixMilli()
		hlc := func() string {
			mu.Lock()
			defer mu.Unlock()
			clock = max(clock+1, time.Now().UnixMilli())
			return fmt.Sprintf("%013d-%04x-%s", clock, 0, node)
		}

		// The requests an open app makes, each budgetRounds times.
		kinds := []struct {
			name string
			do   func(i int) (int, string)
		}{
			{"auth state", func(int) (int, string) { return owner.get("/auth/state") }},
			{"workspaces", func(int) (int, string) { return owner.get("/workspaces") }},
			{"members", func(int) (int, string) { return owner.get("/workspaces/" + ws + "/members") }},
			{"pull, nothing new", func(int) (int, string) {
				return owner.get(fmt.Sprintf("/workspaces/%s/sync/changes?"+syncQ+"&after=%d&epoch=%s", ws, seq, snap.Epoch))
			}},
			{"pull, a few changes", func(int) (int, string) {
				return owner.get(fmt.Sprintf("/workspaces/%s/sync/changes?"+syncQ+"&after=%d&epoch=%s", ws, seq-20, snap.Epoch))
			}},
			{"push, 1 to 5 ops", func(i int) (int, string) {
				var ops []syncer.Op
				for j := range 1 + i%5 {
					task := tasks[(i*7+j)%len(tasks)]
					ops = append(ops, syncer.Op{OpID: uuid.NewString(), Entity: "task", ID: task, HLC: hlc(),
						Fields: map[string]json.RawMessage{"title": json.RawMessage(fmt.Sprintf("%q", fmt.Sprintf("Edited %d.%d", i, j)))}})
				}
				return owner.send("POST", "/workspaces/"+ws+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": ops})
			}},
			{"task history", func(i int) (int, string) {
				return owner.get("/workspaces/" + ws + "/tasks/" + tasks[i%len(tasks)] + "/activity")
			}},
			{"notifications", func(int) (int, string) { return owner.get("/me/notifications") }},
			// The REST endpoints a script calls.
			{"REST list tasks", func(i int) (int, string) {
				return owner.get(fmt.Sprintf("/workspaces/%s/tasks?limit=100&after=%s", ws, tasks[i%len(tasks)]))
			}},
			{"REST get task", func(i int) (int, string) {
				return owner.get("/workspaces/" + ws + "/tasks/" + tasks[(i*13)%len(tasks)])
			}},
			{"REST patch task", func(i int) (int, string) {
				return owner.send("PATCH", "/workspaces/"+ws+"/tasks/"+tasks[(i*17)%len(tasks)], map[string]any{"title": fmt.Sprintf("Patched %d", i)})
			}},
			{"trash", func(int) (int, string) { return owner.get("/workspaces/" + ws + "/trash") }},
			// What an agent asks most: a task by its ID.
			{"action get-task", func(i int) (int, string) {
				return owner.get("/workspaces/" + ws + "/actions/get-task?task=" + tasks[(i*19)%len(tasks)])
			}},
		}
		var all []time.Duration
		for _, k := range kinds {
			got := make([]time.Duration, budgetRounds)
			next := make(chan int)
			var wg sync.WaitGroup
			var failed sync.Once
			for range budgetConcurrency {
				wg.Go(func() {
					for i := range next {
						start := time.Now()
						code, body := k.do(i)
						got[i] = time.Since(start)
						if code != 200 {
							failed.Do(func() { t.Errorf("%s = %d %s", k.name, code, body) })
						}
					}
				})
			}
			for i := range budgetRounds {
				next <- i
			}
			close(next)
			wg.Wait()
			p := p95(got)
			t.Logf("%-20s p50 %6.2f ms  p95 %6.2f ms", k.name, ms(p50(got)), ms(p))
			if p > apiP95Budget {
				t.Errorf("%s: p95 %v, budget %v", k.name, p, apiP95Budget)
			}
			all = append(all, got...)
		}
		p := p95(all)
		t.Logf("%-20s p50 %6.2f ms  p95 %6.2f ms", "all", ms(p50(all)), ms(p))
		if p > apiP95Budget {
			t.Errorf("API p95 %v, budget %v", p, apiP95Budget)
		}

		// find-tasks, asked in turn as it is one at a time for each person:
		// words, nothing, the queue's question, a saved view and a kind. A
		// view or a kind is checked a page at a time in order, so each reads
		// only as far as its answer.
		for _, find := range []struct{ name, query string }{
			{"words", "words="},
			{"nothing", ""},
			{"queue", "assignee=me&kind=todo"},
			{"view", "view=Open+work"},
			{"kind", "kind=todo&state=all"},
		} {
			var finds []time.Duration
			for i := range 20 {
				q := find.query
				if find.name == "words" {
					q += fmt.Sprint(i)
				}
				start := time.Now()
				if code, body := owner.get("/workspaces/" + ws + "/actions/find-tasks?" + q); code != 200 {
					t.Fatalf("find-tasks, %s = %d %s", find.name, code, body)
				}
				finds = append(finds, time.Since(start))
			}
			p := p95(finds)
			t.Logf("%-20s p50 %6.2f ms  p95 %6.2f ms", "find-tasks, "+find.name, ms(p50(finds)), ms(p))
			if p > apiP95Budget {
				t.Errorf("find-tasks, %s: p95 %v, budget %v", find.name, p, apiP95Budget)
			}
		}
	})
}

// find-tasks at its worst, on a generated workspace of MILA_FIND_TASKS
// tasks: searches SQL can't narrow, which read the whole workspace. It
// only measures, and says how long each took.
func TestFindTasksAtScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("MILA_FIND_TASKS"))
	if n <= 0 {
		t.Skip("set MILA_FIND_TASKS to a number of tasks to time find-tasks' worst cases")
	}
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		gen, err := datagen.Generate(ctx, s.st, s.sync, datagen.Spec{Email: "shawn@example.com", Tasks: n, Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{"words=zzqx", "words=%C3%A9", "kind=canceled&state=all", "kind=canceled&state=all&limit=500", "state=all&limit=500"} {
			var times []time.Duration
			body := ""
			for range 3 {
				start := time.Now()
				code, failed := owner.get("/workspaces/" + gen.WorkspaceID + "/actions/find-tasks?" + q)
				if code != 200 {
					t.Fatalf("%s = %d %s", q, code, failed)
				}
				times = append(times, time.Since(start))
				_, out := owner.do("GET", "/workspaces/"+gen.WorkspaceID+"/actions/find-tasks?"+q, nil)
				items, _ := out["items"].([]any)
				body = fmt.Sprintf("%d items, more %v, next %v", len(items), out["more"], out["next"] != nil)
			}
			slices.Sort(times)
			t.Logf("%d tasks, %-38s median %v of %v (%s)", n, q, times[1].Round(time.Millisecond), times, body)
			// The whole search, going on from each answer's next (none
			// before next was given) until no more, or 1,000 tasks.
			start, answers, items, after := time.Now(), 0, 0, ""
			for answers < 200 && items < 1000 {
				path := "/workspaces/" + gen.WorkspaceID + "/actions/find-tasks?" + q
				if after != "" {
					path += "&after=" + after
				}
				_, out := owner.do("GET", path, nil)
				answers++
				list, _ := out["items"].([]any)
				items += len(list)
				next, _ := out["next"].(string)
				if next == "" {
					break
				}
				after = next
			}
			t.Logf("%d tasks, %-38s through to the end: %v in %d answers, %d items", n, q, time.Since(start).Round(time.Millisecond), answers, items)
		}
	})
}

// get and send make one request and return its status and, when it
// failed, its body.
func (c *client) get(path string) (int, string) { return c.send("GET", path, nil) }

func (c *client) send(method, path string, body any) (int, string) {
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = strings.NewReader(string(raw))
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.origin)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return resp.StatusCode, string(raw)
	}
	return 200, ""
}

// bootstrapWireSize is the bootstrap's size as sent to a browser.
func bootstrapWireSize(t *testing.T, c *client, ws string) int64 {
	return rawBootstrap(t, c, ws, "gzip")
}

// jsonSize is the bootstrap's size uncompressed.
func jsonSize(t *testing.T, c *client, ws string) int64 {
	return rawBootstrap(t, c, ws, "identity")
}

func rawBootstrap(t *testing.T, c *client, ws, encoding string) int64 {
	t.Helper()
	req, _ := http.NewRequest("GET", c.base+"/workspaces/"+ws+"/sync/bootstrap?"+syncQ, nil)
	req.Header.Set("Accept-Encoding", encoding)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Content-Encoding"); (encoding == "gzip") != (got == "gzip") {
		t.Fatalf("bootstrap asked for %s came as %q", encoding, got)
	}
	return n
}

func quantile(d []time.Duration, q float64) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*q))]
}

func p50(d []time.Duration) time.Duration { return quantile(d, 0.50) }
func p95(d []time.Duration) time.Duration { return quantile(d, 0.95) }

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
