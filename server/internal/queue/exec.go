package queue

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

// outputMax is the most of a command's output kept: more than a comment
// holds, so a long answer is cut where the comment says so.
const outputMax = 1 << 20

// Command is a Work that runs a program for each job, never through a
// shell. The task is on its standard input, as Markdown or, with asJSON,
// as get-task's JSON; MILA_TASK_ID, MILA_TASK_TITLE, MILA_BATCH and
// MILA_WORKSPACE are set beside the rest of the environment, so `mila mcp`
// or `mila api` run inside it write in the job's batch. What it writes to
// standard output is said on the task; its standard error goes to stderr,
// never into Mila. Exiting with anything but 0 means it didn't do the job.
func Command(argv []string, asJSON bool, stderr io.Writer) Work {
	return func(ctx context.Context, job Job) (string, error) {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = append(os.Environ(), "MILA_TASK_ID="+job.ID, "MILA_TASK_TITLE="+job.Title,
			"MILA_BATCH="+job.Batch, "MILA_WORKSPACE="+job.Workspace)
		if asJSON {
			cmd.Stdin = bytes.NewReader(job.Whole)
		} else {
			cmd.Stdin = bytes.NewReader([]byte(job.Text))
		}
		out := &capped{max: outputMax}
		cmd.Stdout, cmd.Stderr = out, stderr
		// A program that leaves children holding its output is waited for
		// only so long after it ends or is stopped.
		cmd.WaitDelay = 10 * time.Second
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return out.String(), nil
	}
}

// capped keeps the first max bytes written to it and drops the rest,
// without ever refusing a write, so the program is never cut off.
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
