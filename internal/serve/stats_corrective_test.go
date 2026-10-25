package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestStatsWindowAliases(t *testing.T) {
	s, _ := dashboardFixture(t)
	for _, pair := range [][2]string{{"07d", "7d"}, {"+7d", "7d"}, {"030d", "30d"}, {"+30d", "30d"}, {"090d", "90d"}, {"+90d", "90d"}} {
		t.Run(pair[0], func(t *testing.T) {
			for _, path := range []string{"/api/stats?", "/api/sessions?include_stats=true&"} {
				alias := requestJSON(t, s, path+"window="+url.QueryEscape(pair[0]))
				canonical := requestJSON(t, s, path+"window="+pair[1])
				if path != "/api/stats?" {
					if !reflect.DeepEqual(alias["sessions"], canonical["sessions"]) {
						t.Fatal("alias selected different sessions")
					}
					alias = alias["stats"].(map[string]any)
					canonical = canonical["stats"].(map[string]any)
				}
				// Ignore only execution metadata. This compares denominators, complete
				// project bin arrays, charts, totals, heatmap, and normalized window.
				for _, key := range []string{"generated_at", "timing"} {
					delete(alias, key)
					delete(canonical, key)
				}
				if !reflect.DeepEqual(alias, canonical) {
					t.Fatalf("alias %s differs from %s: got %v want %v", pair[0], pair[1], alias, canonical)
				}
			}
		})
	}
}

func TestStatsRejectsUnusableIdentity(t *testing.T) {
	for _, key := range []string{"session_id", "id"} {
		for _, kind := range []string{"missing", "null", "empty", "whitespace"} {
			t.Run(key+"/"+kind, func(t *testing.T) {
				s, f := dashboardFixture(t)
				good := map[string]any{"id": "good", "session_id": "good", "end": float64(3)}
				bad := map[string]any{"id": "bad", "session_id": "bad", "end": float64(2), "cost": float64(2)}
				switch kind {
				case "missing":
					delete(bad, key)
				case "null":
					bad[key] = nil
				case "empty":
					bad[key] = ""
				case "whitespace":
					bad[key] = " \t"
				}
				f.rows["test-sessions"] = []map[string]any{good, bad}
				for _, path := range []string{"/api/stats?window=all", "/api/sessions?window=all&include_stats=true", "/api/values?window=all&facet=model"} {
					w := httptest.NewRecorder()
					s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
					if w.Code == 200 || !strings.Contains(w.Body.String(), "unusable session_id or physical id") {
						t.Fatalf("%s silently accepted malformed %s: status %d %s", path, key, w.Code, w.Body.String())
					}
				}
			})
		}
	}
}

// Real localhost transport, no archive access. Block at each source stage to
// prove cancellation reaches schema, session/eval queries and continuation.
func TestStatsLocalRequestCancellation(t *testing.T) {
	for _, path := range []string{"/api/stats?window=all", "/api/sessions?window=all&include_stats=true"} {
		t.Run(path, func(t *testing.T) {
			t.Run("pre", func(t *testing.T) {
				var calls atomic.Int32
				wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					http.Error(w, "unexpected source call", 500)
				}))
				defer wire.Close()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				w := httptest.NewRecorder()
				New(layer.New(wire.URL, "fixture", "test", "")).Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil).WithContext(ctx))
				if w.Code == 200 || calls.Load() != 0 {
					t.Fatalf("pre-cancelled request: status %d calls %d", w.Code, calls.Load())
				}
			})
			for _, stage := range []string{"schema", "sessions", "continuation", "evals"} {
				t.Run(stage, func(t *testing.T) {
					entered, observed := make(chan struct{}), make(chan struct{})
					var pages atomic.Int32
					wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasSuffix(r.URL.Path, "/scans/rows") {
							w.WriteHeader(422)
							fmt.Fprint(w, `{"error":"UnsupportedByStore","feature":"projected_source_rows"}`)
							return
						}
						_, _ = io.Copy(io.Discard, r.Body)
						current := "evals"
						if strings.HasSuffix(r.URL.Path, "/schema") {
							current = "schema"
						} else if strings.Contains(r.URL.Path, "-sessions/") {
							current = "sessions"
							if pages.Add(1) > 1 {
								current = "continuation"
							}
						}
						if current == stage {
							close(entered)
							select {
							case <-r.Context().Done():
								close(observed)
							case <-time.After(5 * time.Second):
								t.Error("source did not observe cancellation")
							}
							return
						}
						if current == "schema" {
							fmt.Fprint(w, `{"session_id":{},"end":{}}`)
							return
						}
						if current == "sessions" && stage == "continuation" {
							rows := make([]map[string]any, 10000)
							for i := range rows {
								id := fmt.Sprintf("%05d", i)
								rows[i] = map[string]any{"id": id, "session_id": id, "end": 1}
							}
							_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
							return
						}
						fmt.Fprint(w, `{"rows":[{"id":"one","session_id":"one","end":1}]}`)
					}))
					defer wire.Close()
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					w := httptest.NewRecorder()
					done := make(chan struct{})
					go func() {
						defer close(done)
						New(layer.New(wire.URL, "fixture", "test", "")).Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil).WithContext(ctx))
					}()
					select {
					case <-entered:
					case <-time.After(5 * time.Second):
						t.Fatal("did not reach source stage")
					}
					cancel()
					select {
					case <-observed:
					case <-time.After(5 * time.Second):
						t.Fatal("source cancellation missing")
					}
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("local handler did not stop")
					}
					if w.Code == 200 {
						t.Fatalf("cancelled %s returned successful dashboard", stage)
					}
				})
			}
		})
	}
}

func TestStatsRejectsUnknownIdentityFromReader(t *testing.T) {
	// Readers without the Layer projection interface also must fail before
	// latest selection, rather than invent a physical-ID identity fallback.
	reader := &fakeReader{sessions: []trace.SessionRow{{ID: "one", Cost: 1}, {ID: "two", Cost: 2}}}
	for _, path := range []string{"/api/stats?window=all", "/api/sessions?window=all&include_stats=true"} {
		w := httptest.NewRecorder()
		New(reader).Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code == 200 || !strings.Contains(w.Body.String(), "unusable session_id") {
			t.Fatalf("unknown sessions accepted: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestStatsCancellationDoesNotPoisonSharedReader(t *testing.T) {
	s, _ := dashboardFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/stats?window=all", nil).WithContext(ctx))
	if w.Code == 200 {
		t.Fatal("cancelled request succeeded")
	}
	got := requestJSON(t, s, "/api/sessions?window=all&include_stats=true")
	if got["stats"].(map[string]any)["traces"] != float64(len(got["sessions"].([]any))) || len(got["sessions"].([]any)) == 0 {
		t.Fatal("cancelled context leaked to subsequent paired request")
	}
}
