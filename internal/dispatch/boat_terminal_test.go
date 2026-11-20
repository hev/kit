package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoatInitialResultIsNotTerminal(t *testing.T) {
	for _, action := range []string{"watch", "stop", "show", "list"} {
		t.Run(action, func(t *testing.T) {
			files := map[string]string{"meta.json": `{"name":"one","host":"cloud"}`, "result.json": `{"exit_code":0}`}
			f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: files}
			withFakeBoat(t, f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads := 0
			p := &reworkProvider{fakeEphemeral: f}
			p.exec = func(ctx context.Context, cmd string) (string, error) {
				if strings.Contains(cmd, "hev dispatch rpc") {
					reads++
					if f.stopped != 0 {
						t.Error("initial result stopped VM before cancellation")
					}
					if reads == 3 {
						cancel()
					}
					data, _ := json.Marshal(Response{Files: files})
					return string(data), nil
				}
				return f.Exec(ctx, "vm", cmd)
			}
			installReworkProvider(t, p)
			old := boatWatchPoll
			boatWatchPoll = time.Millisecond
			defer func() { boatWatchPoll = old }()
			if action == "watch" {
				_ = WatchBoat(ctx, Host{Name: "cloud"}, "one")
				if reads < 3 {
					t.Fatal("watcher accepted initial result")
				}
			} else if action == "stop" {
				_, _ = boatRPC(ctx, Host{Name: "cloud"}, Request{Action: action, Name: "one"})
				if reads < 3 {
					t.Fatal("stop accepted initial result as terminal")
				}
			} else {
				_, err := boatRPC(ctx, Host{Name: "cloud"}, Request{Action: action, Name: "one"})
				if err != nil || f.stopped != 0 {
					t.Fatal(err, f.stopped)
				}
			}
		})
	}
}

func TestBoatWatcherWaitsForAttemptLock(t *testing.T) {
	files := map[string]string{"meta.json": `{"name":"one","host":"cloud"}`, "result.json": `{"exit_code":0}`}
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: files}
	withFakeBoat(t, f)
	guard, err := lockBoat("cloud", "one", "attempt")
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			guard()
		}
	}()
	p := &reworkProvider{fakeEphemeral: f}
	reads := 0
	p.exec = func(ctx context.Context, cmd string) (string, error) {
		if strings.Contains(cmd, "hev dispatch rpc") {
			reads++
			if reads == 2 {
				files["result.json"] = `{"end":"2026-01-01T00:00:00Z"}`
				time.AfterFunc(30*time.Millisecond, guard)
				released = true
			}
			data, _ := json.Marshal(Response{Files: files})
			return string(data), nil
		}
		return "", fmt.Errorf("fetch failed")
	}
	installReworkProvider(t, p)
	oldPoll, oldDrain := boatWatchPoll, boatWatchDrain
	boatWatchPoll = time.Millisecond
	boatWatchDrain = 0
	defer func() { boatWatchPoll, boatWatchDrain = oldPoll, oldDrain }()
	if err := WatchBoat(context.Background(), Host{Name: "cloud"}, "one"); err == nil || reads < 2 || f.stopped != 1 {
		t.Fatal(err, reads, f.stopped)
	}
}

func TestBoatDefiniteCreateFailureFreesName(t *testing.T) {
	for _, status := range []int{400, 402, 403, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := &fakeEphemeral{}
			withFakeBoat(t, f)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					w.WriteHeader(status)
					return
				}
				fmt.Fprint(w, `{"sandboxes":[]}`)
			}))
			defer srv.Close()
			old := ephemeralProvider
			ephemeralProvider = func(Host) (EphemeralProvider, error) { return &Boat{endpoint: srv.URL, client: srv.Client()}, nil }
			defer func() { ephemeralProvider = old }()
			oldTimeout := boatReconcileTimeout
			boatReconcileTimeout = time.Millisecond
			defer func() { boatReconcileTimeout = oldTimeout }()
			_, err := boatRPC(context.Background(), Host{Name: "cloud", Cap: 2}, Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Name: "one", Repo: "example/repo", Validation: "none", Timeout: 1}}})
			if err == nil {
				t.Fatal("accepted")
			}
			_, err = os.Stat(filepath.Join(boatRecordDir("cloud", "one"), "create-unknown"))
			if (err == nil) != (status >= 500) {
				t.Fatal("unknown marker", err)
			}
		})
	}
}

func TestBoatUnknownLookupHonorsCancellation(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &reworkProvider{fakeEphemeral: f, list: func(ctx context.Context) ([]Machine, error) { return nil, ctx.Err() }}
	start := time.Now()
	if err := reconcileBoatCreateContext(ctx, p, Host{Name: "cloud"}, "one"); err == nil || time.Since(start) > time.Second {
		t.Fatal(err)
	}
}

func TestBoatUnknownListDoesNotWaitForPropagation(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	installReworkProvider(t, f)
	dir := boatRecordDir("cloud", "one")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "create-unknown"), nil, 0600)
	old := boatReconcileLauncher
	defer func() { boatReconcileLauncher = old }()
	launched := false
	boatReconcileLauncher = func(h Host, name string) error { launched = true; return nil }
	start := time.Now()
	_, err := boatRPC(context.Background(), Host{Name: "cloud"}, Request{Action: "list"})
	if err != nil || !launched || time.Since(start) > time.Second {
		t.Fatal(err, launched)
	}
}

func TestBoatResumeRefusesCapBeforeMutation(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "other", State: "ready"}}}
	withFakeBoat(t, f)
	p := &reworkProvider{fakeEphemeral: f, resume: func(context.Context) error { t.Fatal("resumed over cap"); return nil }}
	if err := resumeBoat(context.Background(), p, Host{Name: "cloud", Cap: 1}, "one", "vm", time.Minute); err == nil {
		t.Fatal("cap ignored")
	}
}
