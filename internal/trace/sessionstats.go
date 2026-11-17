package trace

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// CaptureVersion marks session rows that carry the capture facts hev traces
// stats reads: start_context, tool_results, tool_errors, tools_before_edit,
// command_counts and skill_loads. A row without it predates them; its zero
// values mean "not captured", not "none".
const CaptureVersion = 1

// Command counts recorded per session, by key. The patterns match the
// command text of a shell tool call (Bash, exec_command, Codex's exec
// scripts, PI's bash): kit commands an agent runs to search or record
// shared knowledge and to dispatch work.
var commandPatterns = []struct {
	key string
	re  *regexp.Regexp
}{
	{"board_search", regexp.MustCompile(`\bhev\s+board\s+search\b`)},
	{"board_post", regexp.MustCompile(`\bhev\s+board\s+(?:post|reply)\b`)},
	{"traces_search", regexp.MustCompile(`\bhev\s+traces\s+search\b`)},
	{"dispatch", regexp.MustCompile(`\bhev\s+dispatch\b`)},
}

// CommandKeys lists the command_counts keys in display order.
func CommandKeys() []string {
	keys := make([]string, len(commandPatterns))
	for i, p := range commandPatterns {
		keys[i] = p.key
	}
	return keys
}

var gitCommit = regexp.MustCompile(`\bgit\s+(?:-[Cc]\s+\S+\s+)*commit\b`)

// editTool reports whether a tool call changes files: an edit or write
// tool, a patch, or a shell command that applies a patch or commits.
func editTool(name, text string) bool {
	switch strings.TrimPrefix(strings.ToLower(name), "functions.") {
	case "edit", "write", "multiedit", "notebookedit", "apply_patch", "str_replace_editor", "str_replace_based_edit_tool", "create_file", "write_file", "edit_file", "patch":
		return true
	}
	if anyShellTool(name) || isCommandTool(name) {
		return strings.Contains(text, "apply_patch") || gitCommit.MatchString(text)
	}
	return false
}

// anyShellTool reports whether a tool runs shell commands under any
// harness's naming.
func anyShellTool(name string) bool {
	switch n := bareTool(name); n {
	case "bash", "shell", "shell_command", "exec", "exec_command", "local_shell", "unified_exec":
		return true
	}
	return false
}

// isCommandTool covers the shell-like tools anyShellTool does not: Codex's
// exec script wrapper and custom tools carrying a command.
func isCommandTool(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, "__exec_command") || n == "js" || n == "script"
}

// sessionToolFacts derives the per-session tool facts from turns in order.
func sessionToolFacts(turns []Turn, s *SessionRow) {
	s.CaptureVersion = CaptureVersion
	s.ToolsBeforeEdit = -1
	s.CommandCounts = ToolCounts{}
	resulted := map[string]bool{}
	failed := map[string]bool{}
	var calls int64
	firstLoad := map[string]bool{}
	for _, t := range turns {
		if t.StartContext != nil && s.StartContext == nil {
			c := *t.StartContext
			s.StartContext = &c
		}
		for _, b := range t.Blocks {
			for _, l := range b.SkillLoads {
				if !firstLoad[l.Name] && len(s.SkillLoads) < 64 {
					firstLoad[l.Name] = true
					s.SkillLoads = append(s.SkillLoads, SkillLoad{Name: l.Name, Via: l.Via, Seq: l.Seq, TurnUUID: l.TurnUUID, TS: l.TS})
				}
			}
			switch b.Type {
			case "tool_use":
				if s.ToolsBeforeEdit < 0 && editTool(b.ToolName, b.Text) {
					s.ToolsBeforeEdit = calls
				}
				calls++
				if anyShellTool(b.ToolName) || isCommandTool(b.ToolName) || strings.Contains(strings.ToLower(b.ToolName), "exec") {
					for _, p := range commandPatterns {
						if n := len(p.re.FindAllStringIndex(b.Text, -1)); n > 0 {
							s.CommandCounts[p.key] += n
						}
					}
				}
			case "tool_result":
				id := b.ToolUseID
				if id == "" {
					s.ToolResults++
					if b.IsError {
						s.ToolErrors++
					}
					continue
				}
				if !resulted[id] {
					resulted[id] = true
					s.ToolResults++
				}
				if b.IsError && !failed[id] {
					failed[id] = true
					s.ToolErrors++
				}
			}
		}
	}
	s.StartContextBytes = s.StartContext.Bytes()
}

// TurnsFromBlocks rebuilds turns from a session's whole-block rows, in seq
// order: the lossless read side, which unlike search chunks keeps tool
// results, per-request usage and tool outcomes. meta supplies what block
// rows do not carry (session, harness, working directory, branch, source);
// a tool_use block carries its result's outcome (IsError, ExitCode,
// ResultBytes) when one was seen.
func TurnsFromBlocks(rows []BlockRow, meta Turn) []Turn {
	sorted := append([]BlockRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })
	var turns []Turn
	index := map[string]int{}
	for _, r := range sorted {
		i, ok := index[r.TurnUUID]
		if !ok || i != len(turns)-1 {
			t := meta
			t.Blocks = nil
			t.StartContext = nil
			t.SessionID = r.SessionID
			t.TurnUUID = r.TurnUUID
			t.Seq = int64(len(turns))
			t.Role = r.Role
			t.TS = time.UnixMilli(r.End).UTC().Format(time.RFC3339Nano)
			if r.End == 0 {
				t.TS = ""
			}
			t.AgentID, t.ParentAgentID = r.AgentID, r.ParentAgentID
			t.Model, t.Effort, t.RequestID = r.Model, r.Effort, r.RequestID
			t.Usage = Usage{Input: r.InputTokens, Output: r.OutputTokens, CacheRead: r.CacheReadTokens, CacheCreation: r.CacheCreationTokens}
			t.Cost = r.Cost
			turns = append(turns, t)
			i = len(turns) - 1
			index[r.TurnUUID] = i
		}
		b := Block{Type: r.BlockType, Text: r.Text, ToolName: r.ToolName, ToolUseID: r.ToolUseID, SkillLoads: r.SkillLoads, Images: r.Images, RequestText: r.RequestText, RequestSelected: r.RequestSelected, ResultBytes: r.ResultBytes}
		if (r.BlockType == "tool_use" && r.Resulted) || r.BlockType == "tool_result" {
			b.IsError = !r.OK
		}
		if r.ExitCode != nil {
			code := int(*r.ExitCode)
			b.ExitCode = &code
		}
		turns[i].Blocks = append(turns[i].Blocks, b)
	}
	// Rows captured before tool outcomes were recorded still hold the
	// result text: read the outcome from it, and give each call its
	// result's outcome.
	CaptureToolOutcomes(turns)
	results := map[string]Block{}
	for _, t := range turns {
		for _, b := range t.Blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				if _, ok := results[b.ToolUseID]; !ok {
					results[b.ToolUseID] = b
				}
			}
		}
	}
	for ti := range turns {
		for bi := range turns[ti].Blocks {
			b := &turns[ti].Blocks[bi]
			if r, ok := results[b.ToolUseID]; ok && b.Type == "tool_use" {
				b.IsError, b.ExitCode, b.ResultBytes = r.IsError, r.ExitCode, r.ResultBytes
			}
		}
	}
	return turns
}

// SessionFacts derives the capture facts of one session from its turns:
// tool outcomes are re-read from result text and skill loads re-detected,
// so turns rebuilt from rows captured before capture version 1 yield the
// same facts a fresh capture would (less the starting context, which only
// the transcript holds).
func SessionFacts(turns []Turn) SessionRow {
	turns = CaptureSkills(turns)
	var s SessionRow
	s.Skills = skillNames(turns)
	sessionToolFacts(turns, &s)
	for _, t := range turns {
		for _, b := range t.Blocks {
			if b.Type == "tool_use" {
				s.ToolCount++
			}
		}
	}
	return s
}
