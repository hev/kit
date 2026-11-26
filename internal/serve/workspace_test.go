package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestWorkspaceTierRoutes(t *testing.T) {
	const secret = "test-secret-used-by-the-upstream"
	const file = "/home/operator/private/fixture"
	for _, tier := range []Tier{OSS, Pro, Hosted} {
		t.Run(string(tier), func(t *testing.T) {
			reader := &MemoryReader{
				Sessions: []trace.SessionRow{{ID: "s", SessionID: "s", End: time.Now().UnixMilli(), Summary: "Useful trace " + secret}},
				Blocks:   []trace.BlockRow{{SessionID: "s", TurnUUID: "turn", Text: "needle " + file}},
			}
			var sourceCalls int
			source := func(_ context.Context, query string) ([]WorkspaceRow, error) {
				sourceCalls++
				if query == "error" {
					return nil, errors.New(secret + " " + file)
				}
				return []WorkspaceRow{{ID: "fixture", Title: "needle fixture", Detail: file + " " + secret}}, nil
			}
			board := LocalBoard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceCalls++
				writeJSON(w, []any{map[string]any{"title": "needle fixture " + secret, "cwd": file}})
			}), "filesystem")
			cfg := Workspace{Tier: tier, Evidence: source, Orders: source, Peers: source, Secrets: []string{secret}}
			h := New(reader).WithBoard(board).WithWorkspace(cfg).Handler()
			check := func(method, path string, code int, includes string) string {
				t.Helper()
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
				body := w.Body.String()
				if w.Code != code || !strings.Contains(body, includes) {
					t.Fatalf("%s %s returned %d, wanted %d containing %q", method, path, w.Code, code, includes)
				}
				if strings.Contains(body, secret) || strings.Contains(body, file) {
					t.Fatalf("%s %s leaked source details", method, path)
				}
				return body
			}
			html := check("GET", "/", 200, `id="workspace-sidebar"`)
			nav := strings.SplitN(strings.SplitN(html, `id="workspace-sidebar"`, 2)[1], "</aside>", 2)[0]
			for _, marker := range []string{`href="/#traces"`, `id="workspace-collapse"`, `id="workspace-theme"`, `aria-keyshortcuts="Meta+K Control+K"`} {
				if !strings.Contains(nav, marker) {
					t.Fatalf("sidebar lacked %s", marker)
				}
			}
			check("GET", "/api/search?q=needle&window=all", 200, `"id":"s"`)
			check("GET", "/api/search?q=unmatched&window=all", 200, `"sessions":[]`)
			for _, name := range []string{"board", "evidence", "orders", "peers", "mobile", "settings"} {
				if tier == OSS {
					if strings.Contains(nav, `href="/`+name+`"`) {
						t.Fatalf("OSS sidebar exposed %s", name)
					}
					check("GET", "/"+name, 403, "requires Kit Pro")
					check("GET", "/api/"+name+"/unknown", 403, "requires Kit Pro")
					check("POST", "/api/"+name, 403, "requires Kit Pro")
				} else {
					if !strings.Contains(nav, `href="/`+name+`"`) {
						t.Fatalf("%s sidebar lacked %s", tier, name)
					}
					check("GET", "/"+name, 200, `id="workspace-sidebar"`)
				}
			}
			if tier == OSS {
				check("GET", "/api/sessions?eval=passed", 403, "requires Kit Pro")
				check("GET", "/api/bo%61rd/search?q=needle", 403, "requires Kit Pro")
				if sourceCalls != 0 {
					t.Fatal("OSS called Pro sources")
				}
				return
			}
			for _, section := range []string{"evidence", "orders", "peers", "board/search"} {
				check("GET", "/api/"+section+"?q=needle", 200, "needle fixture")
			}
			check("POST", "/api/evidence", 405, "Method Not Allowed")
			check("GET", "/api/evidence?q=error", 502, "dashboard source unavailable")
		})
	}
}

type workspaceEvalReader struct{ MemoryReader }

func (r *workspaceEvalReader) ListEvalRows(any) ([]layer.EvalRow, error) {
	return []layer.EvalRow{{ID: "grade", SessionID: "graded", TS: time.Now().Format(time.RFC3339), Poor: true, Marks: `{"quality":2}`, Evidence: "{}", Findings: "[]"}}, nil
}

func (r *workspaceEvalReader) SearchEvals(string, int, any) ([]layer.Hit, error) {
	return nil, nil
}

func TestWorkspaceEvalFiltersKeepOnlySelectedStatus(t *testing.T) {
	reader := &workspaceEvalReader{MemoryReader{Sessions: []trace.SessionRow{
		{ID: "graded", SessionID: "graded", End: time.Now().UnixMilli()},
		{ID: "ordinary", SessionID: "ordinary", End: time.Now().UnixMilli()},
	}}}
	handler := New(reader).WithWorkspace(Workspace{Tier: Pro}).Handler()
	for filter, want := range map[string]string{"failed": "graded", "unevaluated": "ordinary"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?window=all&eval="+filter, nil))
		var body struct {
			Sessions []struct {
				ID     string
				Status string `json:"eval_status"`
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || len(body.Sessions) != 1 || body.Sessions[0].ID != want || body.Sessions[0].Status != filter {
			t.Fatalf("%s filter: %d %s (%v)", filter, w.Code, w.Body.String(), err)
		}
	}
}
