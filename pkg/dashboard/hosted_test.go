package dashboard_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/pkg/dashboard"
)

func TestTenantsAcrossDashboardRoutes(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer key-")
		if tenant != "alice" && tenant != "bob" {
			t.Errorf("unexpected credential %q", r.Header.Get("Authorization"))
			w.WriteHeader(403)
			return
		}
		base := "/v2/namespaces/kit-" + tenant + "-traces"
		if !strings.HasPrefix(r.URL.Path, base+"/") && !strings.HasPrefix(r.URL.Path, base+"-") {
			t.Errorf("key/namespace mismatch: %s %s", tenant, r.URL.Path)
			w.WriteHeader(403)
			return
		}
		mu.Lock()
		seen[tenant+strings.TrimPrefix(r.URL.Path, base)] = true
		mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var rows []any
		id := "shared-session" // Same session ID in both tenants catches cache leakage.
		switch {
		case strings.HasSuffix(r.URL.Path, "-sessions/query"):
			if !strings.Contains(string(body), "foreign-session") {
				rows = []any{map[string]any{"id": id, "session_id": id, "summary": tenant + " archive", "host": tenant, "start": time.Now().UnixMilli(), "end": time.Now().UnixMilli()}}
			}
		case strings.HasSuffix(r.URL.Path, "-blocks/query"):
			rows = []any{map[string]any{"id": "block", "session_id": id, "role": "user", "block_type": "text", "text": tenant + " secret", "turn_uuid": "turn"}}
		case strings.HasSuffix(r.URL.Path, "-evals/query"):
			// Return evals on list and detail; distinguish eval search by its queries.
			if strings.Contains(string(body), "queries") {
				json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"rows": []any{}}}})
				return
			}
			rows = []any{map[string]any{"id": "eval", "session_id": id, "ts": time.Now().UTC().Format(time.RFC3339), "marks": "{}", "evidence": "{}", "findings": "[]"}}
		case strings.HasSuffix(r.URL.Path, "/query"):
			text := "alice own semantic noise"
			if tenant == "bob" {
				text = "bob secret"
			}
			hits := []any{map[string]any{"id": "hit", "session_id": id, "text": text, "$dist": 1}}
			if !strings.Contains(string(body), "queries") {
				var query map[string]any
				json.Unmarshal(body, &query)
				if fmt.Sprint(query["rank_by"]) != "[id asc]" || !strings.Contains(string(body), "session_id") {
					t.Errorf("phrase scan wire: %s", body)
				}
				json.NewEncoder(w).Encode(map[string]any{"rows": hits})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"rows": hits}}})
			return
		default:
			t.Errorf("unexpected gateway path %s", r.URL.Path)
		}
		if rows == nil {
			rows = []any{}
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	}))
	defer gateway.Close()
	handler, err := dashboard.NewHosted(dashboard.Config{Endpoint: gateway.URL}, func(r *http.Request) (dashboard.Credentials, error) {
		cookie, err := r.Cookie("session")
		if err != nil {
			return dashboard.Credentials{}, err
		}
		return dashboard.Credentials{Key: "key-" + cookie.Value, Namespace: "kit-" + cookie.Value + "-traces"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/api/sessions?window=all", "/api/session/shared-session", "/api/stats?window=all", "/api/values?facet=host&window=all", "/api/search?q=bob+secret&window=all", "/api/search?q=bob+secret&window=all&mode=phrase"}
	check := func(tenant, path string) {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: tenant})
		// These untrusted selectors must not choose a different gateway identity.
		r.Header.Set("Authorization", "Bearer global-admin")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("%s %s: %d %s", tenant, path, w.Code, w.Body.String())
			return
		}
		other := "bob"
		if tenant == "bob" {
			other = "alice"
		}
		var payload map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Error(err)
		}
		// Search echoes the query, so inspect results separately.
		data := w.Body.String()
		if strings.HasPrefix(path, "/api/search") {
			encoded, _ := json.Marshal(payload["sessions"])
			data = string(encoded)
			if tenant == "alice" && strings.Contains(path, "mode=phrase") && data != "[]" {
				t.Errorf("alice found bob's phrase: %s", data)
			}
			if tenant == "alice" && !strings.Contains(path, "mode=phrase") && !strings.Contains(data, "alice own semantic noise") {
				t.Errorf("default hybrid lost own semantic hit: %s", data)
			}
			if tenant == "bob" && !strings.Contains(data, "bob secret") {
				t.Errorf("bob search missing result: %s", data)
			}
		}
		if strings.Contains(data, other+" secret") || strings.Contains(data, other+" archive") || strings.Contains(data, `"`+other+`"`) {
			t.Errorf("tenant leak: %s", data)
		}
		if strings.Contains(data, "key-") {
			t.Error("credential exposed")
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Error("cacheable tenant response")
		}
	}
	// Repeat sequentially, then overlap requests to catch shared caches and mutation.
	for i := 0; i < 2; i++ {
		for _, tenant := range []string{"alice", "bob"} {
			for _, p := range paths {
				check(tenant, p)
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		for _, tenant := range []string{"alice", "bob"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, p := range paths {
					check(tenant, p)
				}
			}()
		}
	}
	wg.Wait()
	for _, tenant := range []string{"alice", "bob"} {
		for _, suffix := range []string{"-sessions/query", "-blocks/query", "-evals/query", "/query"} {
			if !seen[tenant+suffix] {
				t.Errorf("not exercised %s %s", tenant, suffix)
			}
		}
	}
	r := httptest.NewRequest("GET", "/api/session/foreign-session", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "alice"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("foreign detail: %d", w.Code)
	}
}

func TestAuthenticationFailsClosed(t *testing.T) {
	for _, cred := range []dashboard.Credentials{{}, {Key: "key"}, {Namespace: "kit-a"}, {Key: "key", Namespace: "../bob"}, {Key: "key", Namespace: "kit-a?x=y"}} {
		h, err := dashboard.NewHosted(dashboard.Config{Endpoint: "http://127.0.0.1:1"}, func(*http.Request) (dashboard.Credentials, error) { return cred, nil })
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/", "/ui/components.js", "/api/sessions", "/api/session/x", "/api/search?q=x", "/api/values", "/api/stats", "/api/factory"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			if w.Code != 401 {
				t.Errorf("%s: %d", path, w.Code)
			}
		}
	}
	h, _ := dashboard.NewHosted(dashboard.Config{Endpoint: "http://127.0.0.1:1"}, func(*http.Request) (dashboard.Credentials, error) {
		return dashboard.Credentials{Key: "secret", Namespace: "kit-a"}, errors.New("private session secret")
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 401 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body.String())
	}
	for _, endpoint := range []string{"", "ftp://example.com", "https://user:secret@example.com", "https://example.com?key=x"} {
		if _, err := dashboard.NewHosted(dashboard.Config{Endpoint: endpoint}, func(*http.Request) (dashboard.Credentials, error) { return dashboard.Credentials{}, nil }); err == nil {
			t.Error(fmt.Sprintf("accepted %q", endpoint))
		}
	}
}

func TestGatewayRedirectCannotForwardCredential(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed gateway redirect") }))
	defer destination.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer gateway.Close()
	h, err := dashboard.NewHosted(dashboard.Config{Endpoint: gateway.URL}, func(*http.Request) (dashboard.Credentials, error) {
		return dashboard.Credentials{Key: "scoped", Namespace: "kit-a"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil))
	if w.Code == 200 {
		t.Fatal("redirect accepted as data")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGatewayInheritsRequestCancellation(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "request"))
	cancel()
	var calls atomic.Int64
	h, err := dashboard.NewHosted(dashboard.Config{Endpoint: "https://gateway.example", Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Context().Value(contextKey{}) != "request" || !errors.Is(r.Context().Err(), context.Canceled) {
			t.Error("gateway lost request context")
		}
		return nil, r.Context().Err()
	})}, func(*http.Request) (dashboard.Credentials, error) {
		return dashboard.Credentials{Key: "scoped", Namespace: "kit-a"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions", nil).WithContext(ctx))
	if calls.Load() == 0 || w.Code == 200 {
		t.Fatal("cancellation not exercised")
	}
}
