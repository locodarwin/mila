// Package milaclient talks to a Mila server's API with a personal API token,
// for `mila mcp` and `mila api`: the
// API's actions, its reads, and undoing a batch, in one workspace.
package milaclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/version"
)

// Client is one token's access to one Mila server, in one workspace.
type Client struct {
	base      string // https://mila.example.com/api/v1
	token     string
	http      *http.Client
	Workspace Workspace
	// Batch is put on every write, so all of them can be undone as one.
	Batch string
}

// Workspace is a workspace the token reaches.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// Me is whom the token acts for.
type Me struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Timezone string `json:"timezone"`
}

// NewBatch names a new batch of writes, kind first.
func NewBatch(kind string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return kind + "-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// Error is the server's refusal: its status, and the words it gave.
type Error struct {
	Status  int
	Code    string `json:"error"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Field != "" {
		msg = e.Field + ": " + msg
	}
	return fmt.Sprintf("%s (%d)", msg, e.Status)
}

// Options is where to find the server, the token and the workspace.
type Options struct {
	// Server is the server's address, such as https://mila.example.com.
	Server string
	// TokenFile names a file holding the token; Token is the token itself,
	// for when only the environment can carry it.
	TokenFile, Token string
	// Workspace is a workspace's ID or name; empty for the only one the
	// token reaches.
	Workspace string
	// HTTP is the client to send with; nil for one with a timeout.
	HTTP *http.Client
	// Choose says how to choose a workspace, for when the token reaches
	// more than one; empty for the command line's flag and variable.
	Choose string
}

// Cleartext says whether a server's address sends the token unencrypted:
// http to anywhere but this computer.
func Cleartext(server string) bool {
	u, err := url.Parse(strings.TrimSpace(server))
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

// Open checks the server and the token, and settles on the workspace.
func Open(ctx context.Context, o Options) (*Client, error) {
	server := strings.TrimRight(strings.TrimSpace(o.Server), "/")
	if server == "" {
		return nil, errors.New("no server: set MILA_SERVER or --server to Mila's address, such as https://mila.example.com")
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("the server %q is not an http or https address", o.Server)
	}
	token, err := readToken(o)
	if err != nil {
		return nil, err
	}
	c := &Client{base: strings.TrimSuffix(server, "/api/v1") + "/api/v1", token: token, http: o.HTTP}
	if c.http == nil {
		c.http = &http.Client{Timeout: 60 * time.Second}
	}
	var spaces []Workspace
	if err := c.Get(ctx, "/workspaces", &spaces); err != nil {
		return nil, fmt.Errorf("reaching %s: %w", server, err)
	}
	ref := strings.TrimSpace(o.Workspace)
	var found []Workspace
	for _, w := range spaces {
		if ref == "" || w.ID == ref || strings.EqualFold(w.Name, ref) {
			found = append(found, w)
		}
	}
	switch {
	case len(found) == 1:
		c.Workspace = found[0]
	case len(found) == 0 && ref != "":
		return nil, fmt.Errorf("the token reaches no workspace called %q; it reaches %s", ref, names(spaces))
	case len(found) == 0:
		return nil, errors.New("the token reaches no workspace")
	case ref == "":
		choose := o.Choose
		if choose == "" {
			choose = "with MILA_WORKSPACE or --workspace"
		}
		return nil, fmt.Errorf("the token reaches more than one workspace (%s); choose one %s", names(spaces), choose)
	default:
		return nil, fmt.Errorf("more than one workspace is called %q; give its ID", ref)
	}
	return c, nil
}

func names(spaces []Workspace) string {
	out := make([]string, len(spaces))
	for i, w := range spaces {
		out[i] = strconv.Quote(w.Name)
	}
	return strings.Join(out, ", ")
}

// readToken reads the token: from its file, kept from other users of the
// computer, or else as given.
func readToken(o Options) (string, error) {
	token := strings.TrimSpace(o.Token)
	if o.TokenFile != "" {
		info, err := os.Stat(o.TokenFile)
		if err != nil {
			return "", fmt.Errorf("the token file: %w", err)
		}
		// Windows keeps no such bits (every file reads as 0666): there the
		// file's own access rules, a user folder's, keep it.
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("the token file %s can be read by others; run chmod 600 on it", o.TokenFile)
		}
		raw, err := os.ReadFile(o.TokenFile)
		if err != nil {
			return "", fmt.Errorf("the token file: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	if token == "" {
		return "", errors.New("no token: make one in Mila's Settings, API tokens, save it to a file only you can read, and set MILA_TOKEN_FILE or --token-file to it")
	}
	if !strings.HasPrefix(token, "mila_") {
		return "", errors.New("that is not a Mila API token (they start mila_)")
	}
	return token, nil
}

// do sends a request and reads its JSON answer into out. A request that
// may be sent again (a read, or a write with an Idempotency-Key) is tried
// again after a dropped connection or a 429 or 503, as the server asks.
func (c *Client) do(ctx context.Context, method, path string, body any, headers map[string]string, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	again := method == http.MethodGet || headers["Idempotency-Key"] != ""
	for try := 1; ; try++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "mila-client/"+version.Version)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if again && try < 3 && ctx.Err() == nil {
				sleep(ctx, time.Duration(try)*time.Second)
				continue
			}
			return err
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) && again && try < 3 {
			wait, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if wait > 0 && wait <= 60 {
				sleep(ctx, time.Duration(wait)*time.Second)
				continue
			}
		}
		if resp.StatusCode >= 300 {
			e := &Error{Status: resp.StatusCode}
			_ = json.Unmarshal(raw, e)
			return e
		}
		if out == nil || len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, out)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// Get reads any path of the API, such as /me or a task's row.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, nil, out)
}

// Me is whom the token acts for.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var me Me
	return me, c.Get(ctx, "/me", &me)
}

// In is a path inside the workspace.
func (c *Client) In(path string) string {
	return "/workspaces/" + url.PathEscape(c.Workspace.ID) + path
}

// Action runs one of the workspace's actions with a body, in the client's
// batch, and reads its answer into out. Each gets a key of its own, so a
// retry after a dropped connection makes nothing twice.
func (c *Client) Action(ctx context.Context, name string, body, out any) error {
	return c.do(ctx, http.MethodPost, c.In("/actions/"+url.PathEscape(name)), body,
		map[string]string{"Idempotency-Key": uuid.NewString(), "Mila-Batch": c.Batch}, out)
}

// Find runs find-tasks with its query.
func (c *Client) Find(ctx context.Context, q url.Values, out any) error {
	return c.Get(ctx, c.In("/actions/find-tasks?"+q.Encode()), out)
}

// Undo undoes a batch, and reads what it did into out.
func (c *Client) Undo(ctx context.Context, batch string, out any) error {
	return c.do(ctx, http.MethodPost, c.In("/actions/undo-batch"), map[string]string{"batch": batch}, nil, out)
}

// Rows lists every row of a collection of the workspace, such as
// "checklist-items", with the filters given.
func (c *Client) Rows(ctx context.Context, collection string, filters url.Values) ([]map[string]any, error) {
	q := url.Values{}
	for k, v := range filters {
		q[k] = v
	}
	q.Set("limit", "500")
	var all []map[string]any
	for {
		var page struct {
			Items []map[string]any `json:"items"`
			Next  string           `json:"next"`
		}
		if err := c.Get(ctx, c.In("/"+collection+"?"+q.Encode()), &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.Next == "" {
			return all, nil
		}
		q.Set("after", page.Next)
	}
}

// Write changes one row in the client's batch: POST to make one (path the
// collection), PATCH to change it, DELETE to delete it.
func (c *Client) Write(ctx context.Context, method, path string, body, out any) error {
	headers := map[string]string{"Mila-Batch": c.Batch}
	if method == http.MethodPost {
		headers["Idempotency-Key"] = uuid.NewString()
	}
	return c.do(ctx, method, c.In(path), body, headers, out)
}
