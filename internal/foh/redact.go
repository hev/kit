package foh

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

type secretPattern struct {
	re    *regexp.Regexp
	repl  string
	group int // when > 0, replace only capture group g and advance to its end
}

var secretPatterns = []secretPattern{
	// Authorization headers and bearer tokens, whatever the token shape.
	{re: regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s"',]+`), repl: "${1}" + redacted},
	{re: regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), repl: redacted},
	// OpenRouter, OpenAI and Anthropic style keys (including after URL percent-escapes).
	{re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_]|%[0-9A-Fa-f]{2})(sk-(?:or-v1-|ant-|proj-)?[A-Za-z0-9_-]{16,})`), repl: redacted, group: 1},
	// Unified Layer/Kit (hev_/hvl_) keys: bounded 24+ char tokens, plus exact 32-char tokens even adjacent to _.
	{re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_]|%[0-9A-Fa-f]{2})((?:hev|hvl)_[A-Za-z0-9]{24,})(?:$|[^A-Za-z0-9_]|%[0-9A-Fa-f]{2})`), repl: redacted, group: 1},
	{re: regexp.MustCompile(`(?:^|[^A-Za-z0-9]|%[0-9A-Fa-f]{2})((?:hev|hvl)_[A-Za-z0-9]{32})(?:$|[^A-Za-z0-9])`), repl: redacted, group: 1},
	// Legacy Club Kit (hk_) keys (including after URL percent-escapes).
	{re: regexp.MustCompile(`(?:^|[^A-Za-z0-9_]|%[0-9A-Fa-f]{2})(hk_[A-Za-z0-9_-]{16,})`), repl: redacted, group: 1},
	// key=value pairs (and JSON "key": "value") that name a key or token.
	{re: regexp.MustCompile(`(?i)\b((?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|secret)"?\s*[=:]\s*"?)[^\s"',&]{8,}`), repl: "${1}" + redacted},
	// The ChatGPT account header and its JSON form, whatever the value.
	{re: regexp.MustCompile(`(?i)\b((?:chatgpt[_-]account[_-]id|account[_-]id)"?\s*[=:]\s*"?)[^\s"',&]{4,}`), repl: "${1}" + redacted},
	// JWTs (OAuth access and id tokens).
	{re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), repl: redacted},
}

// replaceGroup replaces capture group g of each match with repl while advancing
// only to the end of group g so trailing delimiters remain available as leading
// boundaries for back-to-back tokens.
func replaceGroup(s string, re *regexp.Regexp, group int, repl string) string {
	var out strings.Builder
	pos := 0
	for pos < len(s) {
		m := re.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			break
		}
		gStart, gEnd := m[group*2], m[group*2+1]
		if gStart < 0 || gEnd <= gStart {
			if m[1] <= 0 {
				break
			}
			out.WriteString(s[pos : pos+m[1]])
			pos += m[1]
			continue
		}
		out.WriteString(s[pos : pos+gStart])
		out.WriteString(repl)
		pos += gEnd
	}
	if pos == 0 {
		return s
	}
	out.WriteString(s[pos:])
	return out.String()
}

// Scrub removes the given secret values and anything shaped like an API key
// or bearer token from s.
func Scrub(s string, secrets ...string) string {
	for _, v := range secrets {
		if len(v) >= 4 {
			s = strings.ReplaceAll(s, v, redacted)
		}
	}
	for _, p := range secretPatterns {
		if p.group > 0 {
			s = replaceGroup(s, p.re, p.group, p.repl)
		} else {
			s = p.re.ReplaceAllString(s, p.repl)
		}
	}
	return s
}

// scrubError wraps err so its text carries no secret values.
type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

func scrubErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	return &scrubbedError{msg: Scrub(err.Error(), secrets...), err: err}
}
