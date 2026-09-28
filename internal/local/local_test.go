package local

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/version"
)

// fakeDocker puts a `docker` on PATH that logs its arguments and the
// environment kit controls, and exits with infoExit for `docker info`.
func fakeDocker(t *testing.T, infoExit int) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "docker.log")
	script := `#!/bin/sh
echo "docker $* | GATEWAY_IMAGE=$GATEWAY_IMAGE GATEWAY_PORT=$GATEWAY_PORT KIT_IMAGE=$KIT_IMAGE KIT_VERSION=$KIT_VERSION SERVE_PORT=$SERVE_PORT LAYER_NAMESPACE=$LAYER_NAMESPACE LAYER_STORE=$LAYER_STORE LAYER_API_KEY=[$LAYER_API_KEY] EMBED_IMAGE=$EMBED_IMAGE TURBOPUFFER_API_KEY=[$TURBOPUFFER_API_KEY]" >> "` + log + `"
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
	t.Setenv("EMBED_IMAGE", "someone/elses:embed")
	t.Setenv("KIT_VERSION", "9.9.9")
	setVersion(t, "0.1.0")
	s := Stack{
		Store: StoreTurbopuffer, Image: "hevlayer/layer-gateway:edge", EmbedImage: "hevlayer/layer-embed:edge",
		Port: 18080, Project: "hev-test",
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
	// The free lane: no key reaches the gateway, which is what selects
	// Postgres, and the Postgres overlay is laid over the base file.
	pg := s
	pg.Store = StorePostgres
	if err := pg.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	file := filepath.Join(s.Dir, "docker-compose.yml")
	overlay := filepath.Join(s.Dir, "docker-compose.postgres.yml")
	env := "GATEWAY_IMAGE=hevlayer/layer-gateway:edge GATEWAY_PORT=18080 KIT_IMAGE=hevlayer/kit:0.1.0 KIT_VERSION=0.1.0 SERVE_PORT=18099 LAYER_NAMESPACE=hev-traces"
	tpuf := env + " LAYER_STORE=turbopuffer LAYER_API_KEY=[tpuf_config_key] EMBED_IMAGE=hevlayer/layer-embed:edge TURBOPUFFER_API_KEY=[tpuf_config_key]"
	want := []string{
		"docker compose -p hev-test -f " + file + " up --detach --wait --remove-orphans | " + tpuf,
		"docker compose -p hev-test -f " + file + " up --detach --wait --remove-orphans gateway | " + tpuf,
		"docker compose -p hev-test -f " + file + " -f " + overlay + " down --remove-orphans | " + env + " LAYER_STORE=turbopuffer LAYER_API_KEY=[local] EMBED_IMAGE=hevlayer/layer-embed:edge TURBOPUFFER_API_KEY=[]",
		"docker compose -p hev-test -f " + file + " -f " + overlay + " up --detach --wait --remove-orphans | " + env + " LAYER_STORE=pgvector LAYER_API_KEY=[local] EMBED_IMAGE=hevlayer/layer-embed:edge TURBOPUFFER_API_KEY=[]",
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
	both, _ := os.ReadFile(overlay)
	both = append(vendored, both...)
	for _, service := range pg.Services() {
		if !strings.Contains(string(both), "\n  "+service+":\n") {
			t.Fatalf("compose files have no %s service", service)
		}
	}
	// Neither file can refuse to start for want of a key.
	if strings.Contains(string(both), ":?") {
		t.Fatal("a compose variable is required")
	}
	if got := strings.Join(pg.Services(), " "); got != "postgres embed gateway dashboard" {
		t.Fatalf("postgres services = %s", got)
	}
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	was := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = was })
}

// The gateway's telemetry names kit and the release running `hev up`
// (LYR-141): a fixed source, and the version the compose env carries, which
// is this binary's and "dev" outside a release build.
func TestTelemetryNamesKitAndItsVersion(t *testing.T) {
	gateway := string(composeFile)
	gateway = gateway[strings.Index(gateway, "\n  gateway:\n"):strings.Index(gateway, "\n  dashboard:\n")]
	for _, want := range []string{
		"\n      LAYER_TELEMETRY_SOURCE: kit\n",
		"\n      LAYER_TELEMETRY_SOURCE_VERSION: ${KIT_VERSION:-dev}\n",
		// The opt-outs still pass through from the shell.
		"\n      LAYER_TELEMETRY: ${LAYER_TELEMETRY:-}\n",
		"\n      DO_NOT_TRACK: ${DO_NOT_TRACK:-}\n",
	} {
		if !strings.Contains(gateway, want) {
			t.Errorf("gateway environment has no %q", strings.TrimSpace(want))
		}
	}
	for v, want := range map[string]string{"0.3.3": "0.3.3", "dev": "dev", "": "dev"} {
		setVersion(t, v)
		if got := kitVersion(); got != want {
			t.Errorf("Version %q: KIT_VERSION = %q, want %q", v, got, want)
		}
	}
}

// With a real docker compose on PATH, the rendered gateway environment carries
// kit's version and the shell's opt-out, on both lanes.
func TestComposeRendersTelemetrySource(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("no docker compose")
	}
	setVersion(t, "0.3.3")
	t.Setenv("KIT_VERSION", "9.9.9")
	t.Setenv("DO_NOT_TRACK", "1")
	for _, store := range []string{StoreTurbopuffer, StorePostgres} {
		s := Stack{
			Store: store, Image: "hevlayer/layer-gateway:0.7.3", EmbedImage: "hevlayer/layer-embed:0.7.3",
			Port: 18080, Project: "hev-test", KitImage: "hevlayer/kit:0.3.3", ServePort: 18099,
			Namespace: "hev-traces", Dir: t.TempDir(),
		}
		files, err := s.materialize(s.Postgres())
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		s.Log = &out
		if err := s.compose(context.Background(), files, "config", "--format", "json"); err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Services map[string]struct {
				Environment map[string]*string
			}
		}
		if err := json.Unmarshal([]byte(out.String()), &cfg); err != nil {
			t.Fatalf("%s: %v\n%s", store, err, out.String())
		}
		env := cfg.Services["gateway"].Environment
		for k, want := range map[string]string{"LAYER_TELEMETRY_SOURCE": "kit", "LAYER_TELEMETRY_SOURCE_VERSION": "0.3.3", "DO_NOT_TRACK": "1"} {
			if env[k] == nil || *env[k] != want {
				t.Errorf("%s: gateway %s = %v, want %q", store, k, env[k], want)
			}
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
