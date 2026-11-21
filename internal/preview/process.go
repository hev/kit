package preview

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/redact"
)

const outputLimit = 64 << 10

type boundedCapture struct {
	mu       sync.Mutex
	streams  [2]bytes.Buffer
	size     int
	overflow bool
}

type captureWriter struct {
	c     *boundedCapture
	index int
}

func (w captureWriter) Write(p []byte) (int, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	if len(p) > outputLimit-w.c.size {
		w.c.overflow = true
	}
	if !w.c.overflow {
		_, _ = w.c.streams[w.index].Write(p)
		w.c.size += len(p)
	}
	return len(p), nil
}

var outputScrubber = sync.OnceValues(func() (*redact.Scrubber, error) {
	var salt [32]byte
	_, _ = rand.Read(salt[:])
	return redact.New(salt[:])
})

// scrubText removes secrets via kit's gitleaks scrubber and strips local paths
// and raw credentials before text is returned in errors.
func scrubText(text string, redactTerms ...string) string {
	if text == "" {
		return ""
	}
	scrubber, err := outputScrubber()
	if err != nil {
		return "[output omitted]"
	}
	terms := make([]string, 0, len(redactTerms)*2)
	seen := make(map[string]bool, len(redactTerms)*2)
	addTerm := func(t string) {
		if t == "" || len(t) < 2 || seen[t] {
			return
		}
		seen[t] = true
		terms = append(terms, t)
	}
	for _, raw := range redactTerms {
		if raw == "" {
			continue
		}
		if filepath.IsAbs(raw) && filepath.Dir(raw) != raw {
			addTerm(raw)
			addTerm(filepath.ToSlash(raw))
		} else if strings.Contains(raw, "://") && strings.Contains(raw, "@") {
			addTerm(raw)
		}
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	if len(terms) > 0 {
		pairs := make([]string, 0, len(terms)*2)
		for _, t := range terms {
			pairs = append(pairs, t, "[REDACTED]")
		}
		text = strings.NewReplacer(pairs...).Replace(text)
	}
	clean, _ := scrubber.Text(text)
	clean = strings.ToValidUTF8(clean, "\uFFFD")
	if len(clean) > outputLimit {
		return "[output omitted]"
	}
	return strings.TrimSpace(clean)
}

func (c *boundedCapture) scrub(redactTerms ...string) string {
	if c.overflow {
		return "[output omitted: exceeded limit]"
	}
	var parts []string
	for i := range c.streams {
		if s := scrubText(c.streams[i].String(), redactTerms...); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

func runCommandTree(ctx context.Context, dir string, env []string, redactTerms []string, name string, args ...string) (stdout string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var outBuf bytes.Buffer
	cap := &boundedCapture{}
	cmd.Stdout = &multiLimitedWriter{buf: &outBuf, limit: outputLimit, cap: captureWriter{c: cap, index: 0}}
	cmd.Stderr = captureWriter{c: cap, index: 1}
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Cancel = nil

	stop, startErr := startTree(cmd)
	if startErr != nil {
		return "", fmt.Errorf("start %s: %s", filepath.Base(name), scrubText(startErr.Error(), redactTerms...))
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		stop()
	case <-ctx.Done():
		stop()
		<-done
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s timed out", filepath.Base(name))
		}
		return "", fmt.Errorf("%s canceled", filepath.Base(name))
	}
	if err != nil {
		detail := cap.scrub(redactTerms...)
		if detail != "" {
			return "", fmt.Errorf("%s failed: %s", filepath.Base(name), detail)
		}
		return "", fmt.Errorf("%s failed: %s", filepath.Base(name), scrubText(err.Error(), redactTerms...))
	}
	return outBuf.String(), nil
}

type multiLimitedWriter struct {
	buf   *bytes.Buffer
	limit int
	cap   captureWriter
}

func (m *multiLimitedWriter) Write(p []byte) (int, error) {
	if m.buf.Len() < m.limit {
		rem := m.limit - m.buf.Len()
		if len(p) <= rem {
			_, _ = m.buf.Write(p)
		} else {
			_, _ = m.buf.Write(p[:rem])
		}
	}
	return m.cap.Write(p)
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	skip := make(map[string]bool, len(overrides))
	for k := range overrides {
		if runtime.GOOS == "windows" {
			skip[strings.ToUpper(k)] = true
		} else {
			skip[k] = true
		}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		k, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		cmp := k
		if runtime.GOOS == "windows" {
			cmp = strings.ToUpper(k)
		}
		if skip[cmp] {
			continue
		}
		out = append(out, entry)
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out
}

func gitEnv(extra ...string) []string {
	env := mergeEnv(os.Environ(), map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
	})
	return append(env, extra...)
}
