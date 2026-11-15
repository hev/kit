package dispatch

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// commandLog withholds a value-length tail so secrets split across writes
// cannot leak. Only requested values are retained; nothing is written to a
// separate spool. stdout and stderr may copy concurrently.
type commandLog struct {
	mu      sync.Mutex
	dst     io.Writer
	values  []string
	pending []byte
	width   int
}

func newCommandLog(dst io.Writer, env []string) *commandLog {
	l := &commandLog{dst: dst, width: 1}
	platform := map[string]bool{}
	for _, v := range MinimalExecEnvironment() {
		k, _, _ := strings.Cut(v, "=")
		platform[k] = true
	}
	for _, pair := range env {
		k, v, _ := strings.Cut(pair, "=")
		if v != "" && !platform[k] && k != "HEV_JOB_NAME" && k != "HEV_JOB_OCCURRENCE" {
			l.values = append(l.values, v)
			l.width = max(l.width, len(v))
		}
	}
	return l
}
func (l *commandLog) drain(final bool) error {
	var out bytes.Buffer
	for len(l.pending) >= l.width || final && len(l.pending) > 0 {
		found := ""
		for _, v := range l.values {
			if len(v) > len(found) && bytes.HasPrefix(l.pending, []byte(v)) {
				found = v
			}
		}
		if found != "" {
			out.WriteString("[REDACTED]")
			l.pending = l.pending[len(found):]
		} else {
			out.WriteByte(l.pending[0])
			l.pending = l.pending[1:]
		}
	}
	_, e := l.dst.Write(out.Bytes())
	return e
}
func (l *commandLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, p...)
	if e := l.drain(false); e != nil {
		return 0, e
	}
	return len(p), nil
}
func (l *commandLog) Flush() error { l.mu.Lock(); defer l.mu.Unlock(); return l.drain(true) }
