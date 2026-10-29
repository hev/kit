package trace

import (
	"encoding/json"
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

// NoTaskStatement recognizes explicit absence of a task, independent of punctuation.
var noTaskStatement = regexp.MustCompile(`\b(no user task|no task provided|no request|no concrete task|nothing to do|no instructions from the user|no task|no actual request)\b`)

func noTaskSummary(summary string) bool {
	s := strings.ToLower(strings.TrimSpace(summary))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return ' '
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	return s == "no task" || noTaskStatement.MatchString(s)
}

// Setup vocabulary is shared by every summary writer. Word stems allow inflected
// forms without matching substrings inside unrelated words.
var setupVocabulary = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bagents\.md\b`),
	regexp.MustCompile(`(?i)\bclaude\.md\b`),
	regexp.MustCompile(`(?i)\binstruct\w*\b`),
	regexp.MustCompile(`(?i)\bsystem[\s-]+prompt\w*\b`),
	regexp.MustCompile(`(?i)\bpreambl\w*\b`),
	regexp.MustCompile(`(?i)\bsandbox\w*\b`),
	regexp.MustCompile(`(?i)\bpermission\w*\b`),
	regexp.MustCompile(`(?i)\bapproval[\s-]+polic\w*\b`),
	regexp.MustCompile(`(?i)\benvironment\w*\b`),
	regexp.MustCompile(`(?i)\benvironment[\s-]+context\w*\b`),
	regexp.MustCompile(`(?i)\bcoordinat\w*\b`),
	regexp.MustCompile(`(?i)\bagent[\s-]+coordinat\w*\b`),
	regexp.MustCompile(`(?i)\bpeer[\s-]+messag\w*\b`),
	regexp.MustCompile(`(?i)\bconfig\w*\b`),
	regexp.MustCompile(`(?i)\bsetup\w*\b`),
	regexp.MustCompile(`(?i)\binject\w*\b`),
	regexp.MustCompile(`(?i)\bharness\w*\b`),
	regexp.MustCompile(`(?i)\bharness[\s-]+context\w*\b`),
	regexp.MustCompile(`(?i)\barchived[\s-]+prompt\w*\b`),
	regexp.MustCompile(`(?i)\bpreamble[\s-]+only\b`),
}

// PreambleSummary rejects explicit no-task statements and setup vocabulary not
// grounded in the selected request. It does not classify arbitrary paraphrases.
func PreambleSummary(summary, request string) bool {
	if noTaskSummary(summary) {
		return true
	}
	for _, term := range setupVocabulary {
		if term.MatchString(summary) && !term.MatchString(request) {
			return true
		}
	}
	return false
}

// ParseSummaryAnswer accepts only the structured model contract. Plain text,
// including the legacy NO_TASK sentinel, is rejected and remains retryable.
func ParseSummaryAnswer(answer string) (string, bool) {
	var output struct {
		TaskPresent *bool   `json:"task_present"`
		Summary     *string `json:"summary"`
	}
	if json.Unmarshal([]byte(answer), &output) != nil || output.TaskPresent == nil || !*output.TaskPresent || output.Summary == nil {
		return "", false
	}
	summary := strings.Join(strings.Fields(*output.Summary), " ")
	if len(strings.Fields(summary)) < 2 || noTaskSummary(summary) {
		return "", false
	}
	return summary, true
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
