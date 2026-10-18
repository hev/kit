//go:build linux || darwin

package nativegate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

func TestRetainedReceiverChild(t *testing.T) {
	release := os.Getenv("RETAINED_RECEIVER_RELEASE")
	if release == "" {
		return
	}
	f, err := os.OpenFile("effect", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = f.WriteString("once\n")
	_ = f.Close()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if _, err = os.Stat(release); err == nil {
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(3)
}

type retainedFixture struct {
	cfg               ReceiverConfig
	config            string
	call              ReceiverCall
	store             *n.Store
	provenance        Provenance
	release, launches string
}

func retainedSetup(t *testing.T) retainedFixture {
	t.Helper()
	root := t.TempDir()
	journal := filepath.Join(root, "journal")
	enrollment := n.Enrollment{HostID: "host", ExecutorID: "executor"}
	store, err := n.Open(journal, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	c, p, _ := sample()
	r := c.Request
	r.Directory = root
	r.Argv = []string{os.Args[0], "-test.run=^TestRetainedReceiverChild$"}
	release := filepath.Join(root, "release")
	r.Environment = []string{"RETAINED_RECEIVER_RELEASE=" + release}
	r.PayloadDigest = n.RequestDigest(r)
	if err = store.AdvanceFence(p.OriginalFence); err != nil {
		t.Fatal(err)
	}
	j, err := store.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "retained")
	if err = os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	launches := filepath.Join(root, "launches")
	launcher := filepath.Join(root, "launcher")
	script := "#!/bin/sh\nprintf 'once\\n' >> '" + launches + "'\nprintf 'accepted\\n'\n"
	if err = os.WriteFile(launcher, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	supervisor, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ld, err := fileDigest(launcher)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := fileDigest(supervisor)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ReceiverConfig{Version: ReceiverVersion, JournalRoot: journal, Enrollment: enrollment, TimeoutSeconds: 5, Retained: &RetainedConfig{Version: SupervisorVersion, Launcher: launcher, LauncherDigest: ld, Supervisor: supervisor, SupervisorDigest: sd, StateRoot: state, WorkloadTimeoutSeconds: 10}}
	config := filepath.Join(root, "receiver.json")
	raw, _ := json.Marshal(cfg)
	if err = os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	claim := dbClaim()
	binding := a.NativeBinding{OrderUID: r.Identity.OrderUID, Generation: 3, ScopeDigest: r.Identity.ScopeDigest, JobID: j.ID, RequestDigest: r.PayloadDigest}
	call := ReceiverCall{Version: ReceiverVersion, Operation: "start", JobID: j.ID, Claim: &claim, Binding: &binding}
	return retainedFixture{cfg, config, call, store, p, release, launches}
}
func (f retainedFixture) run(ctx context.Context, cfg ReceiverConfig, c ReceiverCall, observe func(n.StartedObservation)) (Inspection, error) {
	facade := fakeDBFacade(func(ctx context.Context, claim a.DBClaim, b a.NativeBinding, k a.Check, launch a.DBLaunch) (a.Started, error) {
		return launch(a.Receipt{Version: n.StartVersion, JobID: k.JobID, PayloadDigest: k.PayloadDigest, Authority: k.Authority, DecisionID: claim.ActionID, AuthorityRevision: "db-claim;maintenance"}, func() error { return ctx.Err() })
	})
	if _, err := startObserved(ctx, f.store, facade, *c.Claim, *c.Binding, f.provenance, time.Duration(cfg.Retained.WorkloadTimeoutSeconds)*time.Second, observe); err != nil {
		return Inspection{}, err
	}
	return Inspect(ctx, f.store, c.JobID)
}
func waitPath(t *testing.T, path string) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("missing owned fixture path", path)
}
func TestRetainedReadinessCompletionAndDuplicateDoNotRelaunch(t *testing.T) {
	f := retainedSetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	type result struct {
		i   Inspection
		err error
	}
	started := make(chan result, 1)
	go func() { i, err := RunReceiverFromConfig(ctx, f.config, f.cfg, f.call); started <- result{i, err} }()
	path := descriptorPath(f.cfg, f.call.JobID)
	waitPath(t, f.launches)
	finished := make(chan result, 1)
	go func() { i, err := supervise(ctx, f.config, f.cfg, path, f.run); finished <- result{i, err} }()
	var first result
	select {
	case first = <-started:
	case <-ctx.Done():
		t.Fatal("start did not return while child lived")
	}
	if first.err != nil || first.i.State != "Running" || first.i.Completion != nil {
		t.Fatal(first.i, first.err)
	}
	duplicate, err := RunReceiverFromConfig(ctx, f.config, f.cfg, f.call)
	if err != nil || duplicate.State != "Running" {
		t.Fatal(duplicate, err)
	}
	if err = os.WriteFile(f.release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	done := <-finished
	if done.err != nil || done.i.State != "Completed" || done.i.Completion == nil {
		t.Fatal(done.i, done.err)
	}
	historical, err := RunReceiverFromConfig(ctx, f.config, f.cfg, f.call)
	if err != nil || historical.State != "Completed" || n.Digest(historical.Completion) != n.Digest(done.i.Completion) {
		t.Fatal(historical, err)
	}
	if _, err = supervise(ctx, f.config, f.cfg, path, f.run); !errors.Is(err, ErrRetainedUnknown) {
		t.Fatal("duplicate supervisor admitted", err)
	}
	for _, path := range []string{f.launches, filepath.Join(filepath.Dir(f.release), "effect")} {
		b, err := os.ReadFile(path)
		if err != nil || string(b) != "once\n" {
			t.Fatal("replayed effect", path, string(b), err)
		}
	}
}
func lostRetainedReply(t *testing.T, f retainedFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := RunReceiverFromConfig(ctx, f.config, f.cfg, f.call); done <- err }()
	waitPath(t, f.launches)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("missing readiness accepted")
	}
}
func TestRetainedLostLaunchAndPartialDescriptorAreInspectOnly(t *testing.T) {
	f := retainedSetup(t)
	lostRetainedReply(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, err := RunReceiverFromConfig(ctx, f.config, f.cfg, f.call)
	cancel()
	if err == nil {
		t.Fatal("missing readiness accepted")
	}
	b, err := os.ReadFile(f.launches)
	if err != nil || string(b) != "once\n" {
		t.Fatal("launcher replayed", string(b), err)
	}
	path := descriptorPath(f.cfg, f.call.JobID)
	if err = os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = RunReceiverFromConfig(context.Background(), f.config, f.cfg, f.call); err == nil {
		t.Fatal("partial descriptor accepted")
	}
	j, err := f.store.Query(f.call.JobID)
	if err != nil || j.State != "Pending" {
		t.Fatal(j, err)
	}
}

func TestRetainedNotificationFailureStillCollectsOriginalCompletion(t *testing.T) {
	f := retainedSetup(t)
	lostRetainedReply(t, f)
	path := descriptorPath(f.cfg, f.call.JobID)
	if err := os.Mkdir(filepath.Join(filepath.Dir(path), "ready.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := supervise(context.Background(), f.config, f.cfg, path, f.run)
	if err != nil || got.State != "Completed" || got.Completion == nil {
		t.Fatal(got, err)
	}
	events, err := f.store.Outbox()
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
}
func TestRetainedDescriptorStrictStartPresenceAndPins(t *testing.T) {
	f := retainedSetup(t)
	cfgHash, _ := fileDigest(f.config)
	d := NativeSupervisorDescriptor{Version: SupervisorVersion, ConfigRef: f.config, ConfigDigest: cfgHash, SupervisorRef: f.cfg.Retained.Supervisor, SupervisorDigest: f.cfg.Retained.SupervisorDigest, StartCall: f.call, WorkloadTimeoutSeconds: 10}
	d.DescriptorDigest = DescriptorDigest(d)
	raw, _ := json.Marshal(d)
	for _, bad := range []string{strings.Replace(string(raw), `"start_call":{`, `"start_call":{"identity":null,`, 1), strings.Replace(string(raw), `"start_call":{`, `"start_call":{"job_id":"different",`, 1), strings.Replace(string(raw), `"start_call"`, `"START_CALL"`, 1)} {
		var decoded NativeSupervisorDescriptor
		if json.Unmarshal([]byte(bad), &decoded) == nil {
			t.Fatal("ambiguous descriptor accepted")
		}
	}
	f.cfg.Retained.LauncherDigest = "sha256:" + strings.Repeat("0", 64)
	changed, _ := json.Marshal(f.cfg)
	if err := os.WriteFile(f.config, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunReceiverFromConfig(context.Background(), f.config, f.cfg, f.call); err == nil {
		t.Fatal("launcher pin drift accepted")
	}
	if _, err := os.Stat(f.launches); !os.IsNotExist(err) {
		t.Fatal("untrusted launcher ran", err)
	}
}
