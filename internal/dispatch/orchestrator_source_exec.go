package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// sourceInvocation is one plugin call. Opaque values such as the cursor and
// ack IDs travel in Env and Stdin, never argv, so batch files cannot expand them.
type sourceInvocation struct {
	Args  []string
	Env   []string
	Stdin []byte
}

// Grandchildren that keep stdout open cannot hold a round past this delay.
var sourceWaitDelay = 2 * time.Second

const sourceStderrCap = 64 << 10

// sourceCommand is replaced by tests with an in-process fake plugin.
var sourceCommand = execSourcePlugin

// sourceArgs places configured source.args after the verb and before any
// verb-specific flags and operands.
func sourceArgs(s OrchestratorSpec, verb string, rest ...string) []string {
	args := append([]string{verb}, s.Source.Args...)
	return append(args, rest...)
}

// execSourcePlugin execs hev-PLUGIN directly in its own process group (Unix)
// or job object (Windows). Cancellation kills the whole tree; stderr goes to
// the supervisor log, capped per call.
func execSourcePlugin(ctx context.Context, s OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
	path, e := exec.LookPath("hev-" + s.Source.Plugin)
	if e != nil {
		return nil, fmt.Errorf("source plugin: %w", e)
	}
	c := exec.CommandContext(ctx, path, inv.Args...)
	cmdline := ""
	if strings.EqualFold(filepath.Ext(path), ".cmd") || strings.EqualFold(filepath.Ext(path), ".bat") {
		line, err := CmdPluginLine(path, inv.Args)
		if err != nil {
			return nil, err
		}
		c = exec.CommandContext(ctx, "cmd.exe")
		cmdline = "cmd.exe /d /s /c " + line
	}
	c.Env = append(orchestratorEnvironment(""), inv.Env...)
	c.Stdin = bytes.NewReader(inv.Stdin)
	var out bytes.Buffer
	c.Stdout = &out
	log, e := os.OpenFile(filepath.Join(OrchestratorDir(s.Name), "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	defer log.Close()
	stderr := &cappedWriter{w: log, prefix: "source " + s.Source.Plugin + " " + firstArg(inv.Args) + ": ", left: sourceStderrCap}
	c.Stderr = stderr
	c.WaitDelay = sourceWaitDelay
	cleanup, e := startSourceChild(c, cmdline)
	if e != nil {
		return nil, e
	}
	e = c.Wait()
	cleanup()
	stderr.flush()
	if e != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("source plugin %s: %w", firstArg(inv.Args), ctx.Err())
		}
		return nil, fmt.Errorf("source plugin %s: %w", firstArg(inv.Args), e)
	}
	return out.Bytes(), nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// cappedWriter prefixes complete lines and drops log output beyond its cap.
// It always reports success so a chatty plugin never blocks on stderr.
type cappedWriter struct {
	w         io.Writer
	prefix    string
	left      int
	partial   []byte
	truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.truncated {
		return len(p), nil
	}
	c.partial = append(c.partial, p...)
	for {
		i := bytes.IndexByte(c.partial, '\n')
		if i < 0 {
			break
		}
		c.emit(c.partial[:i])
		c.partial = c.partial[i+1:]
	}
	if len(c.partial) > c.left {
		c.emit(c.partial)
		c.partial = nil
	}
	return len(p), nil
}

func (c *cappedWriter) emit(line []byte) {
	if c.truncated {
		return
	}
	n := len(c.prefix) + len(line) + 1
	if n > c.left {
		c.truncated = true
		return
	}
	c.left -= n
	_, _ = fmt.Fprintf(c.w, "%s%s\n", c.prefix, line)
}

func (c *cappedWriter) flush() {
	if len(c.partial) > 0 {
		c.emit(c.partial)
		c.partial = nil
	}
	if c.truncated {
		_, _ = fmt.Fprintf(c.w, "%s[stderr truncated at %d bytes]\n", c.prefix, sourceStderrCap)
	}
}

// CmdPluginLine quotes a batch plugin invocation for cmd.exe /s /c. Batch
// files cannot represent arbitrary argv safely, so shell syntax is refused
// rather than letting cmd expand user arguments or executable paths.
func CmdPluginLine(path string, args []string) (string, error) {
	parts := append([]string{path}, args...)
	for i, part := range parts {
		if strings.ContainsAny(part, "\"^&|<>()%!\r\n") {
			return "", fmt.Errorf("unsafe metacharacter in batch plugin path or argument")
		}
		parts[i] = "\"" + part + "\""
	}
	return "\"" + strings.Join(parts, " ") + "\"", nil
}
