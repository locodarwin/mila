package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for an agent's command.
func TestMain(m *testing.M) {
	if os.Getenv("MILA_QUEUE_TEST_AGENT") == "1" {
		in, _ := io.ReadAll(os.Stdin)
		fmt.Printf("%s|%s|%s|%s|%s", os.Getenv("MILA_TASK_ID"), os.Getenv("MILA_BATCH"), os.Getenv("MILA_WORKSPACE"), os.Getenv("MILA_TASK_TITLE"), in)
		if strings.Contains(string(in), "fail") {
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestACommandGetsTheJobAndSaysWhatItDid(t *testing.T) {
	t.Setenv("MILA_QUEUE_TEST_AGENT", "1")
	job := Job{ID: "t1", Title: "Mend $HOME; rm -rf", Batch: "queue-1", Workspace: "w1", Whole: json.RawMessage(`{"task":{}}`), Text: "# Mend"}
	said, err := Command([]string{os.Args[0]}, false, io.Discard)(context.Background(), job)
	if err != nil || said != "t1|queue-1|w1|Mend $HOME; rm -rf|# Mend" {
		t.Fatalf("said %q, %v", said, err)
	}
	if said, _ := Command([]string{os.Args[0]}, true, io.Discard)(context.Background(), job); !strings.HasSuffix(said, `{"task":{}}`) {
		t.Fatalf("with JSON, said %q", said)
	}
	job.Text = "fail"
	if _, err := Command([]string{os.Args[0]}, false, io.Discard)(context.Background(), job); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("a failing command: %v", err)
	}
}

func TestCapped(t *testing.T) {
	c := &capped{max: 5}
	for _, s := range []string{"abc", "defg", "h"} {
		if n, err := c.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("write %q = %d %v", s, n, err)
		}
	}
	if c.String() != "abcde" {
		t.Fatalf("kept %q", c.String())
	}
	if got := cut(strings.Repeat("é", 30), 20); len([]rune(got)) != 20 || !strings.HasSuffix(got, "(cut short)") {
		t.Fatalf("cut = %q", got)
	}
}

func TestMarkdown(t *testing.T) {
	raw := `{"task":{"title":"Fence","notes":"North side","priority":"high","due_date":"2026-11-13","due_time":"09:00","status":{"name":"To do"},"project":{"name":"Garden"},
		"labels":[{"name":"Outdoor"}],"assignees":[{"name":"Agent"}]},"checklist":[{"text":"Posts","done":true},{"text":"Wire","done":false}],
		"comments":[{"author":{"name":"Shawn"},"body":"Use the old wire.","created_at":"2026-11-06T10:00:00Z"}],"subtasks":[{"title":"Gate","completed_at":null}]}`
	got := Markdown(json.RawMessage(raw))
	for _, want := range []string{"# Fence\n", "- Project: Garden\n", "- Priority: high\n", "- Due: 2026-11-13 09:00\n", "- Labels: Outdoor\n",
		"## Notes\n\nNorth side\n", "- [x] Posts\n- [ ] Wire\n", "## Subtasks\n\n- [ ] Gate\n", "Shawn, 2026-11-06 10:00:\n\nUse the old wire.\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(Markdown(json.RawMessage(`{"task":{"title":"x","priority":"none"}}`)), "the Inbox") {
		t.Error("an Inbox task doesn't say so")
	}
}
