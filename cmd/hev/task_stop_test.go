package main

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

type fakeDaemonControl struct {
	signals []bool
	gone    bool
	closed  bool
}

func (p *fakeDaemonControl) Signal(force bool) error {
	p.signals = append(p.signals, force)
	if force {
		p.gone = true
	}
	return nil
}
func (p *fakeDaemonControl) Exited() (bool, error) { return p.gone, nil }
func (p *fakeDaemonControl) Close()                { p.closed = true }
func TestStopWindowsTaskFailures(t *testing.T) {
	boom := errors.New("denied")
	for _, failure := range []string{"query", "/Change", "/End", "/Delete", "final-query", "still-registered"} {
		t.Run(failure, func(t *testing.T) {
			deleted := false
			active := true
			query := func() (bool, bool, error) {
				if failure == "query" || deleted && failure == "final-query" {
					return false, false, boom
				}
				return !deleted || failure == "still-registered", active, nil
			}
			run := func(args ...string) (string, error) {
				if args[0] == failure {
					return "", boom
				}
				if args[0] == "/End" {
					active = false
				}
				if args[0] == "/Delete" {
					deleted = true
				}
				return "", nil
			}
			err := stopWindowsTask("uninstall", query, run, func() (daemonControl, error) { return nil, os.ErrNotExist }, time.Millisecond)
			if err == nil {
				t.Fatal("reported success without confirmation")
			}
		})
	}
}
func TestStopWindowsTaskStartupAndOwnership(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "startup", true: "published"}[published], func(t *testing.T) {
			running := true
			registered := true
			p := &fakeDaemonControl{}
			query := func() (bool, bool, error) { return registered, running, nil }
			var calls []string
			run := func(args ...string) (string, error) {
				calls = append(calls, args[0])
				if args[0] == "/End" {
					running = false
				}
				if args[0] == "/Delete" {
					registered = false
				}
				return "", nil
			}
			open := func() (daemonControl, error) {
				if published {
					return p, nil
				}
				return nil, os.ErrNotExist
			}
			if err := stopWindowsTask("uninstall", query, run, open, time.Millisecond); err != nil {
				t.Fatal(err)
			}
			if registered || running {
				t.Fatal("task survived")
			}
			if !reflect.DeepEqual(calls, []string{"/Change", "/End", "/Delete"}) {
				t.Fatal(calls)
			}
			if published && (!p.closed || !reflect.DeepEqual(p.signals, []bool{false, true})) {
				t.Fatalf("ownership lost: %+v", p)
			}
		})
	}
}
func TestStopWindowsTaskAbsent(t *testing.T) {
	err := stopWindowsTask("uninstall", func() (bool, bool, error) { return false, false, nil }, func(...string) (string, error) { t.Fatal("mutated absent task"); return "", nil }, func() (daemonControl, error) { return nil, os.ErrNotExist }, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStopWindowsTaskTimeout(t *testing.T) {
	started := time.Now()
	err := stopWindowsTask("stop", func() (bool, bool, error) { return true, true, nil }, func(...string) (string, error) { return "", nil }, func() (daemonControl, error) { return nil, os.ErrNotExist }, time.Millisecond)
	if err == nil {
		t.Fatal("accepted a running task after End")
	}
	if time.Since(started) > time.Second {
		t.Fatal("timeout was not bounded")
	}
}

func TestConfirmTaskStartup(t *testing.T) {
	for _, test := range []struct {
		name                                                 string
		exists, running, published, gone, queryError, wantOK bool
	}{
		{name: "absent"},
		{name: "query failure", queryError: true},
		{name: "registration alone", exists: true},
		{name: "starting without publication", exists: true, running: true},
		{name: "dead publication", exists: true, running: true, published: true, gone: true},
		{name: "confirmed", exists: true, running: true, published: true, wantOK: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := &fakeDaemonControl{gone: test.gone}
			err := confirmTaskStartup(func() (bool, bool, error) {
				if test.queryError {
					return false, false, errors.New("denied")
				}
				return test.exists, test.running, nil
			}, func() (daemonControl, error) {
				if test.published {
					return p, nil
				}
				return nil, os.ErrNotExist
			}, time.Millisecond)
			if (err == nil) != test.wantOK {
				t.Fatalf("startup result: %v", err)
			}
			if test.published && !p.closed {
				t.Fatal("leaked process ownership")
			}
		})
	}
}

// Keep the concrete pointer return type: returning nil directly from an
// interface-valued fake would miss the conversion that caused the panic.
func TestDaemonControlResultMissingPublication(t *testing.T) {
	missing := func() (*fakeDaemonControl, error) { return nil, os.ErrNotExist }
	open := func() (daemonControl, error) { return daemonControlResult(missing()) }
	testMissingDaemonPublication(t, open)
}

func testMissingDaemonPublication(t *testing.T, open func() (daemonControl, error)) {
	t.Helper()
	p, err := open()
	if p != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing process: %v, %v", p, err)
	}
	for _, action := range []string{"stop", "uninstall"} {
		t.Run(action, func(t *testing.T) {
			for range 2 {
				err := stopWindowsTask(action, func() (bool, bool, error) { return false, false, nil }, func(...string) (string, error) {
					t.Fatal("mutated absent task")
					return "", nil
				}, open, time.Millisecond)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if err := confirmTaskStartup(func() (bool, bool, error) { return true, true, nil }, open, time.Millisecond); err == nil {
		t.Fatal("accepted startup without publication")
	}
}

func TestDaemonControlResultPreservesResult(t *testing.T) {
	boom := errors.New("open denied")
	var missing *fakeDaemonControl
	p, err := daemonControlResult(missing, boom)
	if p != nil || !errors.Is(err, boom) {
		t.Fatalf("open error lost: %v, %v", p, err)
	}
	if err := stopWindowsTask("uninstall", func() (bool, bool, error) { return false, false, nil }, nil, func() (daemonControl, error) {
		return daemonControlResult(missing, boom)
	}, time.Millisecond); !errors.Is(err, boom) {
		t.Fatalf("stop swallowed open error: %v", err)
	}
	live := &fakeDaemonControl{}
	p, err = daemonControlResult(live, nil)
	if p != live || err != nil {
		t.Fatalf("process ownership lost: %v, %v", p, err)
	}
}
