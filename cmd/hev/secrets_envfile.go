package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hev/kit/internal/secrets"
)

// envLine is one NAME=VALUE from an env file.
type envLine struct {
	name, value string
}

// maxEnvFile bounds an env file; it is a list of names and references.
const maxEnvFile = 1 << 20

// refSchemes are the value prefixes an env file treats as references; any
// other value passes through unchanged.
var refSchemes = []string{"secret://", "op://", "azkv://"}

func isReference(v string) bool {
	for _, s := range refSchemes {
		if strings.HasPrefix(v, s) {
			return true
		}
	}
	return false
}

// readEnvFile parses dotenv lines: NAME=VALUE with an optional "export "
// prefix, blank lines and # comments ignored, values unquoted, 'single
// quoted' (literal) or "double quoted" (\n, \t, \r, \", \\ and \$ escapes).
// An unquoted value ends at whitespace followed by #. A quoted value cannot
// span lines. A later line for the same NAME replaces an earlier one, as in a
// shell that sources the file. Errors name the line number only, never its
// content, which may be a value.
func readEnvFile(path string) ([]envLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("--env-file: cannot open %s", path)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxEnvFile+1))
	if err != nil {
		return nil, fmt.Errorf("--env-file: cannot read %s", path)
	}
	if len(b) > maxEnvFile {
		return nil, fmt.Errorf("--env-file: %s is larger than 1 MiB", path)
	}
	var out []envLine
	at := map[string]int{}
	for i, raw := range strings.Split(string(b), "\n") {
		n := i + 1
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !envName(name) {
			return nil, fmt.Errorf("--env-file %s line %d: want NAME=VALUE with NAME made of letters, digits and _", path, n)
		}
		v, err := envValue(strings.TrimLeft(value, " \t"))
		if err != nil {
			return nil, fmt.Errorf("--env-file %s line %d: %v", path, n, err)
		}
		if j, dup := at[name]; dup {
			out[j].value = v
			continue
		}
		at[name] = len(out)
		out = append(out, envLine{name, v})
	}
	return out, nil
}

func envName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return false
	}
	for _, c := range s {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// envValue unquotes one value; what follows a closing quote may only be
// whitespace or a # comment.
func envValue(s string) (string, error) {
	tail := func(rest string) error {
		rest = strings.TrimSpace(rest)
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return fmt.Errorf("unexpected text after the closing quote")
		}
		return nil
	}
	switch {
	case strings.HasPrefix(s, "'"):
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", fmt.Errorf("unterminated single quote (values cannot span lines)")
		}
		return s[1 : 1+end], tail(s[2+end:])
	case strings.HasPrefix(s, `"`):
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			c := s[i]
			if c == '"' {
				return b.String(), tail(s[i+1:])
			}
			if c == '\\' && i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case 'r':
					b.WriteByte('\r')
				case '"', '\\', '$':
					b.WriteByte(s[i])
				default:
					b.WriteByte('\\')
					b.WriteByte(s[i])
				}
				continue
			}
			b.WriteByte(c)
		}
		return "", fmt.Errorf("unterminated double quote (values cannot span lines)")
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = s[:i]
			break
		}
	}
	return strings.TrimRight(s, " \t"), nil
}

// secretInputs combines an optional env file with --ref and --ttl flags.
// References from both are returned as refs; the file's other values are
// returned as plain, to pass through unchanged. On a duplicate NAME, --ref
// wins over the file, and within the file the later line wins. order lists
// every NAME once, file names first in file order.
func secretInputs(envFile string, refFlags, ttls []string) (rs []secrets.Ref, plain map[string]string, order []string, err error) {
	var lines []envLine
	if envFile != "" {
		if lines, err = readEnvFile(envFile); err != nil {
			return nil, nil, nil, err
		}
	}
	fromFlags := map[string]bool{}
	for _, s := range refFlags {
		k, _, _ := strings.Cut(s, "=")
		fromFlags[k] = true
	}
	plain = map[string]string{}
	var fileRefs []string
	for _, l := range lines {
		order = append(order, l.name)
		if fromFlags[l.name] {
			continue
		}
		if isReference(l.value) {
			fileRefs = append(fileRefs, l.name+"="+l.value)
		} else {
			plain[l.name] = l.value
		}
	}
	rs, err = parseSecretRefs(append(fileRefs, refFlags...), ttls)
	if err != nil {
		return nil, nil, nil, err
	}
	seen := map[string]bool{}
	for _, n := range order {
		seen[n] = true
	}
	for _, r := range rs {
		if !seen[r.Name] {
			seen[r.Name] = true
			order = append(order, r.Name)
		}
	}
	return rs, plain, order, nil
}

// defaultTTL is a reference's freshness when no --ttl names it.
const defaultTTL = 5 * time.Minute
