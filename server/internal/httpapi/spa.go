package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// spa serves the built frontend: real files as they are, and index.html for
// every other path so client-side routes such as /dev/ui load the app.
type spa struct {
	files        fs.FS
	fileServer   http.Handler
	index        []byte
	scriptHashes []string // CSP hashes of index.html's inline scripts
}

const notBuilt = `<!doctype html><meta charset="utf-8"><title>Mila</title>
<p>The frontend is not built into this binary. Run <code>make build</code>.</p>
`

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

func newSPA(files fs.FS) (*spa, error) {
	s := &spa{files: files, fileServer: http.FileServerFS(files)}
	index, err := fs.ReadFile(files, "index.html")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.index = []byte(notBuilt)
	case err != nil:
		return nil, err
	default:
		s.index = index
	}
	for _, m := range inlineScript.FindAllSubmatch(s.index, -1) {
		sum := sha256.Sum256(m[1])
		s.scriptHashes = append(s.scriptHashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return s, nil
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(s.files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			ctype := contentType(name)
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			if s.serveEncoded(w, r, name, ctype) {
				return
			}
			s.fileServer.ServeHTTP(w, r)
			return
		}
		// A missing asset is a 404, not the app: a stale tab asking for an
		// old chunk must fail loudly rather than parse HTML as JavaScript.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(s.index))
}

// assetTypes are the types of the files the build writes. Go looks types
// up in the registry on Windows, where .js can be text/plain and browsers
// then refuse to run the app, so these do not depend on the machine.
var assetTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".ico":         "image/x-icon",
	".js":          "text/javascript; charset=utf-8",
	".json":        "application/json",
	".mjs":         "text/javascript; charset=utf-8",
	".png":         "image/png",
	".svg":         "image/svg+xml",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".woff2":       "font/woff2",
}

// contentType is the type to serve a built file as, or "" to let the
// file server work it out.
func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := assetTypes[ext]; ok {
		return t
	}
	return mime.TypeByExtension(ext)
}

// encodings are the precompressed copies the build writes beside each
// file (adapter-static's precompress), best first.
var encodings = []struct{ name, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

// serveEncoded serves the precompressed copy of a file the browser
// accepts, if the build made one. A binary run without a reverse proxy
// then sends the same small scripts a proxy would.
func (s *spa) serveEncoded(w http.ResponseWriter, r *http.Request, name, ctype string) bool {
	if ctype == "" {
		return false
	}
	for _, enc := range encodings {
		if !accepts(r, enc.name) {
			continue
		}
		f, err := s.files.Open(name + enc.ext)
		if err != nil {
			continue
		}
		defer func() { _ = f.Close() }()
		rs, ok := f.(io.ReadSeeker)
		if !ok {
			return false
		}
		h := w.Header()
		h.Set("Content-Encoding", enc.name)
		h.Add("Vary", "Accept-Encoding")
		// A range would be of the compressed bytes; send the whole file.
		r.Header.Del("Range")
		http.ServeContent(w, r, name, time.Time{}, rs)
		return true
	}
	return false
}

// accepts reports whether the request's Accept-Encoding allows an
// encoding: by name, or by "*" when it is not named, and not with q=0.
func accepts(r *http.Request, encoding string) bool {
	wildcard := false
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		allowed := true
		if v, ok := strings.CutPrefix(strings.ReplaceAll(params, " ", ""), "q="); ok {
			q, err := strconv.ParseFloat(v, 64)
			allowed = err == nil && q > 0
		}
		switch name {
		case encoding:
			return allowed
		case "*":
			wildcard = allowed
		}
	}
	return wildcard
}
