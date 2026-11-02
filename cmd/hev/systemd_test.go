package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeUserService struct {
	calls []string
	fail  bool
}

func (*fakeUserService) Available() bool { return true }
func (f *fakeUserService) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if f.fail {
		return []byte("no user bus"), errors.New("failed")
	}
	return []byte("active"), nil
}
func TestSystemdLifecycle(t *testing.T) {
	old := linuxService
	defer func() { linuxService = old }()
	fake := &fakeUserService{}
	linuxService = fake
	home := t.TempDir()
	if err := ensureSystemd(home, "/path with spaces/hev", "/config%$name", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(systemdPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `ExecStart="/path with spaces/hev" daemon`) {
		t.Fatal(string(raw))
	}
	if !strings.Contains(string(raw), `Environment="HEV_CONFIG=/config%%$name"`) {
		t.Fatal(string(raw))
	}
	if got := strings.Join(fake.calls, ";"); got != "daemon-reload;enable --now hevd.service;restart hevd.service" {
		t.Fatal(got)
	}
	fake.calls = nil
	if err := ensureSystemd(home, "/path with spaces/hev", "/config%$name", false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fake.calls, ";"); got != "enable --now hevd.service" {
		t.Fatal(got)
	}
	if err := systemdRun("disable", "--now", daemonUnit); err != nil {
		t.Fatal(err)
	}
	fake.fail = true
	if err := ensureSystemd(home, "/path with spaces/hev", "/config%$name", false); err == nil || !strings.Contains(err.Error(), "no user bus") {
		t.Fatal(err)
	}
}

func TestDownSystemd(t *testing.T) {
	onOS(t, "linux")
	home, log := sandbox(t, "0")
	managed(t, home)
	old := linuxService
	defer func() { linuxService = old }()
	fake := &fakeUserService{}
	linuxService = fake
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fake.calls, ";") != "disable --now hevd.service" {
		t.Fatal(fake.calls)
	}
	if strings.Contains(calls(t, log), "launchctl") {
		t.Fatal("Linux called launchctl")
	}
}
