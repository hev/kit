package trace

import (
	"regexp"
	"sort"
	"strings"
)

const InjectedSummaryReason = "Only injected harness context in archived prompts"

var contextBlock = regexp.MustCompile(`(?is)<(INSTRUCTIONS|environment_context|system-reminder|permissions instructions|sandbox_context)(?:\s[^>]*)?>.*?</(?:INSTRUCTIONS|environment_context|system-reminder|permissions instructions|sandbox_context)>`)
var instructionHeader = regexp.MustCompile(`(?im)^\s*#+\s*(?:AGENTS|CLAUDE)\.md instructions[^\n]*\n?`)
var pastedTag = regexp.MustCompile(`(?i)</?pasted_content\b[^>]*>`)

// Permission metadata is line-oriented; retain a request following the block.
var permissionLine = regexp.MustCompile(`(?i)^\s*(?:filesystem sandboxing\b.*|(?:-\s*)?\x60?(?:sandbox_mode|approval_policy|network_access|network access|sandbox permissions|approval policy)\x60?\s*(?:[:=]|is\b).*|network access is\b.*|approval policy is\b.*)$`)
var peerHeader = regexp.MustCompile(`(?m)^\[peer (?:message from|chat #)[^\n]*\]\s*\n?`)

// RequestText removes known harness framing without discarding the payload of
// peer messages or pasted requests. An unclosed context block is not evidence.
func RequestText(text string) string {
	// A bare instruction-file preamble has no reliable payload boundary.
	if instructionHeader.MatchString(text) && !strings.Contains(strings.ToLower(text), "</instructions>") {
		return ""
	}
	text = contextBlock.ReplaceAllString(text, "")
	text = instructionHeader.ReplaceAllString(text, "")
	text = peerHeader.ReplaceAllString(text, "")
	text = pastedTag.ReplaceAllString(text, "")
	lower := strings.ToLower(text)
	for _, tag := range []string{"<instructions", "<environment_context", "<system-reminder", "<permissions instructions", "<sandbox_context"} {
		if i := strings.Index(lower, tag); i >= 0 {
			text = text[:i]
			lower = strings.ToLower(text)
		}
	}
	lines := strings.Split(text, "\n")
	for len(lines) > 0 {
		if strings.TrimSpace(lines[0]) != "" && !permissionLine.MatchString(lines[0]) {
			break
		}
		lines = lines[1:]
	}
	text = strings.TrimSpace(strings.Join(lines, "\n"))
	lower = strings.ToLower(text)
	for _, prefix := range []string{"# when to ask the user for permission", "<permissions instructions>", "<permissions", "# permissions", "you are a coding agent", "you are codex,"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	return text
}

// SummarySource prefers every user request over assistant/tool evidence. Blocks
// are copied before sorting so callers retain their original archive ordering.
func SummarySource(first string, blocks []BlockRow) string {
	if text := RequestText(first); text != "" {
		return text
	}
	blocks = append([]BlockRow(nil), blocks...)
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Seq < blocks[j].Seq })
	for _, b := range blocks {
		if b.Role == "user" && b.BlockType == "text" {
			text := RequestText(b.Text)
			if b.RequestSelected {
				text = b.RequestText
			}
			if text != "" {
				return text
			}
		}
	}
	for _, b := range blocks {
		if (b.Role == "assistant" && b.BlockType == "text") || b.BlockType == "tool_use" || b.BlockType == "tool_result" {
			text := RequestText(b.Text)
			if text == "" && b.BlockType == "tool_use" {
				text = b.ToolName
			}
			if text != "" {
				r := []rune(text)
				if len(r) > 1000 {
					text = string(r[:1000])
				}
				return text
			}
		}
	}
	return ""
}

// PreambleSummary enforces the NO_TASK contract and a minimal sanity check.
// Noun-phrase descriptions of setup are not summaries of work. No request-word
// overlap is used: incidental project labels cannot validate an answer.
func PreambleSummary(summary, _ string) bool {
	s := strings.ToLower(strings.TrimSpace(summary))
	if s == "no_task" || strings.HasPrefix(s, "no_task") {
		return true
	}
	if len(strings.Fields(s)) < 2 {
		return true
	}
	setup := regexp.MustCompile(`(?i)(coordination instructions|environment setup|environment configuration|sandbox permissions|harness context|preamble|instructions only|only.*instructions)`)
	action := regexp.MustCompile(`(?i)\b(fix|fixed|repair|repaired|implement|implemented|add|added|update|updated|debug|debugged|test|tested|investigate|investigated|improve|improved|build|built|remove|removed)\b`)
	return setup.MatchString(s) && !action.MatchString(s)
}

// selectParsedRequests preserves raw transcript blocks while recording tagged
// user-item boundaries before projection. Empty selection means injected-only.
func selectParsedRequests(turns []Turn) []Turn {
	for i := range turns {
		if turns[i].Role != "user" {
			continue
		}
		for j := range turns[i].Blocks {
			b := &turns[i].Blocks[j]
			if b.Type != "text" || b.RequestSelected {
				continue
			}
			selected := RequestText(b.Text)
			b.RequestText = selected
			b.RequestSelected = true
		}
	}
	return turns
}
