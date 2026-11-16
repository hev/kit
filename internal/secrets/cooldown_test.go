package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets/fakeop"
)

// A rate-limit answer, then five separate client processes with no daemon
// inside the cooldown: one 1Password request in total.
func TestNoDaemonClientProcessesShareCooldown(t *testing.T) {
	_, f, log := shellFake(t)
	t.Setenv(EndpointEnv, isolatedEndpoint(t)) // nothing serves it
	t.Setenv("FAKE_OP_RATE_LIMIT", time.Now().Add(3*time.Hour).UTC().Format(time.RFC3339))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ { // the first is answered with the limit
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), "HEV_TEST_CLIENT_CHILD=1")
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Fatalf("client %d: want the rate-limit answer, got %v", i, err)
		}
	}
	if n := f.Count(t); n != 1 {
		t.Fatalf("a rate limit and five more client processes made %d requests, want 1", n)
	}
	b, _ := os.ReadFile(log)
	if strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), "outcome=rate_limited") {
		t.Fatalf("the read log should hold the one rate-limited request")
	}
}

// The cooldown file holds only the limit class, reset time and strikes: no
// value, token or reference. A fresh cache honours it; past the reset one read
// goes through and clears it.
func TestCooldownFileIsSafeAndClearedAfterReset(t *testing.T) {
	c, f, log := shellFake(t)
	now := time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	if _, err := c.ResolveAs(context.Background(), "test", refs()); err != nil {
		t.Fatal(err) // a value is cached first, so a leak would have one to leak
	}
	t.Setenv("FAKE_OP_RATE_LIMIT", "2026-11-11T21:30:00Z")
	fresh := []Ref{{"FRESH", "op://example/fresh/credential", time.Minute}}
	if _, err := c.ResolveAs(context.Background(), "test", fresh); !IsRateLimit(err) {
		t.Fatalf("want rate limit, got %v", err)
	}
	p := CooldownPath()
	if p != filepath.Join(filepath.Dir(log), "op-cooldown.json") {
		t.Fatalf("cooldown should sit beside the read log, got %s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("cooldown file mode %v, want 0600", st.Mode().Perm())
	}
	for _, bad := range []string{fakeop.Value, sentinel, "op://", "example", "fake-service-account-token", "TOKEN", "FRESH", "credential"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("cooldown file holds %q", bad)
		}
	}
	var fields map[string]any
	if json.Unmarshal(b, &fields) != nil {
		t.Fatal("cooldown file is not JSON")
	}
	for k := range fields {
		if k != "limit" && k != "reset" && k != "strikes" {
			t.Fatalf("cooldown file has unexpected field %q", k)
		}
	}
	// A fresh cache (a restarted process) honours it without a request.
	t.Setenv("FAKE_OP_RATE_LIMIT", "")
	other := New()
	other.now = c.now
	if _, err := other.ResolveAs(context.Background(), "restart", refs()); !IsRateLimit(err) {
		t.Fatalf("fresh cache ignored the cooldown: %v", err)
	}
	if s := other.LocalStatus(); s.LastLimit != "daily rate limit" || s.Reset.Before(time.Date(2026, 11, 11, 21, 30, 0, 0, time.UTC)) {
		t.Fatalf("status should show the host cooldown, got %q %v", s.LastLimit, s.Reset)
	}
	if n := f.Count(t); n != 2 {
		t.Fatalf("want 2 requests (one ok, one limited), got %d", n)
	}
	// Past the reset: one read goes through and the success clears the file.
	now = time.Date(2026, 11, 11, 21, 31, 0, 0, time.UTC)
	another := New()
	another.now = c.now
	if _, err := another.ResolveAs(context.Background(), "after reset", fresh); err != nil {
		t.Fatal(err)
	}
	if f.Count(t) != 3 {
		t.Fatal("want exactly one request after the reset")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("first success after the reset must clear the cooldown file")
	}
}

// A corrupt or out-of-bounds cooldown cannot block reads for longer than a
// limit's window.
func TestCooldownFileOutOfBoundsIgnored(t *testing.T) {
	c, f, _ := shellFake(t)
	now := time.Now()
	for _, body := range []string{
		`{"limit":"daily rate limit","reset":"` + now.Add(72*time.Hour).UTC().Format(time.RFC3339) + `"}`,
		`{"limit":"forever","reset":"` + now.Add(time.Hour).UTC().Format(time.RFC3339) + `"}`,
		`not json`,
	} {
		os.MkdirAll(filepath.Dir(CooldownPath()), 0o700)
		os.WriteFile(CooldownPath(), []byte(body), 0o600)
		if _, err := c.Resolve(context.Background(), []Ref{{"X", "op://example/x/y", 0}}); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
	}
	if f.Count(t) != 3 {
		t.Fatal("each read should have gone through")
	}
}
