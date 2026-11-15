package daemon

import (
	"context"
	"fmt"
	"github.com/hev/kit/pkg/kitapi"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReviewWhitespace(t *testing.T) {
	t.Setenv("HEV_API_KEY", " \t ")
	got := ResolveAPIKey("stored")
	if got != "stored" {
		t.Fatalf("whitespace overrides stored credential")
	}
}
func TestReviewWidePermissions(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HEV_CONFIG", d+"/config.toml")
	t.Setenv("HEV_API_KEY", "")
	os.WriteFile(DefaultConfigPath(), []byte("[kit]\napi='http://127.0.0.1:1'\nkey='synthetic-review-secret'\n"), 0644)
	os.Chmod(DefaultConfigPath(), 0644)
	_, err := ReadKitConfig()
	i, _ := os.Stat(DefaultConfigPath())
	if err == nil && i.Mode().Perm() != 0600 {
		t.Fatalf("existing credential config accepted with mode %o", i.Mode().Perm())
	}
}
func TestReviewErrorLeaks(t *testing.T) {
	key := "synthetic-review-secret"
	for _, status := range []int{400, 401, 403, 302, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if status == 302 {
					w.Header().Set("Location", "http://127.0.0.1/?key="+key)
				}
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":"invalid key %s"}`, key)
			}))
			defer s.Close()
			c := kitapi.New(s.URL, key)
			c.Retries = 0
			_, err := c.Whoami(context.Background())
			if err != nil && strings.Contains(err.Error(), key) {
				t.Fatal("credential reached error output")
			}
		})
	}
}
func TestReviewAdoptionOutage(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HEV_CONFIG", d+"/config.toml")
	t.Setenv("HEV_API_KEY", "new-synthetic-key")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer s.Close()
	raw := []byte("[kit]\napi='" + s.URL + "'\nkey='stored'\nteam_id='old-team'\n")
	os.WriteFile(DefaultConfigPath(), raw, 0600)
	_, err := ReadKitConfig()
	after, _ := os.ReadFile(DefaultConfigPath())
	if err != nil || string(after) != string(raw) {
		t.Fatal("outage mutated config")
	}
}

func TestReviewTrimEnvironment(t *testing.T) {
	t.Setenv("HEV_API_KEY", " \t real-fixture \n")
	if ResolveAPIKey("stored") != "real-fixture" {
		t.Fatal("environment was not normalized")
	}
}
func TestReviewAdoptionPreservesEditsDuringVerification(t *testing.T) {
	d := t.TempDir()
	t.Setenv("HOME", d)
	t.Setenv("HEV_CONFIG", d+"/config.toml")
	t.Setenv("HEV_API_KEY", "synthetic-new")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(DefaultConfigPath(), []byte("unrelated = 'concurrent edit'\n"), 0600); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, "{}")
	}))
	defer s.Close()
	t.Setenv("HEV_KIT_SERVICE", s.URL)
	if _, err := ReadKitConfig(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(DefaultConfigPath())
	if !strings.Contains(string(raw), "concurrent edit") {
		t.Fatal("lost concurrent edit")
	}
}
