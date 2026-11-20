package harness

import (
	"regexp"
	"strings"
)

var geminiANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var geminiApproval = regexp.MustCompile(`^\s*(?:[❯›>]\s*)?[0-9]+\.\s+(?:Allow once|Allow for this session|Always allow|No, suggest changes)\b`)

// GeminiScreenState reads the visible tool confirmation, loading footer and composer.
func GeminiScreenState(screen string) string {
	lines := strings.Split(strings.TrimRight(geminiANSI.ReplaceAllString(screen, ""), "\n"), "\n")
	lines = lines[max(0, len(lines)-20):]
	for _, line := range lines {
		if geminiApproval.MatchString(line) {
			return "prompt"
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "(esc to cancel") {
			return "working"
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		line = strings.TrimSpace(strings.TrimLeft(line, "│┃"))
		if strings.HasPrefix(line, ">") || strings.HasPrefix(line, "* ") {
			text := strings.TrimSpace(strings.Trim(strings.TrimLeft(line, ">*"), " │┃"))
			if text != "" && !strings.HasPrefix(text, "Type your message or ") {
				return "draft"
			}
			return "idle"
		}
	}
	return "?"
}
