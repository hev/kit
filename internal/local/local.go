// Package local runs the stack that `hev up` owns: the Layer CE gateway, in
// front of Layer CE's own Postgres store and CPU embedding sidecar by default
// or of the user's Turbopuffer account, and the kit dashboard, from Compose
// files vendored into the binary. It drives the
// docker CLI rather than the Engine API so that whatever `docker` the user has
// — Desktop, Colima, OrbStack — is the one that gets used.
package local

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/version"
)

//go:embed docker-compose.yml
var composeFile []byte

// postgresFile lays the Postgres store and the embedding sidecar over
// composeFile. It is the free lane: no key selects it.
//
//go:embed docker-compose.postgres.yml
var postgresFile []byte

// Store kinds, as the `[layer]` block names them.
const (
	StorePostgres    = "pgvector"
	StoreTurbopuffer = "turbopuffer"
)

// Stack is one Compose project. Image, EmbedImage, Port, Project, KitImage
// and ServePort are the `[local]` block; Store, Namespace and APIKey are the
// `[layer]` values the gateway runs on and the dashboard reads with. APIKey is
// the Turbopuffer key on that lane and empty on Postgres. Dir is where the
// vendored Compose files are materialized, because `down` must be able to find
// them after the terminal that ran `up` is gone.
type Stack struct {
	Store      string
	Image      string
	EmbedImage string
	Port       int
	Project    string
	KitImage   string
	ServePort  int
	Namespace  string
	APIKey     string
	Dir        string
	Log        io.Writer
}

// Services are the Compose services `up` starts on the Turbopuffer lane, in
// the order they come up.
var Services = []string{"gateway", "dashboard"}

// postgresServices come up before the gateway on the Postgres lane.
var postgresServices = []string{"postgres", "embed"}

// Postgres reports whether the stack runs the local Postgres store.
func (s Stack) Postgres() bool { return s.Store == StorePostgres }

// Services is every service of this stack's lane, in the order they come up.
func (s Stack) Services() []string {
	if s.Postgres() {
		return append(append([]string{}, postgresServices...), Services...)
	}
	return Services
}

func (s Stack) Endpoint() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

func (s Stack) DashboardURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.ServePort) }

// Preflight checks the one prerequisite an installer cannot supply. Its error
// is a single line that says how to fix it.
func Preflight(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found: install it (%s), start it, then run `hev up` again", installHint())
	}
	if out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		return fmt.Errorf("docker is not running: start it (%s), then run `hev up` again%s", startHint(), lastLine(out))
	}
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		return fmt.Errorf("docker compose is missing: install the Compose plugin (%s), then run `hev up` again", installHint())
	}
	return nil
}

func installHint() string {
	if runtime.GOOS == "darwin" {
		return "brew install --cask docker"
	}
	return "https://docs.docker.com/engine/install/"
}

func startHint() string {
	if runtime.GOOS == "darwin" {
		return "open -a Docker"
	}
	return "sudo systemctl start docker"
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return " (" + last + ")"
	}
	return ""
}

// Containers returns the id of each of the project's service containers that
// exists, keyed by service. An empty map means the project is not up.
func (s Stack) Containers(ctx context.Context) map[string]string {
	ids := map[string]string{}
	for _, service := range s.Services() {
		out, err := exec.CommandContext(ctx, "docker", "compose", "-p", s.Project, "ps", "-q", service).Output()
		id := strings.TrimSpace(string(out))
		if err == nil && id != "" && !strings.ContainsAny(id, " \n") {
			ids[service] = id
		}
	}
	return ids
}

// PortFree checks the IPv4 loopback address published by Compose. `up` refuses a busy
// port rather than moving to another: an endpoint that silently differs from
// the documented one is worse than a one-line error naming the override.
func PortFree(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	// On macOS, SO_REUSEADDR can let a specific-address bind coexist with
	// an existing wildcard listener. Detect that listener on the actual
	// publish address before testing whether we can reserve the port.
	if conn, err := net.DialTimeout("tcp4", address, 200*time.Millisecond); err == nil {
		conn.Close()
		return false
	}
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// Up brings the named services up (all of them when none are named) and waits
// for every healthcheck. Compose leaves a container whose configuration has
// not changed alone and recreates one whose has — a new key, a new image pin —
// so this is safe to run on a stack that is already up. Orphans are removed:
// on the Turbopuffer lane that is the Postgres and embed containers a free
// `up` left, which the archive no longer names.
func (s Stack) Up(ctx context.Context, services ...string) error {
	files, err := s.materialize(s.Postgres())
	if err != nil {
		return err
	}
	return s.compose(ctx, files, append([]string{"up", "--detach", "--wait", "--remove-orphans"}, services...)...)
}

// Down stops and removes the project's containers, whichever lane started
// them. Volumes are kept: on the Postgres lane the archive is in one, and on
// the Turbopuffer lane it is in the user's account.
func (s Stack) Down(ctx context.Context) error {
	files, err := s.materialize(true)
	if err != nil {
		return err
	}
	return s.compose(ctx, files, "down", "--remove-orphans")
}

func (s Stack) compose(ctx context.Context, files []string, args ...string) error {
	argv := []string{"compose", "-p", s.Project}
	for _, f := range files {
		argv = append(argv, "-f", f)
	}
	cmd := exec.CommandContext(ctx, "docker", append(argv, args...)...)
	// Every variable the files read is set from the Stack, never inherited, so
	// that what runs is what the config says. An empty TURBOPUFFER_API_KEY is
	// what selects Postgres in the gateway; the dashboard's bearer is then the
	// placeholder the gateway accepts without one.
	bearer := s.APIKey
	if bearer == "" {
		bearer = "local"
	}
	store := StoreTurbopuffer
	if s.Postgres() {
		store = StorePostgres
	}
	cmd.Env = append(os.Environ(),
		"GATEWAY_IMAGE="+s.Image,
		"EMBED_IMAGE="+s.EmbedImage,
		fmt.Sprintf("GATEWAY_PORT=%d", s.Port),
		"KIT_IMAGE="+s.KitImage,
		"KIT_VERSION="+kitVersion(),
		fmt.Sprintf("SERVE_PORT=%d", s.ServePort),
		"LAYER_NAMESPACE="+s.Namespace,
		"LAYER_STORE="+store,
		"LAYER_API_KEY="+bearer,
		"TURBOPUFFER_API_KEY="+s.APIKey,
		"TPUF_URL=",
		"HEVLAYER_LICENSE=",
	)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if s.Log != nil {
		cmd.Stdout, cmd.Stderr = io.MultiWriter(&buf, s.Log), io.MultiWriter(&buf, s.Log)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w%s", args[0], err, lastLine(buf.Bytes()))
	}
	return nil
}

// kitVersion is what the gateway's telemetry reports as the kit release that
// started it: the release this binary was built from, or "dev".
func kitVersion() string {
	if version.Version == "" {
		return "dev"
	}
	return version.Version
}

// materialize writes the vendored Compose files where docker can read them,
// leaving an identical file untouched, and returns the ones a lane runs: the
// base file, and the Postgres overlay when postgres is set.
func (s Stack) materialize(postgres bool) ([]string, error) {
	files := []struct {
		name string
		body []byte
	}{{"docker-compose.yml", composeFile}, {"docker-compose.postgres.yml", postgresFile}}
	if !postgres {
		files = files[:1]
	}
	var paths []string
	for _, f := range files {
		path := filepath.Join(s.Dir, f.name)
		paths = append(paths, path)
		if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, f.body) {
			continue
		}
		if err := os.MkdirAll(s.Dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, f.body, 0o644); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

var releaseTag = regexp.MustCompile(`:v?(\d+\.\d+\.\d+\S*)$`)

// CheckPin compares the version a gateway reports against a release pin. A
// moving tag such as `edge` names no version, so there is nothing to compare.
func CheckPin(image, version string) error {
	m := releaseTag.FindStringSubmatch(image)
	if m == nil || m[1] == version {
		return nil
	}
	return fmt.Errorf("gateway reports version %s but [local] image pins %s", version, image)
}

// ShortImage is the image without its registry namespace, for status lines.
func ShortImage(image string) string {
	return image[strings.LastIndex(image, "/")+1:]
}
