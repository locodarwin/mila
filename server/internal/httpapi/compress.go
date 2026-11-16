package httpapi

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// gzipMin is the smallest JSON answer worth compressing: below about a
// packet, gzip's header and the work cost more than they save.
const gzipMin = 1400

var gzipWriters = sync.Pool{New: func() any {
	// The fastest level: a bootstrap is megabytes of JSON, and level 1
	// still shrinks it several times over in a fraction of the time.
	w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return w
}}

// compressJSON gzips the API's JSON answers when the browser accepts it,
// so a bootstrap crosses a slow link in a fraction of the time. Files
// (attachments, exports) and the sync socket pass through untouched.
func compressJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Header.Get("Upgrade") != "" || !accepts(r, "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gw := &gzipWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(gw, r)
		// Not deferred: after a panic, recoverPanics answers on w itself.
		gw.finish()
	})
}

// gzipWriter holds back the start of a JSON answer until it knows the
// answer is large enough to compress.
type gzipWriter struct {
	http.ResponseWriter
	status  int
	buf     []byte
	decided bool
	gz      *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	if !g.decided {
		g.status = code
	}
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.decided {
		h := g.Header()
		// Only answers of the API's own: a file is served with its
		// disposition, and may be asked for by range.
		answer := strings.HasPrefix(h.Get("Content-Type"), "application/json") &&
			h.Get("Content-Disposition") == "" && h.Get("Content-Encoding") == ""
		if !answer || g.status != http.StatusOK && g.status != http.StatusCreated && g.status < 400 {
			g.start(false)
		} else if g.buf = append(g.buf, p...); len(g.buf) < gzipMin {
			return len(p), nil
		} else {
			g.start(true)
			if _, err := g.gz.Write(g.buf); err != nil {
				return 0, err
			}
			g.buf = nil
			return len(p), nil
		}
	}
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

// start sends the header, compressed or not.
func (g *gzipWriter) start(compress bool) {
	g.decided = true
	if compress {
		h := g.Header()
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		g.gz = gzipWriters.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(g.status)
}

// finish sends what is held back, or ends the compressed stream.
func (g *gzipWriter) finish() {
	if !g.decided {
		g.start(false)
		if len(g.buf) > 0 {
			_, _ = g.ResponseWriter.Write(g.buf)
		}
		return
	}
	if g.gz != nil {
		_ = g.gz.Close()
		g.gz.Reset(nil)
		gzipWriters.Put(g.gz)
	}
}
