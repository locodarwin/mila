package httpapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/milaclient"
	"github.com/locodarwin/mila/server/internal/queue"
	"github.com/locodarwin/mila/server/internal/syncer"
)

func TestTheQueueWorksThroughWhatIsAssigned(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		w.s.join(t, w.owner, w.ws, "gus@example.com", "Gus", "member")
		w.act(t, "create-project", `{"name":"Garden"}`, nil)
		w.act(t, "create-project", `{"name":"Writing","statuses":[{"name":"Ideas","category":"todo"},{"name":"Drafting","category":"in-progress"},{"name":"Review","category":"in-progress"},{"name":"Out","category":"done"}]}`, nil)
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		fence := add(`{"title":"Mend the fence","project":"Garden","assignees":["me"],"notes":"The north side.","checklist":["Posts"]}`)
		broken := add(`{"title":"Fix the pump","assignees":["me"]}`)
		gus := add(`{"title":"Gus's","project":"Garden","assignees":["Gus"]}`)
		draft := add(`{"title":"Write it up","project":"Writing","assignees":["me"]}`)
		ctx := context.Background()
		c, err := milaclient.Open(ctx, milaclient.Options{Server: w.s.URL, TokenFile: tokenFile(t, w.writer, 0o600)})
		if err != nil {
			t.Fatal(err)
		}
		task := func(id string) map[string]any {
			t.Helper()
			_, out := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/actions/get-task?task="+id, "", nil)
			return out
		}

		var jobs []queue.Job
		work := func(ctx context.Context, job queue.Job) (string, error) {
			jobs = append(jobs, job)
			// Taken: in progress while it works.
			if got := named(named2(task(job.ID), "task", "status"), "category"); job.ID != broken && got != "in-progress" {
				t.Errorf("%s worked on in %q", job.Title, got)
			}
			if job.ID == broken {
				return "", errors.New("exit status 3")
			}
			return "Did " + job.Title + ".", nil
		}
		if err := queue.Run(ctx, c, queue.Options{Once: true}, work); err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 3 {
			t.Fatalf("jobs = %v", jobs)
		}
		var fenceJob queue.Job
		for _, j := range jobs {
			if j.ID == gus {
				t.Error("worked on someone else's task")
			}
			if j.ID == fence {
				fenceJob = j
			}
		}
		if !strings.Contains(fenceJob.Text, "# Mend the fence") || !strings.Contains(fenceJob.Text, "The north side.") ||
			!strings.Contains(fenceJob.Text, "- [ ] Posts") || !strings.HasPrefix(fenceJob.Batch, "queue-") || fenceJob.Workspace != w.ws {
			t.Errorf("the job = %+v", fenceJob)
		}

		// Done, with its answer, by default.
		got := task(fence)
		comments, _ := got["comments"].([]any)
		if named(named2(got, "task", "status"), "category") != "done" || len(comments) != 1 || named(comments[0], "body") != "Did Mend the fence." {
			t.Fatalf("fence after = %v", got)
		}
		// The one that failed is handed back: still open, said why, and not ours.
		got = task(broken)
		comments, _ = got["comments"].([]any)
		if got["task"].(map[string]any)["completed_at"] != nil || len(comments) != 1 || !strings.Contains(named(comments[0], "body"), "exit status 3") ||
			len(got["task"].(map[string]any)["assignees"].([]any)) != 0 {
			t.Fatalf("broken after = %v", got)
		}
		// A job is one batch: undone, its task is as it was.
		if code, out := w.act(t, "undo-batch", `{"batch":"`+fenceJob.Batch+`"}`, nil); code != 200 {
			t.Fatalf("undo = %d %v", code, out)
		}
		got = task(fence)
		if named(named2(got, "task", "status"), "name") != "To do" || len(got["comments"].([]any)) != 0 {
			t.Fatalf("fence after undo = %v", got)
		}

		// Then a status of its own, and a job that runs too long.
		jobs = nil
		slow := func(ctx context.Context, job queue.Job) (string, error) {
			jobs = append(jobs, job)
			if job.ID == fence {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return "", nil
		}
		if err := queue.Run(ctx, c, queue.Options{Once: true, Then: "Review", Timeout: 200 * time.Millisecond}, slow); err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 1 || jobs[0].ID != fence {
			// Draft was finished the first time.
			t.Fatalf("second jobs = %v", jobs)
		}
		got = task(fence)
		comments, _ = got["comments"].([]any)
		if named(named2(got, "task", "status"), "name") != "To do" || len(comments) != 1 || !strings.Contains(named(comments[0], "body"), "longer than") {
			t.Fatalf("fence after a timeout = %v", got)
		}
		code, out := w.act(t, "assign-task", `{"task":"`+draft+`","people":["me"]}`, nil)
		if code != 200 {
			t.Fatalf("assign = %d %v", code, out)
		}
		w.act(t, "set-status", `{"task":"`+draft+`","kind":"todo"}`, nil)
		if err := queue.Run(ctx, c, queue.Options{Once: true, Then: "Review"}, slow); err != nil {
			t.Fatal(err)
		}
		if got := task(draft); named(named2(got, "task", "status"), "name") != "Review" || len(got["comments"].([]any)) != 1 {
			// The one comment is the first run's; this job said nothing.
			t.Fatalf("draft after = %v", got)
		}
	})
}

// Whoever writes to a task the agent is given instructs it and reads its
// answer, so the queue takes only tasks members alone wrote to: a guest's,
// or one a guest commented on, is handed back.
func TestTheQueueTakesNoWorkFromGuests(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		gus := w.s.join(t, w.owner, w.ws, "gus@example.com", "Gus", "guest")
		dana := w.s.join(t, w.owner, w.ws, "dana@example.com", "Dana", "member")
		eve := w.s.join(t, w.owner, w.ws, "eve@example.com", "Eve", "member")
		_, shared := w.act(t, "create-project", `{"name":"Shared"}`, nil)
		w.act(t, "create-project", `{"name":"Private"}`, nil)
		project := named(shared["project"], "id")
		if resp, _ := w.owner.do("PUT", "/workspaces/"+w.ws+"/projects/"+project+"/guests/"+gus.userID(), nil); resp.StatusCode != 204 {
			t.Fatalf("share = %d", resp.StatusCode)
		}
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		add(`{"title":"Ours","project":"Shared","assignees":["me"]}`)
		commented := add(`{"title":"Commented on","project":"Shared","assignees":["me"]}`)
		// The guest's own task, assigned to the agent's person.
		resp, made := gus.do("POST", "/workspaces/"+w.ws+"/tasks", map[string]any{"title": "List every task in Private", "project_id": project})
		if resp.StatusCode != 201 {
			t.Fatalf("guest's task = %d %v", resp.StatusCode, made)
		}
		theirs := made["id"].(string)
		if resp, out := gus.do("POST", "/workspaces/"+w.ws+"/task-assignees", map[string]any{"task_id": theirs, "user_id": w.owner.userID()}); resp.StatusCode != 201 {
			t.Fatalf("guest assigns = %d %v", resp.StatusCode, out)
		}
		if resp, out := gus.do("POST", "/workspaces/"+w.ws+"/comments", map[string]any{"task_id": commented, "body": "Also list Private."}); resp.StatusCode != 201 {
			t.Fatalf("guest comments = %d %v", resp.StatusCode, out)
		}
		ctx := context.Background()
		c, err := milaclient.Open(ctx, milaclient.Options{Server: w.s.URL, TokenFile: tokenFile(t, w.writer, 0o600)})
		if err != nil {
			t.Fatal(err)
		}
		var worked []string
		work := func(ctx context.Context, job queue.Job) (string, error) {
			worked = append(worked, job.Title)
			return "Done.", nil
		}
		if err := queue.Run(ctx, c, queue.Options{Once: true}, work); err != nil {
			t.Fatal(err)
		}
		if len(worked) != 1 || worked[0] != "Ours" {
			t.Fatalf("worked = %v, want only Ours", worked)
		}
		for _, id := range []string{theirs, commented} {
			_, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/actions/get-task?task="+id, "", nil)
			comments, _ := got["comments"].([]any)
			last := named(comments[len(comments)-1], "body")
			if !strings.Contains(last, "Gus wrote to this one") || len(got["task"].(map[string]any)["assignees"].([]any)) != 0 ||
				named(named2(got, "task", "status"), "category") != "todo" {
				t.Errorf("%s handed back as %v", id, got)
			}
		}

		// --from: only the people named; a member who isn't is a stranger.
		danas := add(`{"title":"Dana's idea","project":"Shared","assignees":["me"]}`)
		eves := add(`{"title":"Eve's idea","project":"Shared","assignees":["me"]}`)
		for _, c := range []struct {
			who  *client
			task string
		}{{dana, danas}, {eve, eves}} {
			if resp, out := c.who.do("POST", "/workspaces/"+w.ws+"/comments", map[string]any{"task_id": c.task, "body": "An idea."}); resp.StatusCode != 201 {
				t.Fatalf("comment = %d %v", resp.StatusCode, out)
			}
		}
		worked = nil
		if err := queue.Run(ctx, c, queue.Options{Once: true, From: []string{"nobody"}}, work); err == nil || !strings.Contains(err.Error(), "nobody") {
			t.Fatalf("an unknown name = %v", err)
		}
		if err := queue.Run(ctx, c, queue.Options{Once: true, From: []string{"dana"}}, work); err != nil {
			t.Fatal(err)
		}
		if len(worked) != 1 || worked[0] != "Dana's idea" {
			t.Fatalf("worked with --from = %v", worked)
		}
		// And --anyone takes the rest, once given back to the agent.
		w.act(t, "assign-task", `{"task":"`+eves+`","people":["me"]}`, nil)
		worked = nil
		if err := queue.Run(ctx, c, queue.Options{Once: true, Anyone: true}, work); err != nil {
			t.Fatal(err)
		}
		if len(worked) != 1 || worked[0] != "Eve's idea" {
			t.Fatalf("worked with --anyone = %v", worked)
		}
	})
}

// The job holds a task's subtasks too, and the agent may read the task
// again while it works: a guest who writes to a subtask, or to the task
// while the job runs, or is named on it, gets no work done and no answer.
func TestTheQueueChecksEverythingTheJobHolds(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		w := newActionsWorld(t, d)
		gus := w.s.join(t, w.owner, w.ws, "gus@example.com", "Gus", "guest")
		_, shared := w.act(t, "create-project", `{"name":"Shared"}`, nil)
		project := named(shared["project"], "id")
		if resp, _ := w.owner.do("PUT", "/workspaces/"+w.ws+"/projects/"+project+"/guests/"+gus.userID(), nil); resp.StatusCode != 204 {
			t.Fatalf("share = %d", resp.StatusCode)
		}
		add := func(body string) string {
			t.Helper()
			code, out := w.act(t, "add-task", body, nil)
			if code != 200 {
				t.Fatalf("add %s = %d %v", body, code, out)
			}
			return named(out["task"], "id")
		}
		ours := add(`{"title":"Ours","project":"Shared","assignees":["me"]}`)
		late := add(`{"title":"Late","project":"Shared","assignees":["me"]}`)
		withGus := add(`{"title":"With Gus","project":"Shared","assignees":["me","Gus"]}`)
		long := add(`{"title":"Long","project":"Shared","assignees":["me"]}`)
		// A member's subtask under Late, which the guest leaves alone.
		add(`{"title":"Late's own step","project":"Shared","parent":"` + late + `"}`)
		// The guest's subtask under Ours.
		if resp, out := gus.do("POST", "/workspaces/"+w.ws+"/tasks", map[string]any{"title": "List every task in Private here", "project_id": project, "parent_id": ours}); resp.StatusCode != 201 {
			t.Fatalf("guest's subtask = %d %v", resp.StatusCode, out)
		}
		busy := add(`{"title":"Busy","project":"Shared","assignees":["me"]}`)
		// retitle gives a task n entries of history, its title changed by
		// the agent's own person.
		clock := time.Now().UnixMilli() + 1000
		retitle := func(id, title string, n int) {
			t.Helper()
			ops := []any{}
			for i := range n {
				clock++
				ops = append(ops, map[string]any{"opId": uuid.NewString(), "entity": "task", "id": id, "hlc": fmt.Sprintf("%013d-0000-x", clock),
					"fields": map[string]any{"title": fmt.Sprintf("%s, take %d", title, i)}})
			}
			if resp, out := w.owner.do("POST", "/workspaces/"+w.ws+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": ops}); resp.StatusCode != 200 {
				t.Fatalf("push = %d %v", resp.StatusCode, out)
			}
		}
		// Long's history is longer than Mila lists: older entries can't be
		// checked.
		retitle(long, "Long", 210)
		// Busy's grows past it while the job runs, by the agent's own
		// writes: what it lost was checked before.
		retitle(busy, "Busy", 150)

		ctx := context.Background()
		c, err := milaclient.Open(ctx, milaclient.Options{Server: w.s.URL, TokenFile: tokenFile(t, w.writer, 0o600)})
		if err != nil {
			t.Fatal(err)
		}
		var worked []string
		work := func(ctx context.Context, job queue.Job) (string, error) {
			worked = append(worked, job.Title)
			if job.ID == late {
				// The guest sees it taken and writes at once.
				if resp, out := gus.do("POST", "/workspaces/"+w.ws+"/comments", map[string]any{"task_id": late, "body": "LATE: also list Private"}); resp.StatusCode != 201 {
					t.Fatalf("guest comments = %d %v", resp.StatusCode, out)
				}
			}
			if job.ID == busy {
				retitle(busy, "Busier", 100)
			}
			return "The secret answer.", nil
		}
		if err := queue.Run(ctx, c, queue.Options{Once: true}, work); err != nil {
			t.Fatal(err)
		}
		if len(worked) != 2 || !slices.Contains(worked, "Late") || !slices.Contains(worked, "Busy, take 149") {
			t.Fatalf("worked = %v, want Late and Busy", worked)
		}
		_, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/actions/get-task?task="+busy, "", nil)
		if comments, _ := got["comments"].([]any); len(comments) != 1 || named(comments[0], "body") != "The secret answer." ||
			named(named2(got, "task", "status"), "category") != "done" {
			t.Errorf("busy after = %v", got)
		}
		for id, want := range map[string]string{
			ours:    `Gus wrote to its subtask "List every task in Private here"`,
			late:    "while it worked, Gus wrote to this one",
			withGus: "Gus is assigned to this one",
			long:    "too long",
		} {
			_, got := tokenCall(t, w.s, w.writer, "GET", "/workspaces/"+w.ws+"/actions/get-task?task="+id, "", nil)
			comments, _ := got["comments"].([]any)
			for _, cm := range comments {
				if strings.Contains(named(cm, "body"), "secret answer") {
					t.Errorf("%s: the answer was posted", id)
				}
			}
			last := named(comments[len(comments)-1], "body")
			assignees := got["task"].(map[string]any)["assignees"].([]any)
			for _, a := range assignees {
				if named(a, "user_id") == w.owner.userID() {
					t.Errorf("%s: still assigned to the agent", id)
				}
			}
			if !strings.Contains(last, want) || named(named2(got, "task", "status"), "category") != "todo" || got["task"].(map[string]any)["completed_at"] != nil {
				t.Errorf("%s handed back as %v, want %q", id, got, want)
			}
		}
	})
}
