package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPrecompressedAssets(t *testing.T) {
	files := fstest.MapFS{
		"index.html":                  {Data: []byte(`<!doctype html>`)},
		"_app/immutable/app.js":       {Data: []byte(`plain`)},
		"_app/immutable/app.js.br":    {Data: []byte(`brotli`)},
		"_app/immutable/app.js.gz":    {Data: []byte(`gzipped`)},
		"_app/immutable/other.js":     {Data: []byte(`only plain`)},
		"_app/immutable/style.css.gz": {Data: []byte(`gz css`)},
		"_app/immutable/style.css":    {Data: []byte(`css`)},
	}
	h := newTest(t, Deps{Web: files})
	for _, c := range []struct{ path, accept, body, encoding, ctype string }{
		{"/_app/immutable/app.js", "gzip, deflate, br, zstd", "brotli", "br", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "gzip", "gzipped", "gzip", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "br;q=0, gzip;q=0.5", "gzipped", "gzip", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "*", "brotli", "br", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "*, br;q=0", "gzipped", "gzip", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "", "plain", "", "text/javascript; charset=utf-8"},
		{"/_app/immutable/app.js", "identity", "plain", "", "text/javascript; charset=utf-8"},
		{"/_app/immutable/other.js", "br", "only plain", "", "text/javascript; charset=utf-8"},
		{"/_app/immutable/style.css", "br, gzip", "gz css", "gzip", "text/css; charset=utf-8"},
	} {
		req := httptest.NewRequest("GET", c.path, nil)
		req.Header.Set("Accept-Encoding", c.accept)
		req.Header.Set("Range", "bytes=0-1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header()
		if c.encoding != "" && (rec.Code != 200 || rec.Body.String() != c.body) {
			t.Errorf("%s with %q = %d %q, want 200 %q", c.path, c.accept, rec.Code, rec.Body, c.body)
		}
		if got.Get("Content-Encoding") != c.encoding || got.Get("Content-Type") != c.ctype {
			t.Errorf("%s with %q: encoding %q type %q, want %q %q", c.path, c.accept,
				got.Get("Content-Encoding"), got.Get("Content-Type"), c.encoding, c.ctype)
		}
		if c.encoding != "" && got.Get("Vary") != "Accept-Encoding" {
			t.Errorf("%s with %q: no Vary", c.path, c.accept)
		}
		if got.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("%s: cache %q", c.path, got.Get("Cache-Control"))
		}
	}
}

func TestCompressJSON(t *testing.T) {
	big := `{"rows":"` + strings.Repeat("x", 5000) + `"}`
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/big":
			writeJSON(w, http.StatusOK, map[string]string{"rows": strings.Repeat("x", 5000)})
		case "/api/v1/small":
			writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
		case "/api/v1/file":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment")
			_, _ = w.Write([]byte(big))
		case "/api/v1/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/other":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(big))
		}
	})
	h := compressJSON(inner)
	for _, c := range []struct {
		path, accept string
		gzipped      bool
		code         int
	}{
		{"/api/v1/big", "gzip, br", true, 200},
		{"/api/v1/big", "br", false, 200},
		{"/api/v1/small", "gzip", false, 200},
		{"/api/v1/file", "gzip", false, 200},
		{"/api/v1/empty", "gzip", false, 204},
		{"/other", "gzip", false, 200},
	} {
		req := httptest.NewRequest("GET", c.path, nil)
		req.Header.Set("Accept-Encoding", c.accept)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.code || (rec.Header().Get("Content-Encoding") == "gzip") != c.gzipped {
			t.Errorf("%s with %q = %d, encoding %q; want %d, gzipped %v", c.path, c.accept, rec.Code,
				rec.Header().Get("Content-Encoding"), c.code, c.gzipped)
			continue
		}
		body := rec.Body.String()
		if c.gzipped {
			zr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(zr)
			if err != nil {
				t.Fatal(err)
			}
			body = string(b)
			if rec.Body.Len() > 200 {
				t.Errorf("%s: %d bytes compressed", c.path, rec.Body.Len())
			}
		}
		if c.code == 200 && !strings.Contains(body, `"`) {
			t.Errorf("%s: body %q", c.path, body)
		}
		if c.path == "/api/v1/big" && strings.TrimSpace(body) != big {
			t.Errorf("%s: body changed", c.path)
		}
	}
	// A socket upgrade is never wrapped: it must reach the real writer.
	req := httptest.NewRequest("GET", "/api/v1/sync/ws", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Upgrade", "websocket")
	compressJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(*gzipWriter); ok {
			t.Error("the socket's writer is wrapped")
		}
	})).ServeHTTP(httptest.NewRecorder(), req)
}
