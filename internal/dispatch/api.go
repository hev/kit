package dispatch

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hev/kit/internal/redact"
)

// CheckAPIOrder applies the host's repository policy before an API submission
// can enter the queue. The normal start path checks again when it launches.
func CheckAPIOrder(o Order, host string) error {
	policy, err := targetPolicy()
	if err != nil {
		return err
	}
	if len(policy.Admission.Repos) == 0 {
		return fmt.Errorf("API execution requires an explicit repository allowlist")
	}
	cfg, err := loadTarget("", host)
	if err != nil {
		return err
	}
	return admitRepo(policy.Admission, resolveRepo(o.Repo, cfg.Workspace), o.Repo, host)
}

// readAPILog only releases complete records. Offset is in the original byte
// stream even when redaction expands a record. Oversize records are skipped
// in bounded steps without exposing fragments to the client.
func readAPILog(dir string, q Request, live bool) (string, int64, bool, error) {
	if q.APILogLimit < 1 || q.APILogLimit > 4<<20 || q.Round < 0 || q.Offset < 0 {
		return "", 0, false, fmt.Errorf("invalid log limits")
	}
	name := "log"
	if q.Round != 0 {
		name = fmt.Sprintf("round-%d.log", q.Round)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", 0, false, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if os.IsNotExist(err) && q.Round == 0 {
		return "", 0, live, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return "", 0, false, fmt.Errorf("log unavailable")
	}
	offset := q.Offset
	if offset > fi.Size() {
		offset = 0
	}
	start := offset
	if offset > 0 {
		var prev [1]byte
		if _, err := f.ReadAt(prev[:], offset-1); err != nil {
			return "", 0, false, err
		}
		if prev[0] != '\n' {
			start--
		}
	}
	raw := make([]byte, min(int64(q.APILogLimit), fi.Size()-offset))
	n, err := f.ReadAt(raw, offset)
	if err != nil && err != io.EOF {
		return "", 0, false, err
	}
	raw = raw[:n]
	limit := offset + int64(n)
	if start != offset {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return "", limit, live || limit < fi.Size(), nil
		}
		raw = raw[i+1:]
		offset += int64(i + 1)
	}
	var result strings.Builder
	for len(raw) > 0 {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			if result.Len() == 0 {
				// Don't expose a fragment. The next offset is checked against
				// the previous byte so it skips the continuation too.
				offset = limit
			}
			break
		}
		line := scrubAPILog(string(raw[:i+1]), dir)
		if result.Len()+len(line) > q.APILogLimit {
			if result.Len() != 0 {
				break
			}
		} else {
			result.WriteString(line)
		}
		offset += int64(i + 1)
		raw = raw[i+1:]
	}
	return result.String(), offset, live || offset < fi.Size(), nil
}

var apiLogRedactor = sync.OnceValue(func() *redact.Scrubber {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil
	}
	s, _ := redact.New(salt)
	return s
})

func scrubAPILog(line, dir string) string {
	s := apiLogRedactor()
	if s == nil {
		return "[REDACTED]\n"
	}
	out, _ := s.Text(line)
	for _, p := range []string{dir, Root(), os.Getenv("HOME")} {
		if p != "" && p != "." && p != string(filepath.Separator) {
			out = strings.ReplaceAll(out, p, "[REDACTED]")
		}
	}
	return redact.PublicTokens(out)
}
