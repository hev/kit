package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/exitcode"
)

// TestExitCodes runs a built binary, since the status is only decided in
// main: usage mistakes exit 2, missing things 4, success 0, and a failure
// writes nothing to stdout.
func TestExitCodes(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "hev")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/threads/404":
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"thread 404 not found"}`))
		case "/api/threads/403":
			w.WriteHeader(403)
			w.Write([]byte(`{"error":"read-only board"}`))
		case "/api/threads/409":
			w.WriteHeader(409)
			w.Write([]byte(`{"error":"thread is locked"}`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer board.Close()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "hev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "hev", "dispatch.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, string, int) {
		t.Helper()
		c := exec.Command(binary, args...)
		c.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
			"FACTORY_BOARD_URL="+board.URL, "HEVLAYER_API_KEY=", "HEVLAYER_API_KEY_REF=", "PATH="+os.Getenv("PATH"))
		var stdout, stderr bytes.Buffer
		c.Stdout, c.Stderr = &stdout, &stderr
		err := c.Run()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return stdout.String(), stderr.String(), code
	}
	for _, tc := range []struct {
		args   []string
		code   int
		stderr []string
	}{
		{[]string{"bogus"}, exitcode.Usage, []string{`unknown command "bogus" for "hev"`, "Run 'hev --help' for usage."}},
		{[]string{"boadr"}, exitcode.Usage, []string{`Did you mean "board"?`}},
		{[]string{"--no-such-flag"}, exitcode.Usage, []string{"unknown flag: --no-such-flag"}},
		{[]string{"version", "extra"}, exitcode.Usage, []string{"Run 'hev version --help' for usage."}},
		{[]string{"traces", "show"}, exitcode.Usage, []string{"accepts 1 arg(s), received 0", "Run 'hev traces show --help' for usage."}},
		{[]string{"board", "read"}, exitcode.Usage, []string{"which thread?", "Run 'hev board --help' for usage."}},
		{[]string{"board", "raed"}, exitcode.Usage, []string{`unknown command "raed"`}},
		{[]string{"board", "read", "--bogus"}, exitcode.Usage, []string{"unknown flag --bogus"}},
		{[]string{"board", "read", "nope-id"}, exitcode.NotFound, []string{"no thread nope-id"}},
		{[]string{"board", "read", "404"}, exitcode.NotFound, []string{"thread 404 not found"}},
		{[]string{"board", "read", "403"}, exitcode.Auth, []string{"read-only board"}},
		{[]string{"board", "read", "409"}, exitcode.Conflict, []string{"thread is locked"}},
		{[]string{"dispatch", "show", "nope-run"}, exitcode.NotFound, []string{"no run named nope-run", "hev dispatch ls --all"}},
		{[]string{"traces", "show", "deadbeefnope"}, exitcode.NotFound, []string{`no trace matching prefix "deadbeefnope"`}},
	} {
		stdout, stderr, code := run(tc.args...)
		if code != tc.code {
			t.Errorf("hev %v: exit %d, want %d\nstderr: %s", tc.args, code, tc.code, stderr)
		}
		if stdout != "" {
			t.Errorf("hev %v: error text on stdout: %q", tc.args, stdout)
		}
		for _, want := range tc.stderr {
			if !strings.Contains(stderr, want) {
				t.Errorf("hev %v: stderr %q lacks %q", tc.args, stderr, want)
			}
		}
		if strings.Contains(stderr, "Usage:") || strings.Count(stderr, strings.SplitN(stderr, "\n", 2)[0]) != 1 {
			t.Errorf("hev %v: usage block or repeated error on stderr:\n%s", tc.args, stderr)
		}
	}

	stdout, stderr, code := run("version")
	if code != 0 || stderr != "" || strings.Count(stdout, "\n") != 1 || strings.TrimSpace(stdout) == "" {
		t.Errorf("hev version: %d %q %q", code, stdout, stderr)
	}
	plain := strings.TrimSpace(stdout)
	stdout, _, code = run("version", "--json")
	var info versionInfo
	if code != 0 || json.Unmarshal([]byte(stdout), &info) != nil || info.Version != plain || info.OS != runtime.GOOS ||
		info.Arch != runtime.GOARCH || info.GoVersion == "" || info.Channel == "" || info.Commit == "" {
		t.Errorf("hev version --json: %d %q", code, stdout)
	}
}

func TestReportErrorKeepsCommandExitSilent(t *testing.T) {
	var out bytes.Buffer
	if code := reportError(&out, rootCmd, &commandExit{code: 76}); code != 76 || out.Len() != 0 {
		t.Fatalf("commandExit: %d %q", code, out.String())
	}
}
