package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

const mirrorTestKey = "hev_0123456789abcdefghijklmnopqrstuv"

func mirrorConfig(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv("HEV_KIT_MIRROR_ORDERS", "")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMirrorSignInWorkspaceIsolation(t *testing.T) {
	home := "https://main.example"
	client := "https://client.example"
	path := mirrorConfig(t, fmt.Sprintf("[kit]\napi=%q\nkey=%q\n[workspaces.isolated]\n[workspaces.client.kit]\napi=%q\nkey=%q\n[workspaces.archived]\narchived=true\n[workspaces.archived.kit]\napi=%q\nkey=%q\n", home, mirrorTestKey, client, mirrorTestKey, client, mirrorTestKey))
	for _, tc := range []struct{ name, api string }{
		{"hev", home}, {"", home}, {"client", client}, {"isolated", ""}, {"missing", ""}, {"archived", ""},
	} {
		got, ok := orderMirrorSignIn(tc.name)
		if ok != (tc.api != "") || got.API != tc.api {
			t.Errorf("workspace %q: got API %q, enabled %t", tc.name, got.API, ok)
		}
	}
	t.Setenv("HEV_KIT_MIRROR_ORDERS", "0")
	if _, ok := orderMirrorSignIn("client"); ok {
		t.Error("disabled workspace was enabled")
	}
	t.Setenv("HEV_KIT_MIRROR_ORDERS", "")
	if err := os.WriteFile(path, []byte("[workspaces.client]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := orderMirrorSignIn("hev"); ok {
		t.Error("missing [kit] was enabled")
	}
}

func TestOrderMirrorMetadataOnWire(t *testing.T) {
	sentinels := []string{
		mirrorTestKey,
		"hvl_0123456789abcdefghijklmnopqrstuv",
		"sk-or-v1-00000000111111112222222233333333",
		"op://vault/item/field",
		"/home/operator/private",
		"C:\\Users\\operator\\private",
	}
	text := "Choose SQLite " + strings.Join(sentinels, " ")
	start := time.Now().UTC().Truncate(time.Millisecond)
	m := Meta{Order: Order{Name: "mirror-test", Host: "oven", Repo: "hev/kit", Base: "v0.7", Harness: "codex"},
		Branch: "d/mirror-test", Start: start, TraceID: "0123456789abcdef0123456789abcdef"}
	result := Result{End: start.Add(time.Second), Question: text, Error: text, PR: "https://github.com/hev/kit/pull/123"}
	initial := mirrorOrder(m, Result{}, "running", text, start)
	terminal := mirrorOrder(m, result, "blocked", text, result.End)
	if initial == nil || terminal == nil {
		t.Fatal("snapshot scrub failed")
	}
	calls := make(chan kitapi.MirroredOrder, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/v1/orders/mirror" || r.Header.Get("Authorization") != "Bearer "+mirrorTestKey {
			t.Error("mirror method, path or bearer mismatch")
		}
		raw, _ := io.ReadAll(r.Body)
		for _, secret := range sentinels {
			if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "operator") {
				t.Error("secret or host path in mirror body")
			}
		}
		var order kitapi.MirroredOrder
		if err := json.Unmarshal(raw, &order); err != nil {
			t.Error(err)
		}
		calls <- order
		w.WriteHeader(204)
	}))
	defer server.Close()
	mirrorConfig(t, fmt.Sprintf("[kit]\napi=%q\nkey=%q\n", server.URL, mirrorTestKey))
	signIn, _ := orderMirrorSignIn("hev")
	watch := mirrorWatch{Order: *initial, Workspace: "hev", Watch: true, Binding: signIn.fingerprint()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	watch.run(ctx, time.Millisecond, func(context.Context) (*kitapi.MirroredOrder, bool) { return terminal, true })
	for _, want := range []*kitapi.MirroredOrder{initial, terminal} {
		select {
		case got := <-calls:
			if got != *want || got.Repo != "hev/kit" || got.TraceID != m.TraceID || (got.State == "blocked" && (got.PRURL != result.PR || !strings.HasPrefix(got.Decision, "Choose SQLite") || !strings.HasPrefix(got.Error, "Choose SQLite"))) {
				t.Errorf("mirror metadata mismatch: state=%q repo=%q trace=%q PR=%q", got.State, got.Repo, got.TraceID, got.PRURL)
			}
		default:
			t.Errorf("missing %s snapshot", want.State)
		}
	}
}

func TestMirrorOrderHTTPFailuresAreBounded(t *testing.T) {
	for _, status := range []int{401, 402, 404, 503, 0} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			requests := make(chan struct{}, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				requests <- struct{}{}
				if status == 0 {
					select {
					case <-r.Context().Done():
					case <-time.After(7 * time.Second):
					}
					return
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			start := time.Now()
			err := kitapi.New(server.URL, mirrorTestKey).MirrorOrder(context.Background(), kitapi.MirroredOrder{})
			if err == nil || time.Since(start) > 6*time.Second || len(requests) != 1 {
				t.Fatalf("mirror error=%v, elapsed=%s, attempts=%d", err, time.Since(start), len(requests))
			}
		})
	}
}
