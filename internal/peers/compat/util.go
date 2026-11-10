package compat

import (
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hev/kit/internal/platformops"
)

// splitlines is Python's str.splitlines(). It walks the string as WTF-8 and
// slices bytes, so lone surrogates in a line come through unchanged.
func splitlines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); {
		r, size := decodeWTF8(s[i:])
		next := i + size
		switch r {
		case '\n', '\r', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', '\u2028', '\u2029':
			out = append(out, s[start:i])
			if r == '\r' && next < len(s) && s[next] == '\n' {
				next++
			}
			start = next
		}
		i = next
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

var shellUnsafe = regexp.MustCompile(`[^\w@%+=:,./-]`)

// shlexQuote is shlex.quote.
func shlexQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !shellUnsafe.MatchString(s) && isASCII(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// shlexSplit is shlex.split(s, posix=True) without comments.
func shlexSplit(s string) ([]string, bool) {
	var words []string
	var cur strings.Builder
	inWord := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case c == '\\':
			inWord = true
			i++
			if i >= len(rs) {
				return nil, false
			}
			cur.WriteRune(rs[i])
		case c == '\'':
			inWord = true
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j >= len(rs) {
				return nil, false
			}
			cur.WriteString(string(rs[i+1 : j]))
			i = j
		case c == '"':
			inWord = true
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\\\"$`\n", rs[j+1]) {
					j++
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, false
			}
			i = j
		default:
			inWord = true
			cur.WriteRune(c)
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}

// realpath is os.path.realpath (non-strict: a missing tail stays as written).
func realpath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	dir, base := filepath.Split(abs)
	if dir == abs || base == "" {
		return abs
	}
	return filepath.Join(realpath(filepath.Clean(dir)), base)
}

func readAll(r io.Reader) (string, error) {
	if r == nil {
		return "", nil
	}
	b, err := io.ReadAll(r)
	return string(b), err
}

func which(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func lockPath(path string) (func(), error) { return platformops.LockPath(path, false) }
