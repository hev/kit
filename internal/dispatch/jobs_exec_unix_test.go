//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

func awaitExec(t *testing.T, name string) *Result {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, r := ExecOutcome(name)
		if state != "running" && state != "queued" && state != "absent" {
			if state == "unknown" {
				t.Fatal("unknown command outcome")
			}
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("command failed to settle")
	return nil
}
func TestJobCommandTimeoutKillsTreeAndRecordsFailure(t *testing.T) {
	jobHome(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	spec := ExecSpec{Argv: []string{"/bin/sh", "-c", `(sleep 0.7; echo leaked > "$1") & echo started; wait`, "stub", marker}, Cwd: dir, Deadline: time.Now().Add(250 * time.Millisecond), Job: "timeout-job", Occurrence: "j-timeout"}
	if e := StartExec("j-timeout", "local", spec, MinimalExecEnvironment()); e != nil {
		t.Fatal(e)
	}
	r := awaitExec(t, "j-timeout")
	if r == nil || !r.Timeout || r.ExitCode == 0 {
		t.Fatal("timeout not recorded", r)
	}
	time.Sleep(800 * time.Millisecond)
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("descendant survived timeout")
	}
	log, e := os.ReadFile(filepath.Join(Root(), "j-timeout", "log"))
	if e != nil || !strings.Contains(string(log), "started") {
		t.Fatal("child did not launch", e)
	}
	if e := StartExec("j-timeout", "local", spec, MinimalExecEnvironment()); e == nil {
		t.Fatal("duplicate run admitted")
	}
}
func TestJobCommandStopUsesDispatch(t *testing.T) {
	jobHome(t)
	dir := t.TempDir()
	spec := ExecSpec{Argv: []string{"/bin/sh", "-c", "echo ready; sleep 30"}, Cwd: dir, Deadline: time.Now().Add(time.Minute)}
	if e := StartExec("j-stop", "local", spec, MinimalExecEnvironment()); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(Root(), "j-stop", "stop"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	r := awaitExec(t, "j-stop")
	if r == nil || !r.Stopped || r.ExitCode != 143 {
		t.Fatal("stop not recorded", r)
	}
}
func TestJobCacheOnlyProviderAttributionAndChildIsolation(t *testing.T) {
	jobHome(t)
	fop := fakeop.Install(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "no-token"))
	t.Setenv("HEV_SECRETS_CACHE", "")
	socketDir := t.TempDir()
	if err := os.Chmod(socketDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_SECRETS_ENDPOINT", filepath.Join(socketDir, "agent.sock"))
	t.Setenv("HEV_SECRETS_MANAGER", "1password")
	t.Setenv("HEV_SECRETS_VAULT", "fixture")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(t.TempDir(), "reads.log"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	close, e := secrets.Serve(ctx, secrets.New())
	if e != nil {
		t.Fatal(e)
	}
	defer close()
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	// The child consumes secrets but writes only a nonsecret verdict.
	s.Exec.Argv = []string{"/bin/sh", "-c", `test -n "$POST_TOKEN" && test "$SOURCE_TOKEN" = "$POST_TOKEN" && test "$MODEL_TOKEN" = "$POST_TOKEN" && test -z "$OP_SERVICE_ACCOUNT_TOKEN" && test -z "$HEV_SECRETS_MANAGER" && test -n "$HEV_JOB_NAME" && test -n "$HEV_JOB_OCCURRENCE" && echo child-environment-ok; printf '%s\n' "$POST_TOKEN"`}
	if _, e = ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	f.now = f.now.Add(time.Minute)
	d := JobDriver{Now: func() time.Time { return f.now }, Notify: func(context.Context, OrchestratorNotice) error { return nil }}
	if e = TickJobs(ctx, d, false); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	run := state.History[0].Run
	r := awaitExec(t, run)
	if r == nil || r.ExitCode != 0 {
		t.Fatal("child env fixture failed", r)
	}
	if e = TickJobs(ctx, d, true); e != nil {
		t.Fatal(e)
	}
	log, e := os.ReadFile(filepath.Join(Root(), run, "log"))
	if e != nil || strings.TrimSpace(string(log)) != "child-environment-ok\n[REDACTED]" {
		t.Fatal("missing child verdict", e)
	}
	readLog, e := os.ReadFile(secrets.ReadLogPath())
	if e != nil || !strings.Contains(string(readLog), "job:"+s.Name) {
		t.Fatal("provider read not attributed", e)
	}
	if fop.Count(t) != 1 {
		t.Fatal("expected one batched provider operation", fop.Count(t))
	}
	// Provider values must never appear in job or dispatch records and logs.
	for _, p := range []string{jobPath(s.Name), filepath.Join(Root(), run, "meta.json"), filepath.Join(Root(), run, "result.json"), filepath.Join(Root(), run, "log"), secrets.ReadLogPath()} {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(b), fakeop.Value) {
			t.Fatalf("secret leaked to %s", p)
		}
	}
}
func TestJobsCacheOutageNeverReadsDirect(t *testing.T) {
	jobHome(t)
	fop := fakeop.Install(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "no-token"))
	t.Setenv("HEV_SECRETS_MANAGER", "1password")
	t.Setenv("HEV_SECRETS_VAULT", "fixture")
	t.Setenv("HEV_SECRETS_CACHE", "off")
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	s.Timeout = "1s"
	if _, e := ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	f.now = f.now.Add(time.Minute)
	d := JobDriver{Now: func() time.Time { return f.now }, Notify: func(context.Context, OrchestratorNotice) error { return nil }}
	if e := TickJobs(context.Background(), d, false); e != nil {
		t.Fatal(e)
	}
	f.now = f.now.Add(2 * time.Second)
	if e := TickJobs(context.Background(), d, true); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	if state.History[0].State != "failed" {
		t.Fatal("cache outage did not reach deadline")
	}
	if fop.Count(t) != 0 {
		t.Fatal("direct provider fallback occurred")
	}
}
func TestJobExecSharesHostCapacity(t *testing.T) {
	jobHome(t)
	config := "cap = 1\n"
	if e := os.WriteFile(os.Getenv("HEV_DISPATCH_CONFIG"), []byte(config), 0600); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	spec := ExecSpec{Argv: []string{"/bin/sh", "-c", "sleep 30"}, Cwd: dir, Deadline: time.Now().Add(time.Minute)}
	if e := StartExec("j-cap-one", "local", spec, MinimalExecEnvironment()); e != nil {
		t.Fatal(e)
	}
	defer func() {
		_ = os.WriteFile(filepath.Join(Root(), "j-cap-one", "stop"), nil, 0600)
		awaitExec(t, "j-cap-one")
	}()
	if e := StartExec("j-cap-two", "local", spec, MinimalExecEnvironment()); e == nil {
		t.Fatal("ignored host capacity")
	}
	if _, e := os.Stat(filepath.Join(Root(), "j-cap-two")); !os.IsNotExist(e) {
		t.Fatal("capacity refusal reserved execution")
	}
}

func TestJobsRealHostBudgetDefersWithoutDirectRead(t *testing.T) {
	jobHome(t)
	fop := fakeop.Install(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "no-token"))
	t.Setenv("HEV_SECRETS_MANAGER", "1password")
	t.Setenv("HEV_SECRETS_VAULT", "fixture")
	t.Setenv("HEV_SECRETS_CACHE", "")
	socketDir, e := os.MkdirTemp("", "job-cache-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(socketDir)
	t.Setenv(secrets.EndpointEnv, filepath.Join(socketDir, "agent.sock"))
	policy := filepath.Join(socketDir, "secrets-policy.json")
	t.Setenv("HEV_SECRETS_POLICY", policy)
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(socketDir, "reads.log"))
	if e = os.WriteFile(policy, []byte(`{"daily":1,"hourly":1,"caller_minute":1}`), 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	close, e := secrets.Serve(ctx, secrets.New())
	if e != nil {
		t.Fatal(e)
	}
	defer close()
	f := newFakeJobs()
	s := applyFakeJob(t, f, "none")
	s.Timeout = "10s"
	s.Exec.Argv = []string{"/bin/sh", "-c", "echo stub"}
	if _, e = ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	d := JobDriver{Now: func() time.Time { return f.now }, Notify: func(context.Context, OrchestratorNotice) error { return nil }}
	f.now = f.now.Add(time.Minute)
	if e = TickJobs(ctx, d, false); e != nil {
		t.Fatal(e)
	}
	first := mustJob(t, s.Name).History[0].Run
	awaitExec(t, first)
	f.now = f.now.Add(time.Second)
	if e = TickJobs(ctx, d, false); e != nil {
		t.Fatal(e)
	}
	// All first references were one provider operation. A fresh reference map
	// must now be denied by the real host ledger before calling the provider.
	s.Secrets = map[string]string{"NEW_TOKEN": "secret://new-token"}
	if _, e = ApplyJob(s, f.now, false, false); e != nil {
		t.Fatal(e)
	}
	f.now = f.now.Add(59 * time.Second)
	if e = TickJobs(ctx, d, false); e != nil {
		t.Fatal(e)
	}
	state := mustJob(t, s.Name)
	if state.History[1].State != "queued" {
		t.Fatal("budget refusal did not defer")
	}
	if fop.Count(t) != 1 {
		t.Fatal("budget refusal called provider", fop.Count(t))
	}
	f.now = f.now.Add(11 * time.Second)
	if e = TickJobs(ctx, d, false); e != nil {
		t.Fatal(e)
	}
	state = mustJob(t, s.Name)
	if state.History[1].State != "failed" {
		t.Fatal("budget deadline did not fail")
	}
	if fop.Count(t) != 1 {
		t.Fatal("direct fallback after budget refusal")
	}
}
