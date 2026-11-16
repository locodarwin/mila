package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"unicode"
	"unicode/utf16"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/locodarwin/mila/server/internal/mcptools"
	"github.com/locodarwin/mila/server/internal/milaclient"
)

// `mila mcp` and `mila api`: a Mila server's API for agents and shells, with
// a personal API token (docs/agents.md).

const clientHelp = `The server is MILA_SERVER or --server. The API token is read from the file
MILA_TOKEN_FILE or --token-file names (or taken from MILA_TOKEN), and that
file must be readable by you alone. The workspace is MILA_WORKSPACE or
--workspace, by name or ID, unless the token reaches only one.`

// clientFlags are the flags both commands take, after the environment.
func clientFlags(fs *flag.FlagSet) *milaclient.Options {
	o := &milaclient.Options{}
	fs.StringVar(&o.Server, "server", os.Getenv("MILA_SERVER"), "Mila's address, such as https://mila.example.com")
	fs.StringVar(&o.TokenFile, "token-file", os.Getenv("MILA_TOKEN_FILE"), "a file holding the API token, readable by you alone")
	fs.StringVar(&o.Workspace, "workspace", os.Getenv("MILA_WORKSPACE"), "the workspace, by name or ID")
	o.Token = os.Getenv("MILA_TOKEN")
	return o
}

func signalled() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// mcpServe runs `mila mcp`: an MCP server on stdin and stdout.
func mcpServe(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), "Usage: mila mcp [flags]\n\nServes Mila's tools to an agent harness over MCP, on stdin and stdout.\n"+clientHelp+"\n\nFlags:")
		fs.PrintDefaults()
	}
	o := clientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signalled()
	defer stop()
	warnCleartext(o.Server)
	c, err := milaclient.Open(ctx, *o)
	if err != nil {
		return err
	}
	// A job that `mila queue` started names its batch, so the agent's
	// writes and the queue's go back together.
	c.Batch = cmp.Or(os.Getenv("MILA_BATCH"), milaclient.NewBatch("mcp"))
	return mcptools.Server(c).Run(ctx, &mcp.StdioTransport{})
}

const apiUsage = `Usage: mila api <command> [flags] [arguments]

Commands:
  whoami                  whom the token acts for, and the workspace
  find [flags]            find tasks (--list inbox|my-day|upcoming, --view, --project,
                          --due-from, --due-to, --state open|done|all, --words,
                          --assignee, --kind, --limit, --after)
  do <action> [json]      run an action, such as add-task or complete-task, with a JSON
                          body as the argument or on stdin; --batch groups it with others
  undo <batch>            put a batch back
  get <path>              read any of the API: /me, or a path in the workspace such as
                          tasks?project_id=<id>

Flags go before the arguments. Answers are JSON, except find's table
without --json. The actions are listed at /api/docs on the server.

`

// apiRun runs `mila api`.
func apiRun(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stdout, apiUsage+clientHelp+"\n")
		if len(args) == 0 {
			return errors.New("no api command given")
		}
		return nil
	}
	cmd := args[0]
	fs := flag.NewFlagSet("api "+cmd, flag.ContinueOnError)
	o := clientFlags(fs)
	var find struct{ list, view, project, dueFrom, dueTo, state, words, assignee, kind, after string }
	var limit int
	var asJSON bool
	var batch string
	switch cmd {
	case "find":
		fs.StringVar(&find.list, "list", "", "inbox, my-day or upcoming")
		fs.StringVar(&find.view, "view", "", "a saved view, by name or ID")
		fs.StringVar(&find.project, "project", "", "a project, by name or ID")
		fs.StringVar(&find.dueFrom, "due-from", "", "due on or after this day")
		fs.StringVar(&find.dueTo, "due-to", "", "due on or before this day")
		fs.StringVar(&find.state, "state", "", "open (the default), done or all")
		fs.StringVar(&find.words, "words", "", "words in the title or notes")
		fs.StringVar(&find.assignee, "assignee", "", "assigned to this person, by name or ID, or me")
		fs.StringVar(&find.kind, "kind", "", "kinds of status, separated by commas: backlog, todo, in-progress, done, canceled")
		fs.IntVar(&limit, "limit", 0, "how many at most (100 unless given, 500 at most)")
		fs.StringVar(&find.after, "after", "", "an earlier answer's next: go on from where it stopped")
		fs.BoolVar(&asJSON, "json", false, "answer with JSON rather than a table")
	case "do":
		fs.StringVar(&batch, "batch", os.Getenv("MILA_BATCH"), "a batch name for the action's writes, to undo with others")
	case "whoami", "undo", "get":
	default:
		_, _ = fmt.Fprint(os.Stderr, apiUsage)
		return fmt.Errorf("unknown api command %q", cmd)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	rest := fs.Args()
	ctx, stop := signalled()
	defer stop()
	warnCleartext(o.Server)
	c, err := milaclient.Open(ctx, *o)
	if err != nil {
		return err
	}
	var out any
	switch cmd {
	case "whoami":
		me, err := c.Me(ctx)
		if err != nil {
			return err
		}
		out = map[string]any{"name": me.Name, "email": me.Email, "timezone": me.Timezone, "workspace": c.Workspace}
	case "find":
		q := url.Values{}
		for k, v := range map[string]string{"list": find.list, "view": find.view, "project": find.project,
			"due_from": find.dueFrom, "due_to": find.dueTo, "state": find.state, "words": find.words,
			"assignee": find.assignee, "kind": find.kind, "after": find.after} {
			if v != "" {
				q.Set(k, v)
			}
		}
		if limit != 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		var found struct {
			Items []map[string]any `json:"items"`
			Today string           `json:"today"`
			More  bool             `json:"more"`
			Next  string           `json:"next,omitempty"`
		}
		if err := c.Find(ctx, q, &found); err != nil {
			return err
		}
		if !asJSON {
			return table(stdout, found.Items, found.More, found.Next)
		}
		out = found
	case "do":
		if len(rest) < 1 || len(rest) > 2 {
			return errors.New(`usage: mila api do <action> ['{"json": "body"}']`)
		}
		var raw []byte
		if len(rest) == 2 && rest[1] != "-" {
			raw = []byte(rest[1])
		} else if raw, err = io.ReadAll(io.LimitReader(stdin, 1<<20)); err != nil {
			return err
		}
		var body any
		if err := json.Unmarshal(raw, &body); err != nil {
			return fmt.Errorf("the body is not JSON: %w", err)
		}
		c.Batch = batch
		if err := c.Action(ctx, rest[0], body, &out); err != nil {
			return err
		}
	case "undo":
		if len(rest) != 1 {
			return errors.New("usage: mila api undo <batch>")
		}
		if err := c.Undo(ctx, rest[0], &out); err != nil {
			return err
		}
	case "get":
		if len(rest) != 1 {
			return errors.New("usage: mila api get <path>")
		}
		path := rest[0]
		if !strings.HasPrefix(path, "/") {
			path = c.In("/" + path)
		}
		if err := c.Get(ctx, path, &out); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	_, err = stdout.Write(terminalJSON(b.Bytes()))
	return err
}

// table writes found tasks for a person to read.
func table(w io.Writer, items []map[string]any, more bool, next string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	// Errors writing to tw come back from Flush.
	_, _ = fmt.Fprintln(tw, "DUE\tSTATUS\tPROJECT\tTITLE\tID")
	for _, t := range items {
		due, _ := t["due_date"].(string)
		if clock, _ := t["due_time"].(string); clock != "" {
			due += " " + clock
		}
		status, project := "", "Inbox"
		if s, ok := t["status"].(map[string]any); ok {
			status, _ = s["name"].(string)
		}
		if t["completed_at"] != nil && status == "" {
			status = "done"
		}
		if p, ok := t["project"].(map[string]any); ok {
			project, _ = p["name"].(string)
		}
		title, _ := t["title"].(string)
		id, _ := t["id"].(string)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", dash(due), dash(printable(status)), printable(project), oneLine(title), printable(id))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if more {
		_, err := fmt.Fprintf(w, "(there may be more: narrow the search, or go on with --after %s)\n", printable(next))
		return err
	}
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(printable(s)), " ")
	if r := []rune(s); len(r) > 60 {
		return string(r[:59]) + "…"
	}
	return s
}

// printable is text from the server made safe for a terminal: anyone who can
// name a task (a guest, an email's sender) chose it, and an escape in it
// would be the terminal's to obey (OSC 52 writes the clipboard). Control
// and format characters become spaces; the joiner emoji need stays.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r != '\u200D' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, "\uFFFD"))
}

// errorLine is an error as main prints it: its words may be the server's,
// naming what someone else named, so they get printable's cleaning, but
// the error's own line ends and tabs (a usage line) stay.
func errorLine(err error) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r != '\u200D' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(err.Error(), "\uFFFD"))
}

// terminalJSON is JSON from encoding/json made safe for a terminal as
// printable makes text, without changing what it says: encoding/json
// escapes C0 controls only, so C1 controls and format characters (the
// marks that turn text around, zero-width ones) are written as \u escapes
// too. Outside the ASCII range they can only be inside strings.
func terminalJSON(b []byte) []byte {
	var out bytes.Buffer
	for _, r := range string(b) {
		if r >= 0x80 && r != '\u200D' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			for _, u := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(&out, `\u%04x`, u)
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.Bytes()
}

// warnCleartext says, on standard error, when the token goes over plain
// http to another computer, where anyone on the way can read it.
func warnCleartext(server string) {
	if milaclient.Cleartext(server) {
		_, _ = fmt.Fprintln(os.Stderr, "mila: warning: "+server+" is plain http, so the token crosses the network unencrypted; use https unless the network is only yours")
	}
}
