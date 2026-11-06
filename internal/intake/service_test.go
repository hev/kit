package intake

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestServiceInstallPlatforms(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			path, body, command, err := servicePlan(platform, "/home/example", "/bin/hev", "/config/intake.toml", "install")
			if err != nil || path == "" || len(command) == 0 || !strings.Contains(body, "watch") || !strings.Contains(body, "--config") {
				t.Fatalf("invalid plan: %q %q %v %v", path, body, command, err)
			}
			for _, action := range []string{"start", "stop", "status"} {
				_, _, cmd, err := servicePlan(platform, "/home/example", "/bin/hev", "/config/intake.toml", action)
				if err != nil || len(cmd) < 2 {
					t.Fatalf("%s: %v %v", action, cmd, err)
				}
			}
		})
	}
}

func TestServiceRestartPolicy(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		_, body, _, err := servicePlan(platform, "/example", "/bin/hev", "/config/intake.toml", "install")
		if err != nil {
			t.Fatal(err)
		}
		switch platform {
		case "linux":
			if !strings.Contains(body, "Restart=no") {
				t.Fatal(body)
			}
		case "darwin":
			if !strings.Contains(body, "<key>KeepAlive</key><false/>") {
				t.Fatal(body)
			}
		case "windows":
			if strings.Contains(body, "RestartOnFailure") || !strings.Contains(body, "IgnoreNew") {
				t.Fatal(body)
			}
		}
	}
}

func TestServiceInstallWithFakeManager(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			home := t.TempDir()
			calls := 0
			err := applyService(platform, home, "/bin/hev", "/config/intake.toml", "install", func(args []string) error {
				calls++
				if len(args) < 2 {
					t.Fatal(args)
				}
				return nil
			})
			expectedCalls := 1
			if platform == "darwin" {
				expectedCalls = 2
			}
			if err != nil || calls != expectedCalls {
				t.Fatalf("%v calls=%d", err, calls)
			}
			path, expected, _, _ := servicePlan(platform, home, "/bin/hev", "/config/intake.toml", "install")
			body, err := os.ReadFile(path)
			if err != nil || string(body) != expected {
				t.Fatalf("file: %v", err)
			}
			rejected := errors.New("manager refused")
			err = applyService(platform, home, "/bin/hev", "/config/intake.toml", "start", func([]string) error { return rejected })
			if !errors.Is(err, rejected) {
				t.Fatalf("lost manager failure: %v", err)
			}
		})
	}
}

func TestDarwinReinstallUnloadsBeforeBootstrap(t *testing.T) {
	var commands [][]string
	err := applyService("darwin", t.TempDir(), "/bin/hev", "/config/watch.toml", "install", func(args []string) error {
		commands = append(commands, args)
		if args[1] == "bootout" {
			return errors.New("not loaded")
		}
		return nil
	})
	if err != nil || len(commands) != 2 || commands[0][1] != "bootout" || commands[1][1] != "bootstrap" {
		t.Fatalf("%v %v", commands, err)
	}
}

func TestWindowsWatchRunsOnBattery(t *testing.T) {
	_, body, _, err := servicePlan("windows", "/home/example", "/bin/hev", "/config/watch.toml", "install")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"DisallowStartIfOnBatteries", "StopIfGoingOnBatteries"} {
		if !strings.Contains(body, "<"+setting+">false</"+setting+">") {
			t.Fatal(body)
		}
	}
}
