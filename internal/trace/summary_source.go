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
	text = strings.TrimSpace(text)
	lower = strings.ToLower(text)
	for _, prefix := range []string{"filesystem sandboxing", "# when to ask the user for permission", "<permissions instructions>", "sandbox_mode:", "approval_policy:", "<permissions", "# permissions", "you are a coding agent", "you are codex,"} {
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
			if text := RequestText(b.Text); text != "" {
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

// PreambleSummary rejects meta descriptions of absent task evidence, while
// allowing actual tasks such as repairing AGENTS.md or sandbox permissions.
func PreambleSummary(summary string) bool {
	s := strings.ToLower(summary)
	context := false
	for _, term := range []string{"preamble", "harness context", "injected context", "coordination", "instructions", "environment context", "environment_context", "agents.md", "claude.md", "system-reminder", "sandbox", "pasted_content"} {
		context = context || strings.Contains(s, term)
	}
	if !context {
		return false
	}
	for _, term := range []string{"only", "no task", "no request", "no actionable", "no concrete", "no actual", "no specific", "just ", "consists of", "contains instructions", "provides instructions", "provided instructions", "instruction preamble"} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
