package secrets

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The read log records every real request a host makes to a secret store
// (1Password or Azure Key Vault), so a quota outage or a throttled vault can
// be attributed in minutes. A line carries time, caller, kind, reference
// labels, outcome and provider. It never carries a value, a token, a template
// or a reference URI: a label is the caller's variable name plus the first
// eight hex digits of the reference's SHA-256, which an operator can match
// offline with `printf %s op://... | sha256sum` (or the azkv:// reference).
//
// HEV_SECRETS_READ_LOG selects the file; "off" disables it. The default is
// ~/.hev/secrets/op-reads.log, mode 0600 in a 0700 directory.

const readLogMax = 4 << 20

var readLogMu sync.Mutex

func ReadLogPath() string {
	if p := os.Getenv("HEV_SECRETS_READ_LOG"); p != "" {
		if p == "off" {
			return ""
		}
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hev", "secrets", "op-reads.log")
}

var unsafeLabel = regexp.MustCompile(`[^A-Za-z0-9_.:/@+-]+`)

// Label returns the loggable label for a reference: never the URI itself.
func Label(name, uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return fmt.Sprintf("%s#%x", sanitizeField(name, 48), sum[:4])
}

func sanitizeField(s string, n int) string {
	s = unsafeLabel.ReplaceAllString(s, "_")
	s = strings.ReplaceAll(s, "://", "_") // a caller string must not smuggle a URI in
	if len(s) > n {
		s = s[:n]
	}
	if s == "" {
		s = "-"
	}
	return s
}

// LogRead appends one line for one real 1Password request. Failures to log
// never block or fail resolution.
func LogRead(now time.Time, caller, kind string, labels []string, outcome string) {
	LogReadProvider(now, "op", caller, kind, labels, outcome)
}

// LogReadProvider appends one line for one real request (a batch) to the
// named provider's store. The provider column is the reference scheme ("op",
// "azkv"); a line without it was written by an older writer and is 1Password.
func LogReadProvider(now time.Time, provider, caller, kind string, labels []string, outcome string) {
	p := ReadLogPath()
	if p == "" {
		return
	}
	readLogMu.Lock()
	defer readLogMu.Unlock()
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	if st, err := os.Stat(p); err == nil && st.Size() > readLogMax {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if len(labels) == 0 {
		labels = []string{"-"}
	}
	fmt.Fprintf(f, "%s caller=%s kind=%s refs=%d labels=%s outcome=%s provider=%s\n",
		now.UTC().Format(time.RFC3339), sanitizeField(caller, 96), sanitizeField(kind, 16),
		len(labels), strings.Join(labels, ","), sanitizeField(outcome, 24), sanitizeField(provider, 16))
}

// CountReadLog returns how many real requests the host's read log records
// since the given time. It reads only the time column.
func CountReadLog(since time.Time) (int, error) {
	p := ReadLogPath()
	if p == "" {
		return 0, os.ErrNotExist
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		stamp, _, _ := strings.Cut(sc.Text(), " ")
		if t, err := time.Parse(time.RFC3339, stamp); err == nil && !t.Before(since) {
			n++
		}
	}
	return n, sc.Err()
}

// DefaultCaller names the process asking for secrets, for the read log only.
// HEV_SECRETS_CALLER overrides it; otherwise it is the parent's command name
// (on Linux) and this process's name with the parent PID.
func DefaultCaller(self string) string {
	if c := os.Getenv("HEV_SECRETS_CALLER"); c != "" {
		return sanitizeField(c, 96)
	}
	ppid := os.Getppid()
	parent := "ppid" + strconv.Itoa(ppid)
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(ppid) + "/comm"); err == nil {
		parent = strings.TrimSpace(string(b)) + "[" + strconv.Itoa(ppid) + "]"
	}
	return sanitizeField(parent+">"+self, 96)
}
