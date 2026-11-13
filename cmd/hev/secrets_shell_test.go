//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// TestMain lets a shell script call this test binary as `hev`: a symlink named
// hev and HEV_TEST_AS_HEV=1 run main() with the script's arguments.
func TestMain(m *testing.M) {
	if os.Getenv("HEV_TEST_AS_HEV") == "1" {
		if p := os.Getenv("HEV_TEST_ARGV_LOG"); p != "" {
			if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				f.WriteString(strings.Join(os.Args, " ") + "\n")
				f.Close()
			}
		}
		os.Args[0] = "hev"
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type shellHost struct {
	root, home, log, argv string
	op                    fakeop.Fake
	env                   []string
}

// newShellHost builds an isolated host: fake op, this binary as hev, a private
// HOME, TMPDIR and read log, and a fake service-account token. withHev false
// leaves hev off PATH.
func newShellHost(t *testing.T, withHev bool) *shellHost {
	t.Helper()
	f := fakeop.Install(t)
	root, err := os.MkdirTemp("/tmp", "kitsh") // short: Unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	h := &shellHost{root: root, home: filepath.Join(root, "home"), log: filepath.Join(root, "op-reads.log"), argv: filepath.Join(root, "argv"), op: f}
	os.MkdirAll(filepath.Join(h.home, "tmp"), 0o700)
	path := f.Bin + ":/usr/bin:/bin"
	if withHev {
		bin := filepath.Join(root, "hevbin")
		os.Mkdir(bin, 0o700)
		exe, _ := os.Executable()
		if err := os.Symlink(exe, filepath.Join(bin, "hev")); err != nil {
			t.Fatal(err)
		}
		path = bin + ":" + path
	}
	vars := map[string]string{
		"HOME": h.home, "TMPDIR": filepath.Join(h.home, "tmp"), "PATH": path,
		"HEV_SECRETS_READ_LOG": h.log, "HEV_TEST_AS_HEV": "1", "HEV_TEST_ARGV_LOG": h.argv,
		"OP_SERVICE_ACCOUNT_TOKEN": "fake-service-account-token", "HEV_OP_TOKEN_FILE": filepath.Join(root, "absent"),
	}
	for k, v := range vars {
		t.Setenv(k, v)
		h.env = append(h.env, k+"="+v)
	}
	h.env = append(h.env, "FAKE_OP_CALLS="+f.Calls)
	return h
}

// serve runs the host's secrets daemon in this process until the test ends.
func (h *shellHost) serve(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(); cancel() })
}

func (h *shellHost) sh(t *testing.T, shell, script string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(shell, append([]string{"-c", script, shell}, args...)...)
	cmd.Env = h.env
	cmd.Dir = h.root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// noValueAnywhere fails if the fake value is in any file under the host root
// or in any recorded process argument list (hev's and op's).
func (h *shellHost) noValueAnywhere(t *testing.T) {
	t.Helper()
	filepath.WalkDir(h.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 || d.Type()&fs.ModeSocket != 0 {
			return nil
		}
		if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(fakeop.Value)) {
			t.Fatalf("value written to disk: %s", p)
		}
		return nil
	})
	for _, l := range h.op.CallLines(t) {
		if strings.Contains(l, fakeop.Value) {
			t.Fatal("value in op arguments")
		}
	}
}

func repoFile(t *testing.T, rel string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", rel)
}

func TestSecretsReadShellCallSite(t *testing.T) {
	h := newShellHost(t, true)
	h.serve(t)
	script := `x=$(hev secrets read --label DB_PASSWORD --ttl 1h op://example/db/password) || exit 9; printf '%s' "$x"`
	for i := 0; i < 2; i++ {
		out, errs, err := h.sh(t, "sh", script)
		if err != nil || out != fakeop.Value {
			t.Fatalf("call %d: value not returned to the caller (%v) %s", i+1, err, errs)
		}
	}
	if h.op.Count(t) != 1 {
		t.Fatalf("second call inside the TTL made a request: op ran %d times", h.op.Count(t))
	}
	argv, _ := os.ReadFile(h.argv)
	if strings.Count(string(argv), "secrets read") != 2 {
		t.Fatal("hev was not invoked as expected")
	}
	h.noValueAnywhere(t)
	log, _ := os.ReadFile(h.log)
	if !strings.Contains(string(log), "labels=DB_PASSWORD#") || strings.Count(string(log), "\n") != 2 {
		t.Fatalf("read log: %q", log)
	}
}

func TestSecretsReadRateLimitExit(t *testing.T) {
	h := newShellHost(t, true)
	h.serve(t)
	// The daemon runs in this process, so its op sees this test's environment.
	t.Setenv("FAKE_OP_RATE_LIMIT", "2099-01-01T00:00:00Z")
	for i := 0; i < 2; i++ {
		_, errs, err := h.sh(t, "sh", `hev secrets read op://example/db/password`)
		e, ok := err.(*exec.ExitError)
		if !ok || e.ExitCode() != exitRateLimited || !strings.Contains(errs, "retry after") {
			t.Fatalf("want exit %d with retry time, got %v %q", exitRateLimited, err, errs)
		}
	}
	if h.op.Count(t) != 1 {
		t.Fatalf("rate limit retried: op ran %d times", h.op.Count(t))
	}
}

func TestSecretsEnvQuotesForEval(t *testing.T) {
	for _, v := range []string{"plain", "it's", "two\nlines", `$(touch x) "q" \`} {
		got, _, err := (&shellHost{env: []string{"PATH=/usr/bin:/bin", "V=" + shellQuote(v)}}).sh(t, "sh", `eval "X=$V"; printf '%s' "$X"`)
		if err != nil || got != v {
			t.Fatalf("quote round trip failed for %q: %q", v, got)
		}
	}
}

func TestStagedOpEnvExportsIntoCallingShell(t *testing.T) {
	script := repoFile(t, "staged/host-secrets/shell/op-env.sh")
	check := `. "$1"; env | grep '^EXAMPLE_' | sort`
	want := "EXAMPLE_ALIAS_CHANNEL_ID=" + fakeop.Value + "\nEXAMPLE_API_KEY=" + fakeop.Value + "\nEXAMPLE_CHANNEL_ID=" + fakeop.Value + "\n"
	for _, shell := range []string{"bash", "sh"} {
		t.Run(shell+"/daemon", func(t *testing.T) {
			h := newShellHost(t, true)
			h.serve(t)
			for i := 0; i < 2; i++ {
				out, errs, err := h.sh(t, shell, check, script)
				if err != nil || out != want {
					t.Fatalf("source %d: exports missing in calling shell: %v %q %q", i+1, err, out, errs)
				}
			}
			calls := h.op.CallLines(t)
			if len(calls) != 1 || calls[0] != "inject" {
				t.Fatalf("want one batch for both sources, got %q", calls)
			}
			if argv, _ := os.ReadFile(h.argv); strings.Count(string(argv), "secrets env --caller") != 2 {
				t.Fatal("op-env.sh did not go through hev secrets env")
			}
			h.noValueAnywhere(t)
		})
		t.Run(shell+"/no-hev", func(t *testing.T) {
			h := newShellHost(t, false)
			out, _, err := h.sh(t, shell, check, script)
			if err != nil || out != want {
				t.Fatalf("exports missing without hev: %v %q", err, out)
			}
			if calls := h.op.CallLines(t); len(calls) != 1 || calls[0] != "inject" {
				t.Fatalf("want one op inject batch, got %q", calls)
			}
			h.noValueAnywhere(t)
		})
	}
}

func TestStagedWithOpReadsThroughCache(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	h := newShellHost(t, true)
	h.serve(t)
	tok := filepath.Join(h.root, "token")
	os.WriteFile(tok, []byte("fake-service-account-token\n"), 0o600)
	h.env = append(h.env, "HEV_OP_TOKEN_FILE="+tok, "OP_SERVICE_ACCOUNT_TOKEN=")
	t.Setenv("HEV_OP_TOKEN_FILE", tok)
	withOp := repoFile(t, "staged/host-secrets/linux/with-op")
	for i := 0; i < 2; i++ {
		cmd := exec.Command(withOp, "op", "read", "op://example/claude/credential")
		cmd.Env = h.env
		out, err := cmd.Output()
		if err != nil || strings.TrimSuffix(string(out), "\n") != fakeop.Value {
			t.Fatalf("with-op op read %d failed: %v", i+1, err)
		}
	}
	cmd := exec.Command(withOp, "op", "read", "--no-newline", "op://example/claude/credential")
	cmd.Env = h.env
	if out, err := cmd.Output(); err != nil || string(out) != fakeop.Value {
		t.Fatalf("--no-newline: %v %q", err, out)
	}
	if h.op.Count(t) != 1 {
		t.Fatalf("with-op made %d requests, want one", h.op.Count(t))
	}
	cmd = exec.Command(withOp, "op", "whoami")
	cmd.Env = h.env
	cmd.Run() // the fake rejects whoami; with-op must pass it through and log it
	log, _ := os.ReadFile(h.log)
	if !strings.Contains(string(log), "caller=with-op:") || !strings.Contains(string(log), "kind=op-whoami") || strings.Contains(string(log), fakeop.Value) {
		t.Fatalf("with-op log: %q", log)
	}
	h.noValueAnywhere(t)
}

// The CLI makes the three rework items visible to a wrapper: a configurable
// endpoint, per-reference dispositions without values, and an explicit
// fallback with a read-free ping.
func TestSecretsCLIEndpointDispositionAndFallback(t *testing.T) {
	h := newShellHost(t, true)
	ep := filepath.Join(h.root, "alt.sock")
	t.Setenv(secrets.EndpointEnv, ep)
	h.env = append(h.env, secrets.EndpointEnv+"="+ep)

	// No daemon: ping is free and exits 69; a read falls back with a notice
	// and, with --direct-status, exit 78 after delivering the value.
	out, _, err := h.sh(t, "sh", `hev secrets ping`)
	if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != exitUnavailable || !strings.Contains(out, "unavailable "+ep) {
		t.Fatalf("ping with no daemon: %v %q", err, out)
	}
	if h.op.Count(t) != 0 {
		t.Fatal("ping ran op")
	}
	out, errs, err := h.sh(t, "sh", `x=$(hev secrets read --direct-status --explain --label DB op://example/db/password); s=$?; printf '%s:%s' "$s" "$x"`)
	if err != nil || out != "78:"+fakeop.Value {
		t.Fatalf("direct read: %v %q", err, out)
	}
	if !strings.Contains(errs, secrets.FallbackNotice) || !strings.Contains(errs, "DB source=read via=direct") {
		t.Fatalf("fallback not explicit: %q", errs)
	}

	// Daemon on the overridden endpoint: ping exits 0, dispositions show a
	// read then a hit, and status --no-quota asks op for nothing.
	h.serve(t)
	before := h.op.Count(t)
	out, _, err = h.sh(t, "sh", `hev secrets ping`)
	if err != nil || strings.TrimSpace(out) != "up "+ep {
		t.Fatalf("ping: %v %q", err, out)
	}
	type detail struct {
		Values   map[string]string
		Sources  map[string]string
		Via      string
		Endpoint string
	}
	for i, want := range []string{"read", "cache"} {
		out, errs, err = h.sh(t, "sh", `hev secrets env --format json-detail --explain --direct-status --ref A=op://example/a/x --ttl A=1h`)
		var d detail
		if err != nil || json.Unmarshal([]byte(out), &d) != nil {
			t.Fatalf("env %d: %v %q", i, err, errs)
		}
		if d.Sources["A"] != want || d.Via != "daemon" || d.Endpoint != ep || d.Values["A"] != fakeop.Value {
			t.Fatalf("env %d: sources=%v via=%s", i, d.Sources, d.Via)
		}
		if strings.Contains(errs, secrets.FallbackNotice) || !strings.Contains(errs, "A source="+want+" via=daemon") || strings.Contains(errs, fakeop.Value) {
			t.Fatalf("env %d stderr: %q", i, errs)
		}
	}
	if _, errs, err = h.sh(t, "sh", `hev secrets status --no-quota`); err != nil {
		t.Fatalf("status --no-quota: %v %q", err, errs)
	}
	if n := h.op.Count(t) - before; n != 1 {
		t.Fatalf("want 1 request (the first env), got %d", n)
	}
	h.noValueAnywhere(t)
}

func TestSecretsBudgetExitAndNoDirectRetry(t *testing.T) {
	h := newShellHost(t, true)
	policy := `{"daily":1,"hourly":1,"caller_minute":1}`
	if e := os.WriteFile(filepath.Join(h.root, "secrets-policy.json"), []byte(policy), 0600); e != nil {
		t.Fatal(e)
	}
	h.serve(t)
	_, errs, e := h.sh(t, "sh", `hev secrets read --caller loop op://example/db/password >/dev/null; hev secrets read --caller loop op://example/db/user`)
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 76 || !strings.Contains(errs, "secrets budget:") {
		t.Fatalf("budget exit %v %s", e, errs)
	}
	if h.op.Count(t) != 1 {
		t.Fatal("budget response retried directly")
	}
	out, errs, e := h.sh(t, "sh", `hev secrets read --caller loop op://example/db/password`)
	if e != nil || out != fakeop.Value+"\n" {
		t.Fatalf("cached after budget %v %s", e, errs)
	}
}

func TestSecretsDirectProcessesShareBudget(t *testing.T) {
	h := newShellHost(t, true)
	if e := os.WriteFile(filepath.Join(h.root, "secrets-policy.json"), []byte(`{"daily":2,"hourly":2,"caller_minute":20}`), 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 5; i++ {
		_, errs, e := h.sh(t, "sh", fmt.Sprintf("hev secrets read --caller stable-worker op://example/item%d/token", i))
		if i < 2 {
			if e != nil {
				t.Fatalf("allowed read %v %s", e, errs)
			}
		} else {
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 76 {
				t.Fatalf("direct process bypass %v %s", e, errs)
			}
		}
	}
	if h.op.Count(t) != 2 {
		t.Fatalf("direct requests %d", h.op.Count(t))
	}
}
