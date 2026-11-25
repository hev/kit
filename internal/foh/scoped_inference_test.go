package foh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScopedInferenceRoutesIndependently(t *testing.T) {
	keyA := "hev_" + strings.Repeat("A", 32)
	keyB := "hvl_" + strings.Repeat("B", 32)
	rotated := "hev_" + strings.Repeat("R", 32)
	const refA, refB = "secret://alpha-key", "secret://beta-key"
	const modelA, modelB = "scope/alpha/test/model", "scope/beta/test/model"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mu sync.Mutex
	keys := map[string]string{refA: keyA, refB: keyB}
	identities := map[string][2]string{keyA: {"user-1", "team-1"}, keyB: {"user-1", "team-2"}, rotated: {"user-1", "team-1"}}
	refused := map[string]bool{}
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		mu.Lock()
		identity, active := identities[key]
		forbidden := refused[key]
		requests[r.URL.Path]++
		mu.Unlock()
		if !active {
			http.Error(w, key, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/whoami":
			fmt.Fprintf(w, `{"author":{"id":%q},"team":{"id":%q}}`, identity[0], identity[1])
		case "/v1/inference/models":
			fmt.Fprintf(w, `{"data":[{"id":"test/model","name":%q}]}`, key)
		case "/v1/inference/chat/completions":
			if forbidden {
				http.Error(w, key, http.StatusForbidden)
				return
			}
			var request ChatRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Model != "test/model" {
				t.Error("inference did not receive the upstream model")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []string{identity[1] + " ", key[:12], key[12:]} {
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", part)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			t.Error("unexpected hosted route")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("HEV_API_KEY", rotated)
	t.Setenv("HEV_FOH_BASE_URL", "https://openrouter.ai")
	t.Setenv("HEV_KIT_SERVICE", "https://openrouter.ai")
	config := `[kit]
api = %q
[[foh.key_scopes]]
name = "alpha"
account = "user-1"
workspace = "team-1"
api_key_ref = "secret://alpha-key"
[[foh.key_scopes]]
name = "beta"
account = "user-1"
workspace = "team-2"
api_key_ref = "secret://beta-key"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(config, server.URL)), 0600); err != nil {
		t.Fatal(err)
	}
	restart := func() *Router {
		t.Helper()
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		provider, err := cfg.ScopedInference()
		if err != nil {
			t.Fatal(err)
		}
		provider.(*scopedInference).resolve = func(_ context.Context, ref string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if key := keys[ref]; key != "" {
				return key, nil
			}
			return "", errors.New("resolver failed with a secret: " + keyA)
		}
		return NewRouter(provider)
	}
	router := restart()
	log, err := OpenSession(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	agent := NewAgent(router, nil, modelA, "")
	agent.Session = log
	for _, selection := range []struct{ model, workspace string }{{modelA, "team-1"}, {modelB, "team-2"}} {
		agent.SetModel(selection.model)
		var output strings.Builder
		err := agent.Run(ctx, "ping", func(event Event) {
			if event.Kind == EventText {
				output.WriteString(event.Text)
			}
		})
		if err != nil || output.String() != selection.workspace+" [REDACTED]" {
			t.Fatal("same session did not use the selected scope or leaked streamed text")
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(log.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{keyA, keyB, rotated} {
		if strings.Contains(string(trace), key) {
			t.Fatal("session trace contained a credential")
		}
	}

	stream := func(router *Router, model, want string) {
		t.Helper()
		res, err := router.Stream(ctx, ChatRequest{Model: model}, nil)
		if err != nil || res.Content != want+" [REDACTED]" {
			t.Error("request did not use the selected scope")
		}
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() { stream(router, modelA, "team-1") })
		workers.Go(func() { stream(router, modelB, "team-2") })
	}
	workers.Wait()

	assertRefused := func(model, message string, inferenceCalls int) {
		t.Helper()
		mu.Lock()
		before := requests["/v1/inference/chat/completions"]
		mu.Unlock()
		_, err := router.Stream(ctx, ChatRequest{Model: model}, nil)
		if err == nil || !strings.Contains(err.Error(), message) || strings.Contains(err.Error(), keyA) || strings.Contains(err.Error(), keyB) {
			t.Fatalf("expected scrubbed refusal containing %q", message)
		}
		mu.Lock()
		after := requests["/v1/inference/chat/completions"]
		mu.Unlock()
		if after != before+inferenceCalls {
			t.Fatal("refused request was rerouted")
		}
		stream(router, modelB, "team-2")
	}
	assertRefused("test/model", "select a configured account", 0)
	assertRefused("scope/foreign/test/model", "select a configured account", 0)
	mu.Lock()
	keys[refA] = keyB
	mu.Unlock()
	assertRefused(modelA, "does not match", 0)
	mu.Lock()
	delete(keys, refA)
	mu.Unlock()
	assertRefused(modelA, "could not resolve", 0)
	mu.Lock()
	keys[refA] = keyA
	refused[keyA] = true
	mu.Unlock()
	assertRefused(modelA, "missing inference entitlement", 1)
	mu.Lock()
	delete(identities, keyA)
	mu.Unlock()
	assertRefused(modelA, "refused scope verification", 0)
	partial, _, err := router.Models(ctx, true)
	if err == nil || len(partial) != 1 || partial[0].ID != modelB {
		t.Fatal("revoked scope hid or replaced the sibling catalog")
	}
	mu.Lock()
	keys[refA] = rotated
	mu.Unlock()
	for _, r := range []*Router{router, restart()} {
		stream(r, modelA, "team-1")
		stream(r, modelB, "team-2")
	}
	models, _, err := router.Models(ctx, true)
	if err != nil || len(models) != 2 || models[0].ID != modelA || models[1].ID != modelB ||
		!strings.HasPrefix(models[0].Name, "[REDACTED]") || !strings.HasPrefix(models[1].Name, "[REDACTED]") {
		t.Fatal("catalog did not keep model identities separate or scrub credentials")
	}
}

func TestScopedInferenceRejectsAmbiguousConfig(t *testing.T) {
	a := KeyScope{Name: "alpha", Account: "user-1", Workspace: "team-1", APIKeyRef: "secret://alpha-key"}
	b := KeyScope{Name: "beta", Account: "user-1", Workspace: "team-2", APIKeyRef: "secret://beta-key"}
	for _, other := range []KeyScope{
		{Name: a.Name, Account: b.Account, Workspace: b.Workspace, APIKeyRef: b.APIKeyRef},
		{Name: b.Name, Account: a.Account, Workspace: a.Workspace, APIKeyRef: b.APIKeyRef},
		{Name: b.Name, Account: b.Account, Workspace: b.Workspace, APIKeyRef: a.APIKeyRef},
	} {
		cfg := Config{KeyScopes: []KeyScope{a, other}}
		if _, err := cfg.ScopedInference(); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatal("ambiguous key scope configuration was accepted")
		}
	}
	cfg := Config{KeyScopes: []KeyScope{a, b}}
	if _, err := cfg.ScopedInference(); err != nil {
		t.Fatal("independent scopes rejected")
	}
}
