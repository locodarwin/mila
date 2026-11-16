package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/locodarwin/mila/server/internal/mcptools"
	"github.com/locodarwin/mila/server/internal/milaclient"
)

// Mila's own MCP endpoint (docs/PLAN.md section 11, Agent access, phase 4):
// the tools `mila mcp` serves over stdio, served at /api/v1/mcp over MCP's
// streamable HTTP to an agent that sends an API token. It keeps no session:
// each POST is answered by a server made for it, whose tools call the API in
// this process with the same token, so every permission, limit and line of
// history is what the token would meet calling the API itself.

// mcpPath is where the endpoint is.
const mcpPath = "/api/v1/mcp"

// mcpBody is the most one message to the endpoint may be: an action's body,
// with room for JSON-RPC around it.
const mcpBody = 2 * actionBody

type mcpServerKey struct{}

// mcpEndpoint serves /api/v1/mcp. The workspace is ?workspace=, by name or
// ID, unless the token reaches only one.
func (h *handlers) mcpEndpoint() http.Handler {
	sdk := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		s, _ := r.Context().Value(mcpServerKey{}).(*mcp.Server)
		return s
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// Nothing is ever sent to the agent unasked, so a plain answer does.
		JSONResponse: true,
		// The SDK refuses a request that reached a loopback address under
		// another host name, against DNS rebinding. Mila behind a proxy on
		// the same computer is reached exactly that way, and a page that
		// rebinds a name cannot add the token every request here needs.
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        mcpBody,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := authOf(r)
		secret, sent := bearerToken(r)
		if a == nil || a.Token == nil || !sent {
			w.Header().Set("WWW-Authenticate", h.mcpChallenge(r))
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Send an API token, as Authorization: Bearer mila_...")
			return
		}
		if h.apiUser(w, r, false) == nil {
			return
		}
		c, err := milaclient.Open(r.Context(), milaclient.Options{
			Server:    "http://mila.internal",
			Token:     secret,
			Workspace: r.URL.Query().Get("workspace"),
			HTTP:      &http.Client{Transport: inProcess{h: h.root, outer: r}},
			Choose:    "by adding ?workspace= with its name or ID to the address",
		})
		if err != nil {
			var refused *milaclient.Error
			if errors.As(err, &refused) {
				writeError(w, refused.Status, refused.Code, refused.Message)
				return
			}
			writeError(w, http.StatusBadRequest, "invalid", "Mila's MCP endpoint: "+err.Error()+".")
			return
		}
		sdk.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mcpServerKey{}, mcptools.HTTPServer(c))))
	})
}

// mcpChallenge is the WWW-Authenticate an agent without a token is given:
// with OAuth on, where to find out how to get one (RFC 9728 section 5.1),
// for the address it asked for, query and all.
func (h *handlers) mcpChallenge(r *http.Request) string {
	if !h.oauth {
		return `Bearer realm="Mila"`
	}
	// A quote in the query (Go's server takes one in the request's
	// target) is percent-encoded, so it can't end the quoted string and add
	// a parameter pointing elsewhere; the rest stays as asked, which the
	// metadata's resource must match.
	meta := h.origin + prmPath + mcpPath
	if r.URL.RawQuery != "" {
		meta += "?" + strings.NewReplacer(`"`, "%22", `\`, "%5C").Replace(r.URL.RawQuery)
	}
	return `Bearer realm="Mila", resource_metadata="` + meta + `"`
}

type insideKey struct{}

// fromInside says a request is one the server made of itself, for the MCP
// endpoint's tools. Nothing from outside can set it.
func fromInside(ctx context.Context) bool {
	inside, _ := ctx.Value(insideKey{}).(bool)
	return inside
}

// inProcess sends the tools' requests to Mila's own handler, as if from
// where the agent's request came, without a connection.
type inProcess struct {
	h     http.Handler
	outer *http.Request
}

func (t inProcess) RoundTrip(req *http.Request) (*http.Response, error) {
	in := req.Clone(context.WithValue(req.Context(), insideKey{}, true))
	if in.Body == nil {
		in.Body = http.NoBody
	}
	in.Host = req.URL.Host
	in.RequestURI = req.URL.RequestURI()
	in.RemoteAddr = t.outer.RemoteAddr
	in.Header.Del("X-Forwarded-For")
	for _, v := range t.outer.Header.Values("X-Forwarded-For") {
		in.Header.Add("X-Forwarded-For", v)
	}
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, in)
	return rec.Result(), nil
}
