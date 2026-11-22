package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runHevChild runs this test binary as hev (see TestMain) with an isolated
// HOME and no inherited provider keys.
func runHevChild(t *testing.T, home string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	base := []string{"HEV_TEST_AS_HEV=1", "HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH"), "TMUX_TMPDIR=" + t.TempDir()}
	if v := os.Getenv("SYSTEMROOT"); v != "" {
		base = append(base, "SYSTEMROOT="+v)
	}
	cmd.Env = append(base, env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code = 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errOut.String(), code
}

func TestFOHShowMascot(t *testing.T) {
	home := t.TempDir()
	out, errOut, code := runHevChild(t, home, nil, "foh", "--show-mascot")
	if code != 0 || !strings.Contains(out, "\x1b[38;2;") || strings.Count(strings.TrimRight(out, "\n"), "\n") != 19 {
		t.Fatalf("default: code=%d stderr=%q lines=%d", code, errOut, strings.Count(out, "\n"))
	}
	for _, tc := range []struct {
		name string
		env  []string
		args []string
	}{
		{"--no-color", nil, []string{"foh", "--show-mascot", "--no-color"}},
		{"NO_COLOR", []string{"NO_COLOR=1"}, []string{"foh", "--show-mascot"}},
	} {
		out, errOut, code := runHevChild(t, home, tc.env, tc.args...)
		if code != 0 || strings.ContainsRune(out, '\x1b') || !strings.Contains(out, "@") {
			t.Fatalf("%s: code=%d out=%q stderr=%q", tc.name, code, out, errOut)
		}
	}
}

func TestFOHOnce(t *testing.T) {
	const key = "sk-or-v1-cmdtest000000000000000000000000"
	var gotModel, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		sc := bufio.NewScanner(r.Body)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			if i := strings.Index(sc.Text(), `"model":"`); i >= 0 {
				rest := sc.Text()[i+9:]
				gotModel = rest[:strings.IndexByte(rest, '"')]
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	home := t.TempDir()
	out, errOut, code := runHevChild(t, home, []string{"OPENROUTER_API_KEY=" + key, "HEV_FOH_BASE_URL=" + srv.URL},
		"foh", "--once", "ping", "--model", "test/model")
	if code != 0 || strings.TrimSpace(out) != "pong" {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, errOut)
	}
	if gotModel != "test/model" || gotAuth != "Bearer "+key {
		t.Fatalf("model=%q auth ok=%v", gotModel, gotAuth == "Bearer "+key)
	}
	sessions, _ := filepath.Glob(filepath.Join(home, ".hev", "foh", "sessions", "*.jsonl"))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v", sessions)
	}
	b, _ := os.ReadFile(sessions[0])
	if strings.Contains(string(b), key) || !strings.Contains(string(b), `"model":"test/model"`) {
		t.Fatalf("session = %s", b)
	}

	// No key: a clear error and a non-zero exit, no session.
	home2 := t.TempDir()
	_, errOut, code = runHevChild(t, home2, nil, "foh", "--once", "ping")
	if code == 0 || !strings.Contains(errOut, "OPENROUTER_API_KEY") {
		t.Fatalf("no key: code=%d stderr=%q", code, errOut)
	}
}
