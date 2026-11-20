// Package local is what kit still knows about Docker: the containers of the
// retired local archive lane (Layer CE on Postgres, and the BYO Turbopuffer
// gateway) that `hev up` may start once to export their rows into the file
// archive. kit runs nothing in Docker; it never removes a container or a
// volume. It drives the docker CLI rather than the Engine API so that
// whatever `docker` the user has — Desktop, Colima, OrbStack — is the one
// that gets used.
package local

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// Container is one container of a Compose project.
type Container struct {
	ID      string
	Service string
	Running bool
}

// Docker reports why the docker CLI cannot be used, or nil.
func Docker(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is not on PATH")
	}
	if out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		return fmt.Errorf("docker is not running%s", lastLine(out))
	}
	return nil
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return " (" + last + ")"
	}
	return ""
}

// Containers lists a Compose project's containers by its label, so no
// Compose file is needed.
func Containers(ctx context.Context, project string) ([]Container, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", `{{.ID}} {{.State}} {{.Label "com.docker.compose.service"}}`).Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	var cs []Container
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		c := Container{ID: f[0], Running: f[1] == "running"}
		if len(f) > 2 {
			c.Service = f[2]
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// Start starts the project's stopped containers and returns the ids it
// started, for Stop to put back as they were.
func Start(ctx context.Context, cs []Container) ([]string, error) {
	var ids []string
	for _, c := range cs {
		if !c.Running {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"start"}, ids...)...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker start: %w%s", err, lastLine(out))
	}
	return ids, nil
}

// Stop stops containers (never removes them).
func Stop(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"stop"}, ids...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w%s", err, lastLine(out))
	}
	return nil
}

// PortFree checks the IPv4 loopback address. `up` refuses a busy port rather
// than moving to another: an endpoint that silently differs from the
// documented one is worse than a one-line error naming the override.
func PortFree(port int) bool {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	// On macOS, SO_REUSEADDR can let a specific-address bind coexist with
	// an existing wildcard listener. Detect that listener on the actual
	// address before testing whether we can reserve the port.
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
