package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/locodarwin/mila/server/internal/milaclient"
	"github.com/locodarwin/mila/server/internal/queue"
)

const queueHelp = `Usage: mila queue [flags] -- <command> [arguments]

Works through the tasks assigned to the token's person, one at a time: each
open task in a to-do status is moved to in progress, given to the command,
and finished once it exits with 0, with what it wrote to standard output as
a comment. A job that fails or takes too long is put back with a comment
saying so, and the person is taken off it. The command is never run
through a shell.

The command reads the task on standard input (Markdown, or get-task's JSON
with --json) and finds MILA_TASK_ID, MILA_TASK_TITLE, MILA_BATCH and
MILA_WORKSPACE in its environment, so "mila mcp" or "mila api" inside it
write in the job's batch: "mila api undo <batch>" puts a whole job back.

Whoever writes to a task the agent is given instructs it, and reads its
answer, which is posted on the task. So the queue takes only tasks that the
workspace's members alone have written to (the task, its notes, checklist
and comments, and its subtasks, in their histories, with nobody else
assigned): one a guest or an email wrote to, or with a history too long to
check, is handed back with a comment saying why. It checks again when the
job ends, and posts nothing if someone else wrote meanwhile, so tell the
agent to work from the task it is given rather than read it again. --from
narrows that to the people named; --anyone takes every task, whoever wrote
it. Give the agent's account the narrowest role that works, and the agent
only the tools the work needs.

`

// queueRun runs `mila queue`.
func queueRun(args []string) error {
	fs := flag.NewFlagSet("queue", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), queueHelp+clientHelp+"\n\nFlags:\n")
		fs.PrintDefaults()
	}
	o := clientFlags(fs)
	var opts queue.Options
	var asJSON bool
	fs.StringVar(&opts.Then, "then", "complete", "what a task becomes once its job worked: complete, or a status of its project by name")
	fs.DurationVar(&opts.Every, "every", time.Minute, "how long to wait between looks at the queue")
	fs.BoolVar(&opts.Once, "once", false, "work through the queue once and stop, as from cron")
	fs.DurationVar(&opts.Timeout, "timeout", 30*time.Minute, "the longest one job may take")
	fs.BoolVar(&asJSON, "json", false, "give the command get-task's JSON rather than Markdown")
	from := fs.String("from", "", "take only tasks these people alone wrote to: names or user IDs, separated by commas (by default, the workspace's members)")
	fs.BoolVar(&opts.Anyone, "anyone", false, "take every task assigned, whoever wrote to it, guests and email senders included")
	if err := fs.Parse(args); err != nil {
		return err
	}
	command := fs.Args()
	if len(command) == 0 {
		fs.Usage()
		return errors.New("no command given: put it after --")
	}
	for name := range strings.SplitSeq(*from, ",") {
		if name = strings.TrimSpace(name); name != "" {
			opts.From = append(opts.From, name)
		}
	}
	if opts.Anyone && len(opts.From) > 0 {
		return errors.New("--anyone and --from don't go together")
	}
	if opts.Every < 10*time.Second {
		return errors.New("--every is 10s at least")
	}
	ctx, stop := signalled()
	defer stop()
	warnCleartext(o.Server)
	c, err := milaclient.Open(ctx, *o)
	if err != nil {
		return err
	}
	opts.Log = os.Stderr
	_, _ = fmt.Fprintf(os.Stderr, "mila queue: working in %q, every %s\n", c.Workspace.Name, opts.Every)
	return queue.Run(ctx, c, opts, queue.Command(command, asJSON, os.Stderr))
}
