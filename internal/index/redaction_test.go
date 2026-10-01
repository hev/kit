package index

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

type secretSource struct {
	text  string
	title string
}

func (s secretSource) Describe() string { return "secret fixture" }
func (s secretSource) Units() ([]trace.Unit, error) {
	return []trace.Unit{{Key: "one", Signature: "v1"}, {Key: "two", Signature: "v1"}}, nil
}
func (s secretSource) Read(u trace.Unit) ([]trace.Turn, error) {
	return []trace.Turn{{SessionID: u.Key, TurnUUID: u.Key + "-user", Seq: 1, Role: "user", Harness: "claude_code", Summary: s.title, TS: "2026-10-01T10:00:00Z", Blocks: []trace.Block{{Type: "text", Text: s.text}, {Type: "tool_use", ToolName: "Bash", Text: s.text}, {Type: "tool_result", Text: s.text}}}}, nil
}
func fixtureSecrets() []string {
	return []string{
		"AKIAABCDEFGHIJKLMNOP",
		"ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		strings.Join([]string{"xoxb", "123456789012", "123456789012", "AbCdEfGhIjKlMnOpQrStUvWx"}, "-"),
		"sk-ABCDEFGHIJKLMNOP0123456789",
		"tpuf_AbCdEfGhIjKlMnOpQrStUvWx01234567",
		"-----BEGIN PRIVATE KEY-----\nAbCdEfGh1234567\n-----END PRIVATE KEY-----",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.AbCdEfGhIjKlMnOpQrStUvWxYz",
		"user:correct-horse-battery", "custom-token-value", "AbCdEfGhIjKlMnOpQrStUvWx0123456789",
	}
}
func fixtureText() string {
	secrets := fixtureSecrets()
	return strings.Join(secrets[:7], "\n") + "\nhttps://" + secrets[7] + "@example.com\nAuthorization: Bearer " + secrets[8] + "\nODD_KEY=" + secrets[9]
}
func stringValues(v any) string {
	switch v := v.(type) {
	case string:
		return v + "\n"
	case []any:
		var s string
		for _, a := range v {
			s += stringValues(a)
		}
		return s
	case map[string]any:
		var s string
		for _, a := range v {
			s += stringValues(a)
		}
		return s
	}
	return ""
}
func assertClean(t *testing.T, text string) {
	t.Helper()
	for _, secret := range fixtureSecrets() {
		if strings.Contains(text, secret) {
			t.Errorf("downstream contains secret category %d", strings.Index(fixtureText(), secret))
		}
	}
}
func TestRedactionRequestBoundaries(t *testing.T) {
	for _, store := range []string{"pgvector", "turbopuffer"} {
		for _, mode := range []string{"all", "read-side", "parallel", "summarize", "harness-title"} {
			t.Run(store+"/"+mode, func(t *testing.T) {
				t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
				var mu sync.Mutex
				writes := 0
				namespaces := map[string]bool{}
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						io.WriteString(w, `{"status":"ok","version":"0.7.3"}`)
						return
					}
					b, _ := io.ReadAll(r.Body)
					var body any
					if err := json.Unmarshal(b, &body); err != nil {
						t.Error(err)
					}
					assertClean(t, stringValues(body))
					if strings.HasSuffix(r.URL.Path, "/query") {
						io.WriteString(w, `{"rows":[]}`)
						return
					}
					mu.Lock()
					writes++
					namespaces[r.URL.Path] = true
					mu.Unlock()
					io.WriteString(w, `{"rows_upserted":1,"status":"OK"}`)
				}))
				defer srv.Close()
				cl, err := layer.New(srv.URL, "key", "test", "").WithStore(store)
				if err != nil {
					t.Fatal(err)
				}
				src := secretSource{text: fixtureText()}
				var report *Report
				called := 0
				if mode == "summarize" || mode == "harness-title" {
					if mode == "harness-title" {
						src.title = fixtureText()
					}
					report, err = Summarize(src, cl, 200, func(string) string { return "" }, nil, func(row trace.SessionRow) (string, error) {
						called++
						b, _ := json.Marshal(row)
						var v any
						json.Unmarshal(b, &v)
						assertClean(t, stringValues(v))
						return fixtureText(), nil
					}, nil)
					if mode == "summarize" && called != 2 {
						t.Fatalf("summary callbacks=%d", called)
					}
					if mode == "harness-title" && called != 0 {
						t.Fatal("harness titles regenerated")
					}
				} else {
					opt := Options{Tiers: trace.AllTiers[:], RepoURL: func(string) string { return "" }}
					if mode != "all" {
						opt.ReadSide = true
					}
					if mode == "parallel" {
						opt.Workers = 2
					}
					report, err = Run(src, cl, &State{Units: map[string]string{}}, opt)
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(report.Errors) > 0 {
					t.Fatal(report.Errors)
				}
				if writes == 0 || report.UnitsIndexed != 2 || len(report.Redactions) < 7 {
					t.Fatalf("writes=%d report=%+v", writes, report)
				}
				if mode == "all" && len(namespaces) != 3 {
					t.Fatalf("missing row kind: %v", namespaces)
				}
			})
		}
	}
}
func TestRedactionOptOutAndFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	os.WriteFile(path, []byte("[capture]\nredact=false\n"), 0600)
	rawSeen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), fixtureSecrets()[0]) {
			rawSeen = true
		}
		if strings.HasSuffix(r.URL.Path, "/query") {
			fmt.Fprint(w, `{"rows":[]}`)
		} else {
			fmt.Fprint(w, `{"rows_upserted":1}`)
		}
	}))
	defer srv.Close()
	report, err := Run(secretSource{text: fixtureText()}, layer.New(srv.URL, "key", "test", ""), &State{Units: map[string]string{}}, Options{})
	if err != nil || len(report.Redactions) != 0 || !rawSeen {
		t.Fatalf("optout raw=%v report=%+v err=%v", rawSeen, report, err)
	}
	os.WriteFile(path, []byte("[capture]\nredact_salt='invalid'\n"), 0600)
	rawSeen = false
	if _, err := Run(secretSource{text: fixtureText()}, nil, &State{Units: map[string]string{}}, Options{DryRun: true}); err == nil {
		t.Fatal("invalid config did not fail closed")
	}
	if _, err := Summarize(secretSource{}, nil, 200, nil, nil, nil, nil); err == nil {
		t.Fatal("summarize did not fail closed")
	}
}
