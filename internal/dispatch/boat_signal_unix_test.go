//go:build !windows

package dispatch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestBoatSignalChild(t *testing.T) {
	endpoint := os.Getenv("HEV_TEST_BOAT_ENDPOINT")
	if endpoint == "" {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ephemeralProvider = func(Host) (EphemeralProvider, error) {
		return &Boat{endpoint: endpoint, client: http.DefaultClient}, nil
	}
	_, err := boatRPC(ctx, Host{Name: "cloud", Cap: 2}, Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Name: "one", Repo: "example/repo", Validation: "none", Timeout: 1}}})
	if err == nil || ctx.Err() == nil {
		t.Fatal("expected signal cancellation", err)
	}
}
func TestBoatSignalDuringStartDeletesVM(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ready, deleted := make(chan struct{}, 1), make(chan struct{}, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "DELETE":
					deleted <- struct{}{}
					fmt.Fprint(w, `{}`)
				case r.Method == "POST":
					fmt.Fprint(w, `{"sandbox":{"id":"vm"}}`)
				case r.URL.Path == "/sandboxes":
					fmt.Fprint(w, `{"sandboxes":[]}`)
				default:
					ready <- struct{}{}
					<-r.Context().Done()
				}
			}))
			defer srv.Close()
			home := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestBoatSignalChild$")
			cmd.Env = append(os.Environ(), "HOME="+home, "HEV_TEST_BOAT_ENDPOINT="+srv.URL)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() { _ = cmd.Process.Kill() }()
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("child never reached ready")
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("child did not unwind")
			}
			select {
			case <-deleted:
			default:
				t.Fatal("signal bypassed fresh-context delete")
			}
			var result Result
			if err := readJSON(filepath.Join(home, "dispatch", ".remote", "cloud", "one", "result.json"), &result); err != nil || !result.AdmissionFailed {
				t.Fatal(result, err)
			}
		})
	}
}
func TestBoatDeadLockChild(t *testing.T) {
	if os.Getenv("HEV_TEST_BOAT_LOCK") == "" {
		return
	}
	if _, err := lockBoat("cloud", "one", "watch"); err != nil {
		t.Fatal(err)
	}
	// Exit without unlocking, as with a crash.
	os.Exit(0)
}
func TestBoatDeadOwnerLockReclaimed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoatDeadLockChild$")
	cmd.Env = append(os.Environ(), "HEV_TEST_BOAT_LOCK=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	unlock, err := lockBoat("cloud", "one", "watch")
	if err != nil {
		t.Fatal("dead owner lock not reclaimed", err)
	}
	defer unlock()
	var owner pidRecord
	if err := readJSON(boatRecordDir("cloud", "one")+".watch-lock.owner", &owner); err != nil || owner.PID != os.Getpid() || owner.Started == "" {
		t.Fatal(owner, err)
	}
}
