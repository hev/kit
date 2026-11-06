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
			if err != nil || calls != 1 {
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
