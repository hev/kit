package secrets

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets/fakeop"
)

// shellFake points a cache at the shell fake op on PATH, with an isolated
// read log, and returns both.
func shellFake(t *testing.T) (*Cache, fakeop.Fake, string) {
	t.Helper()
	f := fakeop.Install(t)
	log := filepath.Join(t.TempDir(), "op-reads.log")
	t.Setenv("HEV_SECRETS_READ_LOG", log)
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	return New(), f, log
}

func TestRateLimitOneAttemptThenNoRequestUntilReset(t *testing.T) {
	c, f, _ := shellFake(t)
	now := time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	reset := "2026-11-11T21:30:00Z"
	t.Setenv("FAKE_OP_RATE_LIMIT", reset)
	_, err := c.ResolveAs(context.Background(), "test", refs())
	if err == nil || !IsRateLimit(err) || !strings.Contains(err.Error(), "retry after 2026-11-11T21:30") {
		t.Fatalf("want a rate-limit answer with op's retry time, got %v", err)
	}
	if f.Count(t) != 1 {
		t.Fatalf("one attempt expected, op ran %d times", f.Count(t))
	}
	// Until the reset nothing reaches op, whatever callers ask for.
	t.Setenv("FAKE_OP_RATE_LIMIT", "")
	for _, step := range []time.Duration{time.Second, time.Hour, 8 * time.Hour} {
		now = time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC).Add(step)
		if _, err := c.ResolveAs(context.Background(), "test", refs()); err == nil || !strings.Contains(err.Error(), "retry after 2026-11-11T21:30") {
			t.Fatalf("cooldown not honoured at +%s: %v", step, err)
		}
		if s := c.Status(context.Background()); s.Requests != 1 {
			t.Fatalf("status made a quota request during cooldown")
		}
	}
	if f.Count(t) != 1 {
		t.Fatalf("cache retried during cooldown: op ran %d times", f.Count(t))
	}
	now = c.status.Reset.Add(time.Second)
	if _, err := c.ResolveAs(context.Background(), "test", refs()); err != nil {
		t.Fatal(err)
	}
	if f.Count(t) != 2 {
		t.Fatal("expected exactly one request after the reset")
	}
}

func TestUnclassifiedRateLimitBacksOff(t *testing.T) {
	c, f, _ := shellFake(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	t.Setenv("FAKE_OP_RATE_LIMIT", "soon") // no parseable time: "too many requests" only
	var gaps []time.Duration
	for i := 0; i < 4; i++ {
		if _, err := c.Resolve(context.Background(), refs()); !IsRateLimit(err) {
			t.Fatalf("want rate limit, got %v", err)
		}
		gaps = append(gaps, c.status.Reset.Sub(now))
		now = c.status.Reset.Add(time.Second)
	}
	if f.Count(t) != 4 || gaps[3] < 7*time.Minute {
		t.Fatalf("unclassified limits should back off exponentially, got %v", gaps)
	}
}

func TestReadLogOneLinePerRealRequestNoValues(t *testing.T) {
	c, f, log := shellFake(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ { // one miss, then two hits
		if _, err := c.ResolveAs(ctx, "op-env.sh:bash", refs()); err != nil {
			t.Fatal(err)
		}
	}
	c.Status(ctx) // one quota request
	c.Status(ctx) // within quotaEvery: none
	t.Setenv("FAKE_OP_RATE_LIMIT", "2099-01-01T00:00:00Z")
	c.ResolveAs(ctx, "with-op:claude", []Ref{{"FRESH", "op://example/fresh/token", 0}})
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if f.Count(t) != 3 || len(lines) != 7 {
		t.Fatalf("want one log line per real request (%d), got %d", f.Count(t), len(lines))
	}
	line := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ caller=\S+ kind=(inject|ratelimit|cache) refs=\d+ labels=\S+ outcome=(ok|error|rate_limited) provider=op$`)
	for _, l := range lines {
		if !line.MatchString(l) {
			t.Fatalf("malformed log line %q", l)
		}
		if strings.Contains(l, fakeop.Value) || strings.Contains(l, "op://") || strings.Contains(l, "fake-service-account-token") {
			t.Fatal("read log holds a value, token or reference")
		}
	}
	if !strings.Contains(lines[0], "caller=op-env.sh:bash") || !strings.Contains(lines[0], "labels=TOKEN#") || !strings.Contains(lines[6], "outcome=rate_limited") {
		t.Fatalf("missing caller, label or outcome: %q", lines)
	}
	if st, _ := os.Stat(log); st.Mode().Perm() != 0o600 {
		t.Fatal("read log must be private")
	}
	if n, err := CountReadLog(time.Now().Add(-time.Hour)); err != nil || n != 3 {
		t.Fatalf("CountReadLog = %d, %v", n, err)
	}
}

func TestStatusQuotaRows(t *testing.T) {
	c, _, _ := shellFake(t)
	s := c.Status(context.Background())
	if len(s.Quota) != 2 || s.Quota[1].Type != "account" || *s.Quota[1].Remaining != 0 || s.Quota[1].Reset != "2026-11-11T21:30:00Z" || s.Quota[0].Reset == "" {
		t.Fatalf("quota rows: %+v", s.Quota)
	}
	if s.Requests != 1 || s.StartedAt.IsZero() {
		t.Fatal("status must count the quota request")
	}
}

func TestTokenFileServesWhenEnvironmentHasNone(t *testing.T) {
	c, f, _ := shellFake(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	tok := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tok, []byte("fake-file-token\n"), 0o644)
	t.Setenv("HEV_OP_TOKEN_FILE", tok)
	if _, err := c.Resolve(context.Background(), refs()); err == nil {
		t.Fatal("a group-readable token file must be refused")
	}
	os.Chmod(tok, 0o600)
	if _, err := c.Resolve(context.Background(), refs()); err != nil {
		t.Fatal(err)
	}
	if f.Count(t) != 1 || accountID() == "" {
		t.Fatal("token file not used")
	}
}
