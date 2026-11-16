// Package updates says when a newer Mila is out: once a day it asks
// GitHub for the latest release of Mila's repository, sending nothing but
// the request (MILA_UPDATE_CHECK=off stops it). Settings, Admin shows what
// it found. It never updates anything.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LatestURL is GitHub's answer for the latest release of Mila's repository.
const LatestURL = "https://api.github.com/repos/locodarwin/mila/releases/latest"

// releasePage is where a release's own page must be, for a link to it.
const releasePage = "https://github.com/locodarwin/mila/releases/"

// Release is a version of Mila that is out.
type Release struct {
	Version     string
	URL         string
	PublishedAt time.Time
}

// Checker asks for the latest release and remembers it.
type Checker struct {
	current string
	url     string
	client  *http.Client
	log     *slog.Logger

	mu      sync.Mutex
	latest  *Release
	checked time.Time
}

// New makes a checker for the version running.
func New(current string, log *slog.Logger) *Checker {
	return &Checker{current: current, url: LatestURL, client: &http.Client{Timeout: 20 * time.Second}, log: log}
}

// Run checks a minute after starting, then once a day, until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := c.Check(ctx); err != nil {
			c.log.Debug("could not check for a newer Mila", "err", err)
		}
		wait = 24 * time.Hour
	}
}

// Check asks once.
func (c *Checker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	// GitHub wants a name; nothing more is sent.
	req.Header.Set("User-Agent", "Mila")
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		// A private repository, or a limit reached: nothing is known.
		return fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return err
	}
	if body.Draft || body.Prerelease || !release.MatchString(body.TagName) {
		return errors.New("the latest release is not a version")
	}
	link := body.HTMLURL
	if !strings.HasPrefix(link, releasePage) {
		link = releasePage + "tag/" + body.TagName
	}
	c.mu.Lock()
	c.latest = &Release{Version: body.TagName, URL: link, PublishedAt: body.PublishedAt}
	c.checked = time.Now()
	c.mu.Unlock()
	return nil
}

// Newer is the latest release when it is newer than the version running,
// or nil when it is not, or nothing is known yet.
func (c *Checker) Newer() *Release {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latest == nil || !IsNewer(c.latest.Version, c.current) {
		return nil
	}
	r := *c.latest
	return &r
}

var release = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// version reads v2.8.0, or a build after it (v2.8.0-6-g0d7dbdd, -dirty).
var version = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(-.*)?$`)

// IsNewer says whether release latest is newer than the version running.
// A build that is no version (dev) is never behind: it is someone's own.
func IsNewer(latest, current string) bool {
	l, okL := parts(latest)
	c, okC := parts(current)
	if !okL || !okC {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parts(v string) ([3]int, bool) {
	m := version.FindStringSubmatch(v)
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
