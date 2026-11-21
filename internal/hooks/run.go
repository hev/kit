package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// Defaults for [hooks] in ~/.hev/config.toml.
const (
	DefaultTimeout     = 2 * time.Second
	DefaultHaltTimeout = 10 * time.Minute
)

// Config is the [hooks] table.
type Config struct {
	Enabled     bool
	Timeout     time.Duration
	HaltTimeout time.Duration
}

// LoadConfig reads [hooks] from path. It never fails: a hook must not stop an
// agent because a config file is broken, so problems yield the defaults and a
// warning string.
func LoadConfig(path string) (Config, string) {
	c := Config{Enabled: true, Timeout: DefaultTimeout, HaltTimeout: DefaultHaltTimeout}
	info, err := os.Stat(path)
	if err != nil {
		return c, ""
	}
	// Opening a FIFO or device would block; a config is a regular file.
	if !info.Mode().IsRegular() {
		return c, fmt.Sprintf("config %s is not a regular file; using hook defaults", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, ""
	}
	var f struct {
		Hooks struct {
			Enabled     *bool  `toml:"enabled"`
			Timeout     string `toml:"timeout"`
			HaltTimeout string `toml:"halt_timeout"`
		} `toml:"hooks"`
	}
	if _, err := toml.Decode(string(b), &f); err != nil {
		return c, fmt.Sprintf("config %s: %v; using hook defaults", path, err)
	}
	var warn []string
	if f.Hooks.Enabled != nil {
		c.Enabled = *f.Hooks.Enabled
	}
	if f.Hooks.Timeout != "" {
		if d, err := time.ParseDuration(f.Hooks.Timeout); err == nil && d > 0 {
			c.Timeout = d
		} else {
			warn = append(warn, fmt.Sprintf("[hooks] timeout %q is not a positive duration; using %s", f.Hooks.Timeout, DefaultTimeout))
		}
	}
	if f.Hooks.HaltTimeout != "" {
		if d, err := time.ParseDuration(f.Hooks.HaltTimeout); err == nil && d > 0 {
			c.HaltTimeout = d
		} else {
			warn = append(warn, fmt.Sprintf("[hooks] halt_timeout %q is not a positive duration; using %s", f.Hooks.HaltTimeout, DefaultHaltTimeout))
		}
	}
	return c, strings.Join(warn, "; ")
}

// Disabled reports the kill switch: HEV_HOOKS=off (or 0, false, no).
func Disabled(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "off", "0", "false", "no", "disabled":
		return true
	}
	return false
}

// ReapGrace bounds the wait for cancelled handlers to be killed: Run returns
// at most this long after its deadline or its context ends.
const ReapGrace = 300 * time.Millisecond

// reapAfter starts the reap grace. Tests replace it to decide when the grace
// is over.
var reapAfter = func() <-chan time.Time { return time.After(ReapGrace) }

// Handler is one hook handler. Run may block; the runner bounds it.
type Handler struct {
	Name string
	Run  func(ctx context.Context, ev Event) (Response, error)
}

// Run executes handlers concurrently under one deadline and merges their
// responses in handler order. It fails open: an error, a panic or a missed
// deadline drops that handler's response and reports a warning.
func Run(ctx context.Context, ev Event, handlers []Handler, timeout time.Duration, warn func(string)) Response {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		i   int
		r   Response
		err error
	}
	results := make(chan result, len(handlers))
	for i, h := range handlers {
		go func(i int, h Handler) {
			defer func() {
				if p := recover(); p != nil {
					results <- result{i: i, err: fmt.Errorf("panic: %v\n%s", p, firstLines(string(debug.Stack()), 8))}
				}
			}()
			r, err := h.Run(ctx, ev)
			results <- result{i: i, r: r, err: err}
		}(i, h)
	}
	got := make([]*Response, len(handlers))
	answered := make([]bool, len(handlers))
	pending := len(handlers)
	for pending > 0 {
		select {
		case res := <-results:
			pending--
			answered[res.i] = true
			if res.err != nil {
				warn(fmt.Sprintf("handler %s failed on %s/%s: %v; continuing", handlers[res.i].Name, ev.Harness, ev.Event, res.err))
				continue
			}
			r := res.r
			got[res.i] = &r
		case <-ctx.Done():
			for i := range got {
				if !answered[i] {
					warn(fmt.Sprintf("handler %s did not answer %s/%s within %s; continuing without it", handlers[i].Name, ev.Harness, ev.Event, timeout))
				}
			}
			// Cancelling kills late handler processes; give that a moment to
			// land before the hook process exits and orphans them.
			cancel()
			grace := reapAfter()
			for reaping := pending; reaping > 0; reaping-- {
				select {
				case <-results:
				case <-grace:
					reaping = 0
				}
			}
			pending = 0
		}
	}
	var rs []Response
	for _, r := range got {
		if r != nil {
			rs = append(rs, *r)
		}
	}
	return Merge(rs...)
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// UserHandlers discovers hev-hook-NAME executables on PATH, first match per
// name, like plugins (docs/plugins.md).
func UserHandlers(path string) []Handler {
	var hs []Handler
	for _, u := range userExecutables(path) {
		exe := u.exe
		hs = append(hs, Handler{Name: u.name, Run: func(ctx context.Context, ev Event) (Response, error) { return runUserHandler(ctx, exe, ev) }})
	}
	return hs
}

type userExecutable struct{ name, exe string }

// userExecutables lists hev-hook-NAME executables on PATH, sorted by name.
func userExecutables(path string) []userExecutable {
	seen := map[string]bool{}
	var names []string
	exes := map[string]string{}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || !testHandlerDir(dir) {
			continue
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasPrefix(n, "hev-hook-") {
				continue
			}
			if runtime.GOOS == "windows" {
				ext := strings.ToLower(filepath.Ext(n))
				if ext != ".exe" && ext != ".cmd" && ext != ".bat" {
					continue
				}
				n = strings.TrimSuffix(n, filepath.Ext(n))
			} else if info, err := e.Info(); err != nil || info.Mode()&0111 == 0 {
				continue
			}
			name := strings.TrimPrefix(n, "hev-hook-")
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
			exes[name] = filepath.Join(dir, e.Name())
		}
	}
	sort.Strings(names)
	var out []userExecutable
	for _, name := range names {
		out = append(out, userExecutable{"hev-hook-" + name, exes[name]})
	}
	return out
}

// testHandlerDir keeps a test binary from running handlers the developer
// installed (~/.local/bin/hev-hook-slack posts to a live Slack channel):
// under `go test` only handlers a test planted in a temp directory run,
// however the test set PATH. Outside tests every directory counts.
func testHandlerDir(dir string) bool {
	if !testing.Testing() {
		return true
	}
	return underTemp(dir)
}

func underTemp(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	tmp := filepath.Clean(os.TempDir())
	return abs == tmp || strings.HasPrefix(abs, tmp+string(filepath.Separator))
}

func runUserHandler(ctx context.Context, exe string, ev Event) (Response, error) {
	in, err := json.Marshal(ev)
	if err != nil {
		return Response{}, err
	}
	c, err := handlerCommand(ctx, exe, ev.Event)
	if err != nil {
		return Response{}, err
	}
	c.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	// A grandchild holding the pipes must not outlive the deadline.
	c.WaitDelay = 100 * time.Millisecond
	ownGroup(c)
	c.Env = append(os.Environ(), "HEV_HOOK_EVENT="+ev.Event, "HEV_HOOK_HARNESS="+ev.Harness)
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return Response{}, fmt.Errorf("timed out")
		}
		return Response{}, fmt.Errorf("%v: %s", err, strings.TrimSpace(firstLines(errb.String(), 3)))
	}
	body := bytes.TrimSpace(out.Bytes())
	if len(body) == 0 {
		return Response{Decision: Continue}, nil
	}
	var r Response
	if body[0] != '{' {
		return Response{}, fmt.Errorf("handler response is not a JSON object")
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Response{}, fmt.Errorf("invalid response JSON: %v", err)
	}
	switch r.Decision {
	case "", Continue, Block, Halt:
	default:
		return Response{}, fmt.Errorf("unknown decision %q", r.Decision)
	}
	return r, nil
}

// Log is the hook layer's local record: a redacted event stream and a
// warnings log, both under ~/.hev/hooks/.
type Log struct{ Dir string }

const maxLogBytes = 32 << 20

// Append writes one JSON line, rotating the file to NAME.1 past 32 MiB.
func (l Log) Append(name string, v any) error {
	if l.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(l.Dir, 0700); err != nil {
		return err
	}
	p := filepath.Join(l.Dir, name)
	if info, err := os.Stat(p); err == nil {
		// Opening a FIFO or device for writing can block; refuse it.
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		if info.Size() > maxLogBytes {
			_ = os.Rename(p, p+".1")
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Event appends the redacted event to events.jsonl.
func (l Log) Event(ev Event, r Response, elapsed time.Duration) error {
	return l.Append("events.jsonl", struct {
		Event
		Decision  string `json:"decision"`
		Injected  int    `json:"injected_bytes,omitempty"`
		ElapsedMs int64  `json:"elapsed_ms"`
	}{ev.Redacted(), r.Decision, len(r.Context), elapsed.Milliseconds()})
}

// Warn appends a warning to warnings.log.
func (l Log) Warn(ev Event, msg string) error {
	return l.Append("warnings.log", map[string]any{"time": time.Now().UTC(), "event": ev.Event, "harness": ev.Harness, "session_id": ev.SessionID, "warning": msg})
}
