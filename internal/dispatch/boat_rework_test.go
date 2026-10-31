package dispatch

import (
	"context"
	"encoding/base64"
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

type reworkProvider struct {
	*fakeEphemeral
	resume func(context.Context) error
	create func(context.Context) (Machine, error)
	list   func(context.Context) ([]Machine, error)
	exec   func(context.Context, string) (string, error)
}

func (p *reworkProvider) Resume(ctx context.Context, _ string, _ time.Duration) error {
	if p.resume != nil {
		return p.resume(ctx)
	}
	return nil
}
func (p *reworkProvider) Create(ctx context.Context, _ MachineSpec) (Machine, error) {
	return p.create(ctx)
}
func (p *reworkProvider) List(ctx context.Context, _ string) ([]Machine, error) {
	if p.list != nil {
		return p.list(ctx)
	}
	return p.machines, nil
}
func (p *reworkProvider) Exec(ctx context.Context, id, cmd string) (string, error) {
	if p.exec != nil {
		return p.exec(ctx, cmd)
	}
	return p.fakeEphemeral.Exec(ctx, id, cmd)
}

// Exercise the real HTTP provider, using the fake only as the server's state.
func installReworkProvider(t *testing.T, p EphemeralProvider) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var out any = map[string]any{}
		var err error
		switch {
		case r.URL.Path == "/sandboxes" && r.Method == "GET":
			var machines []Machine
			machines, err = p.List(ctx, "cloud")
			out = map[string]any{"sandboxes": machines}
		case r.URL.Path == "/sandboxes" && r.Method == "POST":
			var machine Machine
			machine, err = p.Create(ctx, MachineSpec{})
			out = map[string]any{"sandbox": machine}
		case strings.HasSuffix(r.URL.Path, "/commands"):
			var q struct {
				Command string `json:"command"`
			}
			_ = json.NewDecoder(r.Body).Decode(&q)
			var stdout string
			stdout, err = p.Exec(ctx, "vm", q.Command)
			out = map[string]any{"stdout": stdout, "exitCode": 0}
		case strings.HasSuffix(r.URL.Path, "/resume"):
			err = p.Resume(ctx, "vm", time.Minute)
		case strings.HasSuffix(r.URL.Path, "/stop"):
			err = p.Stop(ctx, "vm")
		case r.Method == "DELETE":
			err = p.Delete(ctx, "vm")
		case strings.HasSuffix(r.URL.Path, "/files"):
			var data []byte
			data, err = p.Copy(ctx, "vm", r.URL.Query().Get("path"))
			out = map[string]any{"content": base64.StdEncoding.EncodeToString(data)}
		default:
			out = map[string]any{"sandbox": Machine{ID: "vm", State: "ready"}}
		}
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	old := ephemeralProvider
	ephemeralProvider = func(Host) (EphemeralProvider, error) { return &Boat{endpoint: srv.URL, client: srv.Client()}, nil }
	t.Cleanup(func() { ephemeralProvider = old; srv.Close() })
}

func TestBoatUnknownCreateLagAndReservation(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	oldTimeout, oldPoll := boatReconcileTimeout, boatReconcilePoll
	boatReconcileTimeout, boatReconcilePoll = 20*time.Millisecond, time.Millisecond
	defer func() { boatReconcileTimeout, boatReconcilePoll = oldTimeout, oldPoll }()
	created, visible := false, false
	p := &reworkProvider{fakeEphemeral: f}
	p.create = func(context.Context) (Machine, error) {
		created = true
		return Machine{}, fmt.Errorf("unknown transport outcome")
	}
	p.list = func(context.Context) ([]Machine, error) {
		if created && visible {
			return []Machine{{ID: "late", Labels: map[string]string{"hev.order": "one"}}}, nil
		}
		return nil, nil
	}
	installReworkProvider(t, p)
	q := Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Name: "one", Repo: "example/repo", Validation: "none", Timeout: 1}}}
	h := Host{Name: "cloud", Cap: 2}
	if _, err := boatRPC(context.Background(), h, q); err == nil {
		t.Fatal("expected unknown outcome")
	}
	rows := boatCachedRows("cloud", "one").Rows
	if len(rows) != 1 || rows[0].State != "create-unknown" {
		t.Fatal(rows)
	}
	p.create = func(context.Context) (Machine, error) { t.Fatal("must reserve unknown name"); return Machine{}, nil }
	if _, err := boatRPC(context.Background(), h, q); err == nil {
		t.Fatal("reserved name admitted")
	}
	visible = true
	if err := reconcileBoatCreateForTest(t, h, "one"); err != nil || f.deleted != 1 {
		t.Fatal(err, f.deleted)
	}
	if _, err := os.Stat(filepath.Join(boatRecordDir("cloud", "one"), "create-unknown")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestBoatUnknownCreateRetriesLag(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	old := boatReconcilePoll
	boatReconcilePoll = time.Millisecond
	defer func() { boatReconcilePoll = old }()
	n := 0
	p := &reworkProvider{fakeEphemeral: f}
	p.list = func(context.Context) ([]Machine, error) {
		n++
		if n < 3 {
			return nil, nil
		}
		return []Machine{{ID: "late", Labels: map[string]string{"hev.order": "one"}}}, nil
	}
	dir := boatRecordDir("cloud", "one")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "create-unknown"), nil, 0600)
	if err := reconcileBoatCreate(p, Host{Name: "cloud"}, "one"); err != nil || n != 3 || f.deleted != 1 {
		t.Fatal(err, n, f.deleted)
	}
}
func TestBoatLiveReworkDoesNotRestartCapture(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}}
			withFakeBoat(t, f)
			p := &reworkProvider{fakeEphemeral: f}
			if reject {
				p.exec = func(context.Context, string) (string, error) { return "", fmt.Errorf("rework rejected") }
			}
			installReworkProvider(t, p)
			old := boatWatchLauncher
			boatWatchLauncher = func(Host, string) error { return nil }
			defer func() { boatWatchLauncher = old }()
			_, err := boatRPC(context.Background(), Host{Name: "cloud"}, Request{Action: "rework", Name: "one"})
			if (err != nil) != reject {
				t.Fatal(err)
			}
			for _, cmd := range f.commands {
				if strings.Contains(cmd, "python3") {
					t.Fatal("restarted capture on live VM")
				}
			}
		})
	}
}
func TestBoatResumeDoesNotHoldGlobalLock(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	p := &reworkProvider{fakeEphemeral: f}
	p.resume = func(context.Context) error {
		done := make(chan struct{})
		go func() {
			unlock, err := lockRoot()
			if err == nil {
				unlock()
			}
			close(done)
		}()
		select {
		case <-done:
			return nil
		case <-time.After(time.Second):
			return fmt.Errorf("global lock held through resume")
		}
	}
	if err := resumeBoat(context.Background(), p, Host{Name: "cloud", Cap: 2}, "one", "vm", time.Minute); err != nil {
		t.Fatal(err)
	}
}
func TestBoatListDoesNotStopLiveWatcher(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}}
	withFakeBoat(t, f)
	installReworkProvider(t, f)
	dir := boatRecordDir("cloud", "one")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "one"}})
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now().Add(-time.Hour)})
	unlock, err := lockBoat("cloud", "one", "watch-")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err = boatRPC(context.Background(), Host{Name: "cloud"}, Request{Action: "list"}); err != nil {
		t.Fatal(err)
	}
	if f.stopped != 0 {
		t.Fatal("list stopped watcher VM")
	}
}
func TestBoatOldWatcherDoesNotStopNewAttempt(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: map[string]string{"result.json": `{"end":"2026-01-01T00:00:00Z"}`}}
	withFakeBoat(t, f)
	oldDrain := boatWatchDrain
	boatWatchDrain = 30 * time.Millisecond
	defer func() { boatWatchDrain = oldDrain }()
	p := &reworkProvider{fakeEphemeral: f}
	p.exec = func(ctx context.Context, cmd string) (string, error) {
		out, err := f.Exec(ctx, "vm", cmd)
		if strings.Contains(cmd, "hev dispatch rpc") {
			if e := newBoatAttempt("cloud", "one"); e != nil {
				return "", e
			}
		}
		return out, err
	}
	installReworkProvider(t, p)
	if err := WatchBoat(context.Background(), Host{Name: "cloud"}, "one"); err != nil {
		t.Fatal(err)
	}
	if f.stopped != 0 {
		t.Fatal("old watcher stopped new attempt")
	}
}
func TestBoatRetryRemovesFailedResult(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	q := Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Name: "one", Repo: "example/repo", Validation: "none", Timeout: 1}}}
	h := Host{Name: "cloud", Cap: 2}
	recordBoatAdmission(h, q, fmt.Errorf("previous failure"))
	old := boatWatchLauncher
	boatWatchLauncher = func(Host, string) error { return nil }
	defer func() { boatWatchLauncher = old }()
	if _, err := boatRPC(context.Background(), h, q); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(boatRecordDir("cloud", "one"), "result.json")); !os.IsNotExist(err) {
		t.Fatal("stale failed result", err)
	}
}

func TestBoatNotifyDoesNotHoldGlobalLock(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	unlock, err := lockRoot()
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()
	done := make(chan error, 1)
	go func() {
		done <- notifyBoatRecord(context.Background(), Host{Name: "cloud"}, "one", map[string]string{"meta.json": `{"name":"one"}`, "result.json": `{"end":"2026-01-01T00:00:00Z"}`})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		unlock()
		released = true
		<-done
		t.Fatal("notify waited for global lock")
	}
}

func TestBoatListRecoversOrphanOnlyAfterDrain(t *testing.T) {
	for _, recent := range []bool{false, true} {
		t.Run(fmt.Sprint(recent), func(t *testing.T) {
			end := time.Now().Add(-time.Hour)
			if recent {
				end = time.Now().Add(time.Hour)
			}
			files := map[string]string{"meta.json": `{"name":"one","host":"cloud"}`, "result.json": fmt.Sprintf(`{"end":%q}`, end.Format(time.RFC3339Nano))}
			f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: files, archive: boatArchive(t, files)}
			withFakeBoat(t, f)
			if _, err := boatRPC(context.Background(), Host{Name: "cloud"}, Request{Action: "list"}); err != nil {
				t.Fatal(err)
			}
			if recent && f.stopped != 0 || !recent && f.stopped != 1 {
				t.Fatal("drain stop", f.stopped)
			}
		})
	}
}

func TestBoatResumedReworkCaptureOnlyAfterAcceptance(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "archived", Labels: map[string]string{"hev.order": "one"}}}}
			withFakeBoat(t, f)
			dir := boatRecordDir("cloud", "one")
			_ = os.MkdirAll(dir, 0700)
			_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "one", Timeout: 20}})
			accepted, capture := false, false
			p := &reworkProvider{fakeEphemeral: f}
			p.exec = func(ctx context.Context, cmd string) (string, error) {
				if strings.Contains(cmd, "hev dispatch rpc") {
					if reject {
						return "", fmt.Errorf("rejected")
					}
					accepted = true
					return `{}`, nil
				}
				if strings.Contains(cmd, "python3") {
					if !accepted {
						t.Error("capture before acceptance")
					}
					capture = true
				}
				return "", nil
			}
			installReworkProvider(t, p)
			old := boatWatchLauncher
			boatWatchLauncher = func(Host, string) error { return nil }
			defer func() { boatWatchLauncher = old }()
			_, err := boatRPC(context.Background(), Host{Name: "cloud", Cap: 2}, Request{Action: "rework", Name: "one"})
			if (err != nil) != reject || capture == reject {
				t.Fatal(err, capture)
			}
			if reject && f.stopped == 0 {
				t.Fatal("rejected resumed rework leaked VM")
			}
		})
	}
}

func reconcileBoatCreateForTest(t *testing.T, h Host, name string) error {
	t.Helper()
	p, err := ephemeralProvider(h)
	if err != nil {
		return err
	}
	return reconcileBoatCreate(p, h, name)
}

func TestBoatLiveActionCancellationStopsWithFreshContext(t *testing.T) {
	for _, action := range []string{"clean", "rework", "stop"} {
		t.Run(action, func(t *testing.T) {
			f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}}
			withFakeBoat(t, f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &reworkProvider{fakeEphemeral: f}
			p.exec = func(context.Context, string) (string, error) { cancel(); return "", fmt.Errorf("interrupted") }
			installReworkProvider(t, p)
			if _, err := boatRPC(ctx, Host{Name: "cloud"}, Request{Action: action, Name: "one"}); err == nil {
				t.Fatal("cancellation ignored")
			}
			if f.stopped == 0 {
				t.Fatal("live action failed to stop after cancellation")
			}
		})
	}
}
