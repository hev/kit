package peers

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)
var dialog = regexp.MustCompile(`(?i)^\s*[❯›>]\s*1\.|Enter to confirm|Press enter to continue|Do you want to (proceed|make|create|allow)|Esc to cancel|allow this|permission|trust this|yes, proceed`)
var progress = regexp.MustCompile(`^. [\p{L}]+… \([0-9]+[smh]`)

// Dim input placeholders aren't user drafts. Preserve real text across SGR resets.
func undimmed(line string) string {
	var out strings.Builder
	dim, pos := false, 0
	for _, match := range sgr.FindAllStringSubmatchIndex(line, -1) {
		if !dim {
			out.WriteString(line[pos:match[0]])
		}
		codes := strings.FieldsFunc(line[match[2]:match[3]], func(r rune) bool { return r == ';' || r == ':' })
		if len(codes) == 0 {
			codes = []string{"0"}
		}
		for _, c := range codes {
			if c == "2" {
				dim = true
			}
			if c == "0" || c == "22" {
				dim = false
			}
		}
		pos = match[1]
	}
	if !dim {
		out.WriteString(line[pos:])
	}
	return ansi.ReplaceAllString(out.String(), "")
}

func (e *Engine) paneState(ctx context.Context, pane, agent string, headless bool) string {
	if agent == "" {
		return "-"
	}
	if headless {
		return "working"
	}
	if agent != "claude" && agent != "codex" {
		return "?"
	}
	screen, err := e.tmux(ctx, "capture-pane", "-e", "-p", "-t", pane)
	if err != nil {
		return "?"
	}
	return screenState(screen)
}

func screenState(screen string) string {
	raw, plain := []string{}, []string{}
	for _, line := range strings.Split(screen, "\n") {
		clean := strings.ReplaceAll(ansi.ReplaceAllString(line, ""), "\u00a0", " ")
		if strings.TrimSpace(clean) != "" {
			raw = append(raw, line)
			plain = append(plain, clean)
		}
	}
	start := len(plain) - 12
	if start < 0 {
		start = 0
	}
	for _, line := range plain[start:] {
		if dialog.MatchString(line) {
			return "prompt"
		}
	}
	for i := len(plain) - 1; i >= 0 && i >= len(plain)-20; i-- {
		s := strings.TrimSpace(plain[i])
		if strings.HasPrefix(s, "❯") || strings.HasPrefix(s, "›") {
			typed := strings.TrimSpace(strings.ReplaceAll(undimmed(raw[i]), "\u00a0", " "))
			typed = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(typed, "❯"), "›"))
			if typed != "" {
				return "draft"
			}
			break
		}
	}
	for _, line := range plain[start:] {
		if strings.Contains(strings.ToLower(line), "esc to interrupt") {
			return "working"
		}
		s := strings.TrimLeftFunc(line, unicode.IsSpace)
		if progress.MatchString(s) {
			return "working"
		}
	}
	return "idle"
}
