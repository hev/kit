package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeUserService struct {
	calls       []string
	fail        bool
	unavailable bool
}

func (f *fakeUserService) Available() bool { return !f.unavailable }
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := ensureSystemd(home, "/path with spaces/hev", "/config%$name", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(systemdPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), managedUnitMarker+"\n") || !strings.Contains(string(raw), `Environment="PATH=`) {
		t.Fatal(string(raw))
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.MkdirAll(filepath.Dir(systemdPath(home)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(systemdPath(home), []byte(managedUnitMarker+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
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

func TestSystemdRespectsOperator(t *testing.T) {
	onOS(t, "linux")
	for _, kind := range []string{"unit", "drop-in", "wrapper", "missing"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			old := linuxService
			defer func() { linuxService = old }()
			fake := &fakeUserService{}
			linuxService = fake
			path := systemdPath(home)
			content := []byte("operator content\n")
			switch kind {
			case "unit":
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
			case "drop-in":
				if err := os.MkdirAll(path+".d", 0755); err != nil {
					t.Fatal(err)
				}
			case "wrapper":
				path = filepath.Join(home, ".local", "bin", "hev")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, content, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if kind != "missing" {
				if _, _, err := installSelf(home); !errors.Is(err, errUnmanaged) {
					t.Fatalf("installSelf: %v", err)
				}
				if err := upSystemd(home, "/config", false); !errors.Is(err, errUnmanaged) {
					t.Fatalf("up: %v", err)
				}
			}
			if err := downSystemd(home); !errors.Is(err, errUnmanaged) {
				t.Fatalf("down: %v", err)
			}
			if len(fake.calls) != 0 {
				t.Fatal(fake.calls)
			}
			if kind == "unit" || kind == "wrapper" {
				raw, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(raw, content) {
					t.Fatalf("changed %s: %v", path, err)
				}
			}
		})
	}
}
func TestDownSystemdNeverInstalled(t *testing.T) {
	onOS(t, "linux")
	home, _ := sandbox(t, "0")
	managed(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	old := linuxService
	defer func() { linuxService = old }()
	fake := &fakeUserService{}
	linuxService = fake
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 || !strings.Contains(out.String(), "stop `hev d` yourself") {
		t.Fatalf("%v %s", fake.calls, out.String())
	}
}

func TestSystemdWithoutUserBus(t *testing.T) {
	onOS(t, "linux")
	home, _ := sandbox(t, "0")
	managed(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	old := linuxService
	defer func() { linuxService = old }()
	fake := &fakeUserService{unavailable: true}
	linuxService = fake
	if hasSystemd() {
		t.Fatal("user bus unavailable")
	}
	var out bytes.Buffer
	if err := runDown(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 || !strings.Contains(out.String(), "stop `hev d` yourself") {
		t.Fatalf("%v %s", fake.calls, out.String())
	}
}
