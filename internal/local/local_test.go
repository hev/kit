package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker puts a `docker` on PATH that logs its arguments and the
// environment kit controls, and exits with infoExit for `docker info`.
func fakeDocker(t *testing.T, infoExit int) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "docker.log")
	script := `#!/bin/sh
echo "docker $* | GATEWAY_IMAGE=$GATEWAY_IMAGE GATEWAY_PORT=$GATEWAY_PORT KIT_IMAGE=$KIT_IMAGE SERVE_PORT=$SERVE_PORT LAYER_NAMESPACE=$LAYER_NAMESPACE TURBOPUFFER_API_KEY=[$TURBOPUFFER_API_KEY]" >> "` + log + `"
if [ "$1" = info ]; then
  [ ` + itoa(infoExit) + ` -eq 0 ] || echo "Cannot connect to the Docker daemon at unix:///var/run/docker.sock." >&2
  exit ` + itoa(infoExit) + `
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return log
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return "1"
}

func TestPreflightDockerMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Preflight(context.Background())
	if err == nil {
		t.Fatal("no docker on PATH passed preflight")
	}
	oneLineHint(t, err, "docker not found", "hev up")
}

func TestPreflightDockerStopped(t *testing.T) {
	fakeDocker(t, 1)
	err := Preflight(context.Background())
	if err == nil {
		t.Fatal("a stopped docker passed preflight")
	}
	oneLineHint(t, err, "docker is not running", "start it", "hev up")
}

func TestPreflightDockerRunning(t *testing.T) {
	fakeDocker(t, 0)
	if err := Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func oneLineHint(t *testing.T, err error, wants ...string) {
	t.Helper()
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Fatalf("hint is not one line: %q", msg)
	}
	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Fatalf("hint %q does not say %q", msg, want)
		}
	}
}

func TestComposeRunsWithTheConfiguredKeyAndNothingInherited(t *testing.T) {
	log := fakeDocker(t, 0)
	// What the shell exports must not reach the stack: the key and every pin
	// come from the Stack, which is the config.
	t.Setenv("TURBOPUFFER_API_KEY", "tpuf_shell_key")
	t.Setenv("GATEWAY_IMAGE", "someone/elses:image")
	t.Setenv("KIT_IMAGE", "someone/elses:kit")
	s := Stack{
		Image: "hevlayer/layer-gateway:edge", Port: 18080, Project: "hev-test",
		KitImage: "hevlayer/kit:0.1.0", ServePort: 18099, Namespace: "hev-traces",
		APIKey: "tpuf_config_key", Dir: t.TempDir(),
	}
	if err := s.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Up(context.Background(), "gateway"); err != nil {
		t.Fatal(err)
	}
	// down is run from a config whose key it does not need.
	s.APIKey = ""
	if err := s.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	file := filepath.Join(s.Dir, "docker-compose.yml")
	env := "GATEWAY_IMAGE=hevlayer/layer-gateway:edge GATEWAY_PORT=18080 KIT_IMAGE=hevlayer/kit:0.1.0 SERVE_PORT=18099 LAYER_NAMESPACE=hev-traces"
	want := []string{
		"docker compose -p hev-test -f " + file + " up --detach --wait --remove-orphans | " + env + " TURBOPUFFER_API_KEY=[tpuf_config_key]",
		"docker compose -p hev-test -f " + file + " up --detach --wait --remove-orphans gateway | " + env + " TURBOPUFFER_API_KEY=[tpuf_config_key]",
		"docker compose -p hev-test -f " + file + " down --remove-orphans | " + env + " TURBOPUFFER_API_KEY=[unset]",
	}
	if len(lines) != len(want) {
		t.Fatalf("docker calls:\n%s", raw)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("call %d\n got %s\nwant %s", i, lines[i], want[i])
		}
	}
	vendored, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(vendored), "follows hev/layer-pro public/ce/docker-compose.yml") {
		t.Fatalf("compose file not materialized with its source recorded: %v", err)
	}
	for _, service := range Services {
		if !strings.Contains(string(vendored), "\n  "+service+":\n") {
			t.Fatalf("compose file has no %s service", service)
		}
	}
}

func TestCheckPin(t *testing.T) {
	for _, c := range []struct {
		image, version string
		ok             bool
	}{
		{"hevlayer/layer-gateway:edge", "0.6.0-dev", true},
		{"hevlayer/layer-gateway:v0.6.1", "0.6.1", true},
		{"hevlayer/layer-gateway:v0.6.1", "0.6.0-dev", false},
		{"hevlayer/layer-gateway:0.5.2", "0.5.2", true},
		{"hevlayer/layer-gateway:0.5.2", "0.6.0-dev", false},
		{"localhost:5000/layer-gateway", "0.6.0", true},
	} {
		if err := CheckPin(c.image, c.version); (err == nil) != c.ok {
			t.Errorf("CheckPin(%q, %q) = %v", c.image, c.version, err)
		}
	}
}
