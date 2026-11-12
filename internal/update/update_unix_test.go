//go:build !windows

package update

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A run started before an update keeps the binary it was started from to
// the end; the next run starts on the new build.
func TestRunInProgressFinishesOnOldBinary(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	gate := filepath.Join(t.TempDir(), "go")
	var out bytes.Buffer
	run := exec.Command(h.link)
	run.Env = append(os.Environ(), "HEV_FAKE_WAIT="+gate)
	run.Stdout = &out
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { run.Process.Kill() })
	time.Sleep(100 * time.Millisecond)

	// The first update replaces a plain file; the second a link to a
	// versioned build. Both leave the running process's file alone.
	if _, err := h.updater(edgeA).Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	if h.running(t) != edgeB {
		t.Fatal("a new run does not start on the new build")
	}
	os.WriteFile(gate, nil, 0o600)
	if err := run.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != edgeA {
		t.Fatalf("the run in progress finished as %q, want %s", got, edgeA)
	}

	// A run started from the versioned build survives a rollback too.
	gate2 := filepath.Join(t.TempDir(), "go")
	out.Reset()
	run2 := exec.Command(h.link)
	run2.Env = append(os.Environ(), "HEV_FAKE_WAIT="+gate2)
	run2.Stdout = &out
	if err := run2.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { run2.Process.Kill() })
	time.Sleep(100 * time.Millisecond)
	if _, err := h.updater(edgeB).Run(context.Background(), Request{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(gate2, nil, 0o600)
	run2.Wait()
	if got := strings.TrimSpace(out.String()); got != edgeB {
		t.Fatalf("run finished as %q, want %s", got, edgeB)
	}
}

// The service path `hev up` installs (~/.local/bin/hev) moves with the
// binary hev update was run as, so the daemon restarts onto the new build.
func TestServiceCopyMovesToo(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeB, publishOpt{})
	svcBin := filepath.Join(h.home, ".local", "bin", "hev")
	os.MkdirAll(filepath.Dir(svcBin), 0o755)
	os.WriteFile(svcBin, fakeBinary(edgeA), 0o755)
	u := h.updater(edgeA)
	u.Extra = []string{svcBin, filepath.Join(h.home, "missing", "hev")}
	if _, err := u.Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(svcBin).Output()
	if err != nil || strings.TrimSpace(string(out)) != edgeB {
		t.Fatalf("service copy runs %q, %v", out, err)
	}
	st, _ := u.LoadState()
	if len(st.Links) != 2 {
		t.Fatalf("links = %v", st.Links)
	}
}

func TestDaemonAutoUpdate(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	u := h.updater(edgeA)
	u.Now = func() time.Time { return now }

	// Off by default: no check, not even a request.
	if r := u.Auto(context.Background()); r.Ran || h.repo.Requests() != 0 {
		t.Fatalf("auto ran with the default config: %+v, %d requests", r, h.repo.Requests())
	}
	// Auto on but no channel chosen is still off.
	u.Config = Config{Auto: true}
	if r := u.Auto(context.Background()); r.Ran {
		t.Fatal("auto ran without a channel")
	}

	u.Config = Config{Auto: true, Channel: ChannelEdge, Interval: "2h"}
	r := u.Auto(context.Background())
	if !r.Ran || !r.Restart || r.Err != nil || r.Report.After != edgeB {
		t.Fatalf("auto = %+v", r)
	}
	if h.running(t) != edgeB {
		t.Fatal("not switched")
	}
	if h.svc.calls != 0 {
		t.Fatal("the daemon restarted its own service instead of re-executing")
	}
	st, _ := u.LoadState()
	if st.Pending == nil || st.Pending.To != edgeB || !st.NextCheck.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("state = %+v", st)
	}

	// Before the next check is due nothing happens.
	u.Current = edgeB
	n := h.repo.Requests()
	if r := u.Auto(context.Background()); r.Ran || h.repo.Requests() != n {
		t.Fatal("checked before the schedule")
	}

	// The new daemon confirms after a clean cycle.
	if err := u.Confirm(); err != nil {
		t.Fatal(err)
	}
	if st, _ := u.LoadState(); st.Pending != nil {
		t.Fatal("pending switch not confirmed")
	}

	// A failing source backs off.
	now = now.Add(3 * time.Hour)
	h.repo.srv.Close()
	r = u.Auto(context.Background())
	if !r.Ran || r.Err == nil {
		t.Fatalf("auto against a dead source = %+v", r)
	}
	st, _ = u.LoadState()
	if st.Failures != 1 || !st.NextCheck.Equal(now.Add(4*time.Hour)) {
		t.Fatalf("backoff state = %+v", st)
	}
}

// A self-updated daemon that never completes a cycle rolls itself back.
func TestDaemonStartCheckRollsBackAfterRepeatedFailures(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	u.Config = Config{Auto: true, Channel: ChannelEdge}
	if r := u.Auto(context.Background()); !r.Restart {
		t.Fatalf("auto = %+v", r)
	}
	u.Current = edgeB
	for i := 1; i <= maxStartAttempts; i++ {
		if rb, err := u.StartCheck(context.Background()); rb || err != nil {
			t.Fatalf("start %d: rollback=%v err=%v", i, rb, err)
		}
	}
	rb, err := u.StartCheck(context.Background())
	if err != nil || !rb {
		t.Fatalf("start after %d attempts: rollback=%v err=%v", maxStartAttempts, rb, err)
	}
	if h.running(t) != edgeA {
		t.Fatal("not rolled back")
	}
	st, _ := u.LoadState()
	if st.Pending != nil || st.Current != edgeA {
		t.Fatalf("state = %+v", st)
	}
}
