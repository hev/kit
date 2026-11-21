package services

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
)

const (
	LogLimit      = 1 << 20   // 1 MiB cap for service.log
	compactTarget = 768 << 10 // Compact down to 768 KiB when crossing 1 MiB
	maxLineBytes  = 64 << 10  // Omit single lines exceeding 64 KiB without a newline
)

var outputScrubber = sync.OnceValues(func() (*redact.Scrubber, error) {
	var salt [32]byte
	_, _ = rand.Read(salt[:])
	return redact.New(salt[:])
})

func newTextScrubber(values map[string]string, paths ...string) (func(string) string, *strings.Replacer, int) {
	var terms []string
	var multiTerms []string
	maxMultiLen := 0
	seen := make(map[string]bool)
	addTerm := func(v string, multiline bool) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		terms = append(terms, v)
		if multiline {
			multiTerms = append(multiTerms, v)
			if len(v) > maxMultiLen {
				maxMultiLen = len(v)
			}
		}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		hasNL := strings.Contains(v, "\n")
		addTerm(v, hasNL)
		if hasNL {
			if crlf := strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\n", "\r\n"); crlf != v {
				addTerm(crlf, true)
			}
		}
	}
	for _, p := range paths {
		if filepath.IsAbs(p) && filepath.Dir(p) != p {
			addTerm(p, false)
			if slash := filepath.ToSlash(p); slash != p {
				addTerm(slash, false)
			}
		}
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	pairs := make([]string, 0, len(terms)*2)
	for _, term := range terms {
		pairs = append(pairs, term, "[REDACTED]")
	}
	replacer := strings.NewReplacer(pairs...)

	var multiReplacer *strings.Replacer
	if len(multiTerms) > 0 {
		sort.Slice(multiTerms, func(i, j int) bool { return len(multiTerms[i]) > len(multiTerms[j]) })
		mp := make([]string, 0, len(multiTerms)*2)
		for _, term := range multiTerms {
			mp = append(mp, term, "[REDACTED]")
		}
		multiReplacer = strings.NewReplacer(mp...)
	}

	scrub := func(raw string) string {
		if raw == "" {
			return ""
		}
		scrubber, err := outputScrubber()
		if err != nil || scrubber == nil {
			return ""
		}
		text, _ := scrubber.Text(replacer.Replace(raw))
		return strings.ToValidUTF8(text, "\uFFFD")
	}
	return scrub, multiReplacer, maxMultiLen
}

func scrubReason(reason string, paths ...string) string {
	if reason == "" {
		return ""
	}
	scrub, _, _ := newTextScrubber(nil, paths...)
	return strings.TrimSpace(scrub(reason))
}

type ringLog struct {
	mu          sync.Mutex
	path        string
	buf         []byte
	loaded      bool
	multiline   *strings.Replacer
	maxMultiLen int
}

func newRingLog(path string, multiline *strings.Replacer, maxMultiLen int) *ringLog {
	return &ringLog{path: path, multiline: multiline, maxMultiLen: maxMultiLen}
}

func (r *ringLog) ensureLoadedLocked() error {
	if r.loaded {
		return nil
	}
	if err := platformops.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}
	f, err := os.Open(r.path)
	if os.IsNotExist(err) {
		r.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, LogLimit+1))
	_ = f.Close()
	if readErr != nil {
		return readErr
	}
	r.loaded = true
	if len(data) > LogLimit {
		r.buf = compactBytes(data, compactTarget)
		return platformops.WriteFileAtomic(r.path, r.buf, 0600)
	}
	r.buf = data
	return nil
}

func compactBytes(data []byte, target int) []byte {
	if len(data) <= LogLimit && target >= len(data) {
		return data
	}
	if target > LogLimit {
		target = LogLimit
	}
	if len(data) <= target {
		return data
	}
	tail := data[len(data)-target:]
	if nl := bytes.IndexByte(tail, '\n'); nl >= 0 && nl+1 < len(tail) {
		tail = tail[nl+1:]
	}
	out := make([]byte, len(tail))
	copy(out, tail)
	return out
}

func (r *ringLog) appendScrubbed(clean string) {
	if clean == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureLoadedLocked(); err != nil {
		return
	}
	chunk := []byte(clean)
	if len(chunk) > LogLimit {
		chunk = compactBytes(chunk, LogLimit)
	}
	prevLen := len(r.buf)
	r.buf = append(r.buf, chunk...)

	rewrotePrior := false
	if r.multiline != nil && r.maxMultiLen > 0 {
		window := len(chunk) + r.maxMultiLen
		if window > len(r.buf) {
			window = len(r.buf)
		}
		start := len(r.buf) - window
		before := string(r.buf[start:])
		after := r.multiline.Replace(before)
		if after != before {
			rewrotePrior = true
			r.buf = append(r.buf[:start], []byte(after)...)
		}
	}

	if len(r.buf) > LogLimit {
		target := compactTarget
		if len(chunk) > target {
			target = LogLimit
		}
		r.buf = compactBytes(r.buf, target)
		_ = platformops.WriteFileAtomic(r.path, r.buf, 0600)
		return
	}
	if rewrotePrior || prevLen > LogLimit {
		_ = platformops.WriteFileAtomic(r.path, r.buf, 0600)
		return
	}
	f, err := platformops.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	_, _ = f.Write(chunk)
	_ = f.Close()
}

// streamWriter buffers each stream (stdout/stderr) independently up to newline
// boundaries so concurrent writes cannot interleave inside a secret token.
type streamWriter struct {
	mu       sync.Mutex
	ring     *ringLog
	scrub    func(string) string
	pending  []byte
	dropping bool
}

func (w *streamWriter) Write(p []byte) (int, error) {
	total := len(p)
	w.mu.Lock()
	defer w.mu.Unlock()

	for len(p) > 0 {
		if w.dropping {
			nl := bytes.IndexByte(p, '\n')
			if nl < 0 {
				return total, nil
			}
			w.dropping = false
			w.ring.appendScrubbed("[log line omitted: exceeded 64 KiB]\n")
			p = p[nl+1:]
			continue
		}

		if len(w.pending) == 0 && len(p) <= maxLineBytes {
			if cut := bytes.LastIndexByte(p, '\n'); cut >= 0 {
				block := string(p[:cut+1])
				clean := w.scrub(block)
				if clean != "" && !strings.HasSuffix(clean, "\n") {
					clean += "\n"
				}
				w.ring.appendScrubbed(clean)
				p = p[cut+1:]
				continue
			}
		}

		nl := bytes.IndexByte(p, '\n')
		if nl < 0 {
			if len(w.pending)+len(p) > maxLineBytes {
				w.pending = w.pending[:0]
				w.dropping = true
				return total, nil
			}
			w.pending = append(w.pending, p...)
			return total, nil
		}

		if len(w.pending)+nl+1 > maxLineBytes {
			w.pending = w.pending[:0]
			w.ring.appendScrubbed("[log line omitted: exceeded 64 KiB]\n")
			p = p[nl+1:]
			continue
		}

		w.pending = append(w.pending, p[:nl+1]...)
		clean := w.scrub(string(w.pending))
		w.pending = w.pending[:0]
		if clean != "" && !strings.HasSuffix(clean, "\n") {
			clean += "\n"
		}
		w.ring.appendScrubbed(clean)
		p = p[nl+1:]
	}
	return total, nil
}

func (w *streamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dropping {
		w.dropping = false
		w.ring.appendScrubbed("[log line omitted: exceeded 64 KiB]\n")
		return
	}
	if len(w.pending) > 0 {
		clean := w.scrub(string(w.pending))
		w.pending = w.pending[:0]
		if clean != "" {
			if !strings.HasSuffix(clean, "\n") {
				clean += "\n"
			}
			w.ring.appendScrubbed(clean)
		}
	}
}

func tailLinesBytes(data []byte, lines int) []byte {
	if lines <= 0 || len(data) == 0 {
		return data
	}
	end := len(data)
	if data[end-1] == '\n' {
		end--
	}
	seen := 0
	for i := end - 1; i >= 0; i-- {
		if data[i] == '\n' {
			seen++
			if seen == lines {
				return data[i+1:]
			}
		}
	}
	return data
}

func readServiceLogs(dir string, lines int, paths ...string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "service.log"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", errStorage
	}
	data, readErr := io.ReadAll(io.LimitReader(f, LogLimit+1))
	_ = f.Close()
	if readErr != nil {
		return "", errStorage
	}
	if len(data) > LogLimit {
		data = compactBytes(data, LogLimit)
	}
	if len(data) == 0 {
		return "", nil
	}
	if lines > 0 {
		data = tailLinesBytes(data, lines)
	}
	scrub, _, _ := newTextScrubber(nil, paths...)
	clean := scrub(string(data))
	if lines <= 0 || clean == "" {
		return clean, nil
	}
	trimmed := strings.TrimSuffix(clean, "\n")
	parts := strings.Split(trimmed, "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n") + "\n", nil
}
