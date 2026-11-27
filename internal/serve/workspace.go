package serve

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/hev/kit/internal/redact"
)

// Tier selects the command's dashboard routes. Shared dashboard consumers
// that do their own routing can continue to use New without WithWorkspace.
type Tier string

const (
	OSS    Tier = "oss"
	Pro    Tier = "pro"
	Hosted Tier = "hosted"
)

type WorkspaceRow struct {
	ID     string `json:"id,omitempty"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// WorkspaceSource supplies a read-only list or search projection.
type WorkspaceSource func(context.Context, string) ([]WorkspaceRow, error)

type Workspace struct {
	Tier                    Tier
	Evidence, Orders, Peers WorkspaceSource
	Settings                []WorkspaceRow
	Secrets                 []string

	scrubber *redact.Scrubber
}

func (s *Server) WithWorkspace(cfg Workspace) *Server {
	var salt [32]byte
	_, _ = rand.Read(salt[:])
	cfg.scrubber, _ = redact.New(salt[:])
	cfg.Secrets = workspaceSecrets(cfg.Secrets...)
	s.workspace = &cfg
	return s
}

// RedactJSON applies the same read-side policy to hosted API responses and
// local data. Decode before scrubbing so JSON escaping cannot mask a match.
func (s *Server) RedactJSON(raw []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if len(raw) > maxBoardBody || !json.Valid(raw) {
		return nil, errors.New("invalid dashboard response")
	}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(s.workspace.cleanJSON(value))
}

var localPath = regexp.MustCompile(`(^|[\s"'\x60=(]|~)(/[\w.@%+~-]+(?:/[\w.@%+~:-]+)+|[a-zA-Z]:[\\/][^\s"'\x60]+|\\\\[^\s"'\x60]+)`)
var secretReference = regexp.MustCompile(`(?:secret|op|azkv)://[^\s"'\x60]+`)
var relativeWebPath = regexp.MustCompile(`^(/api/image/|/blob/)[a-f0-9]{64}$`)

func (cfg *Workspace) cleanText(value string) string {
	for _, secret := range cfg.Secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	if cfg.scrubber == nil {
		return "[REDACTED]"
	}
	value, _ = cfg.scrubber.Text(value)
	value = secretReference.ReplaceAllString(value, "[REDACTED]")
	if relativeWebPath.MatchString(value) {
		return value
	}
	return localPath.ReplaceAllString(value, "${1}[local path]")
}

func (cfg *Workspace) cleanJSON(value any) any {
	switch v := value.(type) {
	case string:
		return cfg.cleanText(v)
	case []any:
		for i := range v {
			v[i] = cfg.cleanJSON(v[i])
		}
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if cfg.Tier == OSS {
				switch k {
				case "eval", "evals", "eval_status", "eval_baseline", "evaluation", "evaluation_status", "marks", "poor":
					continue
				}
			}
			switch k {
			case "cwd", "workdir", "config_path", "resolved_repo":
				continue
			}
			out[cfg.cleanText(k)] = cfg.cleanJSON(item)
		}
		return out
	}
	return value
}

func (s *Server) oss() bool { return s.workspace != nil && s.workspace.Tier == OSS }

var proSections = []string{"board", "evidence", "orders", "peers", "mobile", "settings", "profile", "factory", "eval", "evals"}

func proRoute(path string) bool {
	path = strings.TrimPrefix(path, "/")
	for _, prefix := range []string{"api/", "v1/"} {
		path = strings.TrimPrefix(path, prefix)
	}
	for _, name := range proSections {
		if path == name || strings.HasPrefix(path, name+"/") {
			return true
		}
	}
	return false
}

func (s *Server) workspaceRoutes(mux *http.ServeMux) {
	if s.workspace == nil || s.oss() {
		return
	}
	for _, name := range []string{"board", "evidence", "orders", "peers", "mobile", "settings", "profile"} {
		mux.HandleFunc("GET /"+name, s.page)
	}
	for name, source := range map[string]WorkspaceSource{
		"evidence": s.workspace.Evidence, "orders": s.workspace.Orders, "peers": s.workspace.Peers,
		"settings": func(context.Context, string) ([]WorkspaceRow, error) { return s.workspace.Settings, nil },
	} {
		mux.HandleFunc("GET /api/"+name, func(w http.ResponseWriter, r *http.Request) {
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			if len(q) > 8192 {
				writeError(w, errors.New("search exceeds 8192 bytes"), http.StatusBadRequest)
				return
			}
			if source == nil {
				writeError(w, errors.New("source unavailable"), http.StatusServiceUnavailable)
				return
			}
			rows, err := source(r.Context(), q)
			if err != nil {
				writeError(w, errors.New("source unavailable"), http.StatusBadGateway)
				return
			}
			writeJSON(w, map[string]any{"rows": nonNil(rows)[:min(200, len(rows))]})
		})
	}
}

// Workspace sends bounded, sanitized JSON and stable errors at the browser
// boundary. HTML and image bytes have their own handlers.
func (s *Server) protectWorkspace(next http.Handler) http.Handler {
	if s.workspace == nil {
		return next
	}
	jsonHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &cappedRecorder{header: http.Header{}}
		next.ServeHTTP(rec, r)
		if rec.over {
			writeError(w, errors.New("dashboard response too large"), http.StatusBadGateway)
			return
		}
		status := rec.code
		if status >= 400 {
			message := http.StatusText(status)
			if status >= 500 {
				message = "dashboard source unavailable"
			}
			writeError(w, errors.New(message), status)
			return
		}
		if status == 0 {
			status = http.StatusOK
		}
		ctype := rec.header.Get("Content-Type")
		body := rec.body.Bytes()
		if strings.HasPrefix(ctype, "application/json") && len(body) > 0 {
			var err error
			body, err = s.RedactJSON(body)
			if err != nil {
				writeError(w, errors.New("invalid dashboard response"), http.StatusBadGateway)
				return
			}
		}
		for key, values := range rec.header {
			w.Header()[key] = values
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if s.oss() && (proRoute(path.Clean(r.URL.Path)) || evalQuery(r)) {
			writeError(w, errors.New("this route requires Kit Pro or Hosted"), http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/image/") && !strings.HasPrefix(r.URL.Path, "/api/board/blob/") {
			jsonHandler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func evalQuery(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	for key := range r.URL.Query() {
		if key == "eval" || key == "poor" || key == "role" || key == "instance" || (strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max")) {
			return true
		}
	}
	return false
}

func (s *Server) workspaceTier() Tier {
	if s.workspace == nil {
		return ""
	}
	return s.workspace.Tier
}

func workspaceSecrets(extra ...string) []string {
	for _, key := range []string{"HEV_LICENSE", "HEVLAYER_API_KEY", "LAYER_API_KEY", "HEV_KIT_KEY"} {
		extra = append(extra, os.Getenv(key))
	}
	return extra
}

type workspaceLink struct{ Label, Path, Icon string }

var workspaceLinks = []workspaceLink{
	{"Traces", "/#traces", "≋"},
	{"Board", "/board", "▤"},
	{"Evidence", "/evidence", "◇"},
	{"Orders", "/orders", "☷"},
	{"Peers", "/peers", "◎"},
	{"Mobile", "/mobile", "▯"},
}

func (s *Server) sidebar(path string) string {
	links := workspaceLinks
	if s.oss() {
		links = links[:1]
	}
	var out strings.Builder
	out.WriteString(`<button id="workspace-drawer" type="button" aria-label="Toggle navigation" aria-expanded="false" aria-controls="workspace-sidebar">☰</button>
<aside id="workspace-sidebar" aria-label="Workspace navigation">
 <header><a href="/#traces" title="hev kit">hev_ kit</a><button id="workspace-collapse" type="button" aria-label="Toggle sidebar" aria-expanded="true" aria-controls="workspace-sidebar">«</button></header>
 <button id="workspace-search" type="button" aria-label="Search workspace" aria-keyshortcuts="Meta+K Control+K"><span aria-hidden="true">⌕</span><span class="workspace-label">Search</span><kbd>⌘K</kbd></button>
 <nav aria-label="Primary">`)
	for _, link := range links {
		current := ""
		if strings.Split(link.Path, "#")[0] == path {
			current = ` aria-current="page"`
		}
		fmt.Fprintf(&out, `<a href="%s" aria-label="%s" title="%s"%s><span aria-hidden="true">%s</span><span class="workspace-label">%s</span></a>`,
			link.Path, link.Label, link.Label, current, link.Icon, link.Label)
	}
	out.WriteString(`</nav><footer><button id="workspace-theme" type="button" title="Toggle theme" aria-label="Toggle light and dark theme" aria-pressed="false"><span aria-hidden="true">◐</span><span class="workspace-label">Theme</span></button>`)
	if !s.oss() {
		out.WriteString(`<nav aria-label="Settings"><a href="/settings" title="Profile and settings" aria-label="Profile and settings"><span aria-hidden="true">⚙</span><span class="workspace-label">Profile &amp; settings</span></a></nav>`)
	}
	out.WriteString(`</footer></aside>
<dialog id="workspace-search-dialog" aria-label="Workspace search">
 <form role="search"><input type="search" id="workspace-search-input" aria-label="Search" placeholder="Search…" autofocus><button type="submit">Search</button><button type="button" id="workspace-search-close" aria-label="Close search">✕</button></form>
 <p id="workspace-search-status" role="status"></p><div id="workspace-results"></div>
</dialog>`)
	return out.String()
}
