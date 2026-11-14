package asks

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Principal identifies the signed-in viewer. Resolve it from the host's
// authenticated session, never from a request parameter.
type Principal struct{ ID string }
type HostedOptions struct {
	// BasePath is an absolute URL path without a trailing slash. The returned
	// handler consumes that prefix itself; do not use StripPrefix.
	BasePath string
	// TracesPath points at the host's session viewer (for session links).
	TracesPath string
	Principal  func(*http.Request) (Principal, error)
	// Session optionally supplies /api/session/{id} for waterfall expansion.
	// It must be scoped to the same team and must not read local dispatch data.
	Session http.Handler
}

// NewHosted serves the shared asks list and waterfall over an explicit reader.
// The host must select a team-scoped reader before constructing this handler.
// No local records, environment settings or filesystem stores are consulted.
func NewHosted(reader Reader, opts HostedOptions) (http.Handler, error) {
	if reader == nil || opts.Principal == nil {
		return nil, errors.New("asks: reader and principal resolver are required")
	}
	base, err := validBase(opts.BasePath)
	if err != nil {
		return nil, err
	}
	traces, err := validBase(opts.TracesPath)
	if err != nil {
		return nil, err
	}
	reader = hostedReader{reader}
	s := &Server{hosted: true, Reader: reader, ListRows: func(filter any) ([]SessionRow, error) { return reader.ListSessionRows(100000, filter) }}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/asks", s.List)
	mux.HandleFunc("GET /api/asks/{id}", s.Detail)
	if opts.Session != nil {
		mux.Handle("GET /api/session/{id}", opts.Session)
	}
	files, _ := fs.Sub(Assets, "assets/ui")
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(files))))
	page := PageTemplate
	payload, _ := json.Marshal(map[string]any{"session": map[string]any{}, "corpus": []any{}, "facets": map[string]any{}, "rates": map[string]any{}, "days": 30, "storage": map[string]any{"mode": "hosted", "evals": false}})
	page = strings.Replace(page, "/*__DATA__*/null", string(payload), 1)
	page = strings.ReplaceAll(page, "<!--__REVIEW_LINK__-->", "")
	// Only the asks navigation is available here; session links use the host's
	// traces view. The same rendering code supplies the list and waterfall.
	for _, name := range []string{"stats", "traces"} {
		page = strings.ReplaceAll(page, `data-v="`+name+`"`, `hidden data-v="`+name+`"`)
	}
	page = strings.Replace(page, "async function getJSON(url,signal){", "async function getJSON(url,signal){ url="+js(base)+"+url;", 1)
	page = strings.Replace(page, "function askSessionHref(id){", "function askSessionHref(id){return "+js(traces+"/")+"+'?session='+encodeURIComponent(id)+'#transcript';}\nfunction unusedSessionHref(id){", 1)

	page = strings.Replace(page, "includes(h)?h:'stats'", "includes(h)?h:'asks'", 1)
	page = strings.ReplaceAll(page, `href="ui/`, `href="`+base+`/ui/`)
	page = strings.ReplaceAll(page, `import('./ui/`, `import('`+base+`/ui/`)
	page = strings.ReplaceAll(page, `from './ui/`, `from '`+base+`/ui/`)
	page = strings.ReplaceAll(page, `from 'ui/`, `from '`+base+`/ui/`)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		p, e := opts.Principal(r)
		if e != nil || strings.TrimSpace(p.ID) == "" {
			writeError(w, errors.New("authentication required"), 401)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			writeError(w, errors.New("method not allowed"), 405)
			return
		}
		if base != "" && !(r.URL.Path == base || strings.HasPrefix(r.URL.Path, base+"/")) {
			http.NotFound(w, r)
			return
		}
		rr := r.Clone(r.Context())
		rr.URL.Path = strings.TrimPrefix(r.URL.Path, base)
		rr.URL.RawPath = ""
		if rr.URL.Path == "" {
			rr.URL.Path = "/"
		}
		mux.ServeHTTP(&safeErrors{ResponseWriter: w}, rr)
	}), nil
}
func validBase(s string) (string, error) {
	if s == "" || s == "/" {
		return "", nil
	}
	if !strings.HasPrefix(s, "/") || strings.ContainsAny(s, "?#\\\"'<> \t\r\n") || strings.Contains(s, "//") || strings.Contains(s, "..") || strings.Contains(s, "%") {
		return "", errors.New("asks: invalid base path")
	}
	return strings.TrimRight(s, "/"), nil
}
func js(s string) string { b, _ := json.Marshal(s); return string(b) }

// Archive facts contain no connection configuration. Drop local operator
// recipe paths and reduce repository paths to display names at this boundary.
type hostedReader struct{ Reader }

func (r hostedReader) ListTraceSpanRows(filter any) ([]SpanRow, error) {
	rows, e := r.Reader.ListTraceSpanRows(filter)
	if e != nil {
		return nil, e
	}
	out := append([]SpanRow(nil), rows...)
	for i := range out {
		out[i].ContextRecipe = ""
		out[i].Repo = path.Base(strings.ReplaceAll(out[i].Repo, "\\", "/"))
	}
	return out, nil
}

type safeErrors struct {
	http.ResponseWriter
	failed bool
}

func (w *safeErrors) WriteHeader(code int) {
	if code >= 400 {
		w.failed = true
		w.Header().Del("Content-Length")
		writeError(w.ResponseWriter, errors.New(http.StatusText(code)), code)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *safeErrors) Write(b []byte) (int, error) {
	if w.failed {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
