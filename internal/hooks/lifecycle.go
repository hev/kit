package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/feed"
)

// LifecycleEvents are the run-lifecycle events kit raises itself, outside
// any harness session: attention notices (https://hevkit.com/docs/config-reference,
// schemas/attention_event.schema.json). Handlers observe them; their output
// is ignored.
var LifecycleEvents = []string{"needs-you", "ready", "done"}

// ValidLifecycleEvent reports whether event is a run-lifecycle event.
func ValidLifecycleEvent(event string) bool {
	for _, e := range LifecycleEvents {
		if e == event {
			return true
		}
	}
	return false
}

// RunLifecycle delivers one run-lifecycle event to every hev-hook-NAME
// handler on path, concurrently, under the [hooks] budget. It fails open like
// harness hooks: a handler that errors, exits non-zero or misses the budget
// is killed and logged to warnings.log, and the caller is never failed. The
// kill switch (HEV_HOOKS=off, [hooks] enabled = false) skips handlers.
func RunLifecycle(ctx context.Context, event string, payload []byte, path string, cfg Config, log Log) {
	if !cfg.Enabled || Disabled(os.Getenv("HEV_HOOKS")) {
		return
	}
	// The Slack feed is a built-in observer: it only spools the event and
	// wakes its sender, so it costs nothing of the handler budget.
	feed.ObserveLifecycle(event, payload)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, u := range userExecutables(path) {
		wg.Add(1)
		go func(u userExecutable) {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					lifecycleWarn(log, event, fmt.Sprintf("handler %s panicked: %v; continuing", u.name, p))
				}
			}()
			if err := runLifecycleHandler(ctx, u.exe, event, payload); err != nil {
				lifecycleWarn(log, event, fmt.Sprintf("handler %s failed on %s: %v; continuing", u.name, event, err))
			}
		}(u)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		// Each handler's own context kills its process group; give that a
		// moment to land so nothing outlives the budget.
		select {
		case <-done:
		case <-time.After(ReapGrace):
		}
	}
}

func runLifecycleHandler(ctx context.Context, exe, event string, payload []byte) error {
	c := exec.CommandContext(ctx, exe, event)
	c.Stdin = bytes.NewReader(payload)
	var errb bytes.Buffer
	c.Stderr = &errb
	c.WaitDelay = 100 * time.Millisecond
	ownGroup(c)
	c.Env = append(os.Environ(), "HEV_HOOK_EVENT="+event, "HEV_HOOK_HARNESS=kit")
	if err := c.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out")
		}
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(firstLines(errb.String(), 3)))
	}
	return nil
}

func lifecycleWarn(l Log, event, msg string) {
	_ = l.Append("warnings.log", map[string]any{"time": time.Now().UTC(), "event": event, "harness": "kit", "warning": msg})
}

// DefaultLog is ~/.hev/hooks, where hev hook keeps its event and warning logs.
func DefaultLog() Log {
	home, err := os.UserHomeDir()
	if err != nil {
		return Log{}
	}
	return Log{Dir: filepath.Join(home, ".hev", "hooks")}
}
