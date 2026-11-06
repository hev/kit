package intake

import (
	"strings"
	"testing"
)

func TestServiceInstallPlatforms(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			path, body, command, err := servicePlan(platform, "/home/example", "/bin/hev", "/config/intake.toml", "install")
			if err != nil || path == "" || len(command) == 0 || !strings.Contains(body, "intake") || !strings.Contains(body, "--config") {
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
