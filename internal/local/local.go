// Package local runs the stack that `hev up` owns on the Turbopuffer lane:
// the Layer CE gateway in front of the user's own Turbopuffer account, and the
// kit dashboard, from a Compose file vendored into the binary. It drives the
// docker CLI rather than the Engine API so that whatever `docker` the user has
// — Desktop, Colima, OrbStack — is the one that gets used.
package local

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/hev/kit/internal/layerenv"
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

// StoreTurbopuffer is the store kind the `[layer]` block names on this lane.
const StoreTurbopuffer = "turbopuffer"

// Stack is one Compose project. Image, Port, Project, KitImage and ServePort
// are the `[local]` block; Namespace and APIKey are the `[layer]` values the
// gateway runs on and the dashboard reads with. APIKey is the Turbopuffer key.
// Dir is where the vendored Compose file is materialized, because `down` must
// be able to find it after the terminal that ran `up` is gone.
type Stack struct {
	Image     string
	Port      int
	Project   string
	KitImage  string
	ServePort int
	Namespace string
	APIKey    string
	Dir       string
	Log       io.Writer
	// KeepOrphans leaves containers of the project that the Compose file no
	// longer names. It is set when the config still names the retired local
	// store, whose containers kit never removes.
	KeepOrphans bool
}

// Services are the Compose services `up` starts, in the order they come up.
var Services = []string{"gateway", "dashboard"}

// Services is every service of the stack, in the order they come up.
func (s Stack) Services() []string { return Services }

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
// so this is safe to run on a stack that is already up. Orphans (containers
// of the project that this file no longer names) are removed; volumes never
// are.
func (s Stack) Up(ctx context.Context, services ...string) error {
	files, err := s.materialize()
	if err != nil {
		return err
	}
	args := []string{"up", "--detach", "--wait", "--remove-orphans"}
	if s.KeepOrphans {
		args = args[:3]
	}
	return s.compose(ctx, files, append(args, services...)...)
}

// Down stops and removes the project's containers. Volumes are kept; the
// archive is in the user's Turbopuffer account.
func (s Stack) Down(ctx context.Context) error {
	files, err := s.materialize()
	if err != nil {
		return err
	}
	if s.KeepOrphans {
		return s.compose(ctx, files, "down")
	}
	return s.compose(ctx, files, "down", "--remove-orphans")
}

func (s Stack) compose(ctx context.Context, files []string, args ...string) error {
	argv := []string{"compose", "-p", s.Project}
	for _, f := range files {
		argv = append(argv, "-f", f)
	}
	cmd := exec.CommandContext(ctx, "docker", append(argv, args...)...)
	// Every variable the file reads is set from the Stack, never inherited,
	// so that what runs is what the config says.
	cmd.Env = append(os.Environ(),
		"GATEWAY_IMAGE="+s.Image,
		fmt.Sprintf("GATEWAY_PORT=%d", s.Port),
		"KIT_IMAGE="+s.KitImage,
		"KIT_VERSION="+kitVersion(),
		fmt.Sprintf("SERVE_PORT=%d", s.ServePort),
		"HEVLAYER_NAMESPACE="+s.Namespace,
		"HEVLAYER_STORE="+StoreTurbopuffer,
		"HEVLAYER_API_KEY="+s.APIKey,
		"HEV_GATEWAY_KEY_ENV="+layerenv.GatewayKey,
		"HEV_GATEWAY_API_KEY="+s.APIKey,
		"TPUF_URL=",
		"HEVLAYER_LICENSE=",
		"HEVLAYER_TELEMETRY="+layerenv.Get("HEVLAYER_TELEMETRY"),
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

// materialize writes the vendored Compose file where docker can read it,
// leaving an identical file untouched.
func (s Stack) materialize() ([]string, error) {
	path := filepath.Join(s.Dir, "docker-compose.yml")
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, composeFile) {
		return []string{path}, nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, composeFile, 0o644); err != nil {
		return nil, err
	}
	return []string{path}, nil
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
