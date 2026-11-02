package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTargetCapAndSenderLimits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(t.TempDir(), "target.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if err := os.WriteFile(cfg, []byte("cap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// A live reservation represents a request admitted from the first sender.
	dir := filepath.Join(Root(), "first")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := reserve(dir); err != nil {
		t.Fatal(err)
	}
	for _, sender := range []struct {
		name string
		cap  int
		full bool
	}{
		{"lower", 1, true}, {"higher", 100, false}, {"unset", 0, false}, {"negative", -1, false},
	} {
		t.Run(sender.name, func(t *testing.T) {
			_, err := Handle(Request{Action: "start", Name: "second", Cap: sender.cap, Order: "invalid"})
			if err == nil || strings.Contains(err.Error(), "host full") != sender.full {
				t.Fatalf("unexpected admission: %v", err)
			}
		})
	}
	second := filepath.Join(Root(), "second")
	if err := os.MkdirAll(second, 0700); err != nil {
		t.Fatal(err)
	}
	if err := reserve(second); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []int{1, 100} {
		_, err := Handle(Request{Action: "start", Name: "third", Cap: cap, Order: good})
		if err == nil || !strings.Contains(err.Error(), "host full") {
			t.Fatalf("sender cap %d bypassed target: %v", cap, err)
		}
	}
	inv := Inventory(Config{Hosts: map[string]Host{"target": {Local: true, Cap: 100}}}, "")
	if r := inv["target"]; r.Error != "" || r.Cap != 2 || r.Free != 0 || r.Running != 2 {
		t.Fatalf("inventory: %+v", r)
	}
}

func TestTargetDefaultConfigFailsClosed(t *testing.T) {
	for _, content := range []string{"invalid = [", "cap=0", "cap=-1", "[hosts.other.admission]\nrepos=[]", "[hosts.other]\nadmission={repos=[]}"} {
		t.Run(content, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cfg := filepath.Join(t.TempDir(), "target.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			if err := os.WriteFile(cfg, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Handle(Request{Action: "start", Name: "example", Cap: 100, Order: good, Meta: Meta{ConfigPath: os.DevNull}})
			if err == nil || !strings.Contains(err.Error(), "target admission config") {
				t.Fatalf("invalid config admitted: %v", err)
			}
			if strings.Contains(content, "admission") && !strings.Contains(err.Error(), "top-level [admission]") {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(Root(), "example")); !os.IsNotExist(err) {
				t.Fatalf("created record: %v", err)
			}
		})
	}
	t.Run("missing-explicit-default", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", os.Getenv("HOME"))
		t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(t.TempDir(), "missing"))
		_, err := Handle(Request{Action: "start", Name: "example", Cap: 100, Order: good})
		if err == nil || !strings.Contains(err.Error(), "target admission config") {
			t.Fatal(err)
		}
	})
}

func TestLegacyTargetCapacityUpgrade(t *testing.T) {
	const hostname = "Box-Mac-mini.local"
	for _, tc := range []struct {
		name, config string
		want         int
	}{
		{"local", "[hosts.here]\nlocal=true\ncap=8\n", 8},
		{"hostname", "[hosts.\"" + hostname + "\"]\nssh='target'\ncap=8\n", 8},
		{"short-hostname", "[hosts.Box-Mac-mini]\nssh='target'\ncap=8\n", 8},
		{"case-insensitive", "[hosts.box-mac-MINI]\nssh='target'\ncap=8\n", 8},
		{"local-first", "[hosts.here]\nlocal=true\ncap=8\n[hosts.Box-Mac-mini]\nssh='target'\ncap=4\n", 8},
		{"ambiguous-local", "[hosts.one]\nlocal=true\ncap=4\n[hosts.two]\nlocal=true\ncap=5\n[hosts.Box-Mac-mini]\nssh='target'\ncap=8\n", 8},
		{"unmatched", "[hosts.other]\nssh='target'\ncap=8\n", 2},
		{"explicit", "cap=3\n[hosts.here]\nlocal=true\ncap=8\n", 3},
		{"default", "[hosts.here]\nlocal=true\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "target.toml")
			if err := os.WriteFile(cfg, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			originalStderr := os.Stderr
			os.Stderr = stderr
			c, err := loadTarget(cfg, hostname)
			os.Stderr = originalStderr
			if closeErr := stderr.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			warning, readErr := os.ReadFile(stderr.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.want == 2 && (!strings.Contains(string(warning), "warning:") || !strings.Contains(string(warning), "top-level cap")) {
				t.Fatalf("missing default cap warning: %s", warning)
			}
			if err != nil || c.Cap != tc.want {
				t.Fatalf("cap=%d, want %d, err=%v", c.Cap, tc.want, err)
			}
			if tc.want == 8 {
				if err := CheckCap(7, c.Cap); err != nil {
					t.Fatal(err)
				}
				if err := CheckCap(8, c.Cap); err == nil {
					t.Fatal("ninth request admitted")
				}
			}
		})
	}
}

func TestLegacyTargetCapacityMachineHostname(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "target.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.synthetic]\nlocal=true\ncap=8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadTarget(cfg, "")
	if err != nil || c.Cap != 8 {
		t.Fatalf("cap=%d, err=%v", c.Cap, err)
	}
}
