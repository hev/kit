package foh

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

var secretPatterns = []*regexp.Regexp{
	// Authorization headers and bearer tokens, whatever the token shape.
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s"',]+`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`),
	// OpenRouter, OpenAI and Anthropic style keys.
	regexp.MustCompile(`\bsk-(?:or-v1-|ant-|proj-)?[A-Za-z0-9_-]{16,}`),
	// key=value pairs that name a key or token.
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|access[_-]?token|secret)\s*[=:]\s*"?)[^\s"',&]{8,}`),
}

// Scrub removes the given secret values and anything shaped like an API key
// or bearer token from s.
func Scrub(s string, secrets ...string) string {
	for _, v := range secrets {
		if len(v) >= 4 {
			s = strings.ReplaceAll(s, v, redacted)
		}
	}
	for i, re := range secretPatterns {
		switch i {
		case 0:
			s = re.ReplaceAllString(s, "${1}"+redacted)
		case 3:
			s = re.ReplaceAllString(s, "${1}"+redacted)
		default:
			s = re.ReplaceAllString(s, redacted)
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
