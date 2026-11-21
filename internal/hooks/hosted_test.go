package hooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHostedHookSharesLocalDecisionAndBudget(t *testing.T) {
	ev := Event{Schema: Schema, ID: "event-id", Event: "pre-tool", Harness: "codex"}
	for _, tc := range []struct {
		name    string
		reply   string
		status  int
		want    string
		warning bool
	}{
		{"merge strongest decision", `{"event_id":"event-id","decision":"halt","reason":"policy","blocking_hooks":1,"elapsed_ms":1}`, 200, Halt, false},
		{"ignore mismatched event", `{"event_id":"other","decision":"halt","blocking_hooks":1,"elapsed_ms":1}`, 200, Block, true},
		{"fail open on redirect", "", 307, Block, true},
		{"fail open on timeout", "", 0, Block, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("redirect must never be followed")
			}))
			defer redirect.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var posted Event
				if r.Method != http.MethodPost || r.URL.Path != "/v1/hooks/events" ||
					r.Header.Get("Authorization") != "Bearer fake-key" ||
					json.NewDecoder(r.Body).Decode(&posted) != nil || posted != ev {
					t.Error("expected authenticated POST containing the normalized event")
				}
				if tc.status == 0 {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
						t.Error("hosted request was not cancelled")
					}
					return
				}
				w.Header().Set("Location", redirect.URL)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.reply))
			}))
			defer server.Close()

			var warnings []string
			local := Handler{Name: "local", Run: func(context.Context, Event) (Response, error) {
				return Response{Decision: Block, Context: "local context"}, nil
			}}
			got := Run(context.Background(), ev, []Handler{local, HostedHandler(func() (string, string) {
				return server.URL, "fake-key"
			})}, 500*time.Millisecond, func(w string) { warnings = append(warnings, w) })
			if got.Decision != tc.want || got.Context != "local context" || (len(warnings) > 0) != tc.warning {
				t.Fatalf("response = %+v, warnings = %v", got, warnings)
			}
			for _, warning := range warnings {
				if strings.Contains(warning, "fake-key") || strings.Contains(warning, server.URL) {
					t.Fatal("warning exposed a credential or URL")
				}
			}
		})
	}
}
