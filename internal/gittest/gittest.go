// Package gittest serves bare repositories over authenticated HTTPS for tests.
package gittest

import (
	"encoding/pem"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	User     = "bot"
	Password = "host-only-secret"
)

// Server is an HTTPS git server rooted at Root. Paths under /redirect/ answer
// with a redirect to the same repository without that prefix.
type Server struct {
	URL  string // https://127.0.0.1:PORT
	Root string
	CA   string // PEM file trusting the server certificate
}

func Serve(t *testing.T) Server {
	t.Helper()
	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	backend := filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend")
	if _, err = os.Stat(backend); err != nil {
		t.Skipf("git-http-backend unavailable: %v", err)
	}
	root := t.TempDir()
	h := &cgi.Handler{Path: backend, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rest, ok := strings.CutPrefix(r.URL.Path, "/redirect/"); ok {
			http.Redirect(w, r, "/"+rest+"?"+r.URL.RawQuery, http.StatusFound)
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != User || p != Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return Server{URL: srv.URL, Root: root, CA: ca}
}

// HostConfig writes the host account's own git trust and credentials into
// home: the server CA and a stored credential for it.
func (s Server) HostConfig(t *testing.T, home string) {
	t.Helper()
	// The empty helper drops system helpers (Homebrew git's osxkeychain
	// blocks without a desktop session), leaving only the host's store.
	cfg := "[http]\n\tsslCAInfo = " + filepath.ToSlash(s.CA) + "\n[credential]\n\thelper =\n\thelper = store\n[user]\n\tname = Host\n\temail = host@example.test\n[init]\n\tdefaultBranch = main\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	cred := strings.Replace(s.URL, "https://", "https://"+User+":"+Password+"@", 1) + "\n"
	if err := os.WriteFile(filepath.Join(home, ".git-credentials"), []byte(cred), 0600); err != nil {
		t.Fatal(err)
	}
}

// Repo creates ROOT/path as a bare repository with one commit on main.
func (s Server) Repo(t *testing.T, path string, files map[string]string) {
	t.Helper()
	work := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run(work, "init", "-q", "-b", "main")
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(work, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "add", "-A")
	run(work, "commit", "-q", "-m", "initial")
	bare := filepath.Join(s.Root, filepath.FromSlash(path))
	run(work, "clone", "-q", "--bare", work, bare)
}
