package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// StartContextMaxInstructions bounds the instruction files recorded for one
// session, so a pathological import tree cannot grow a session row.
const StartContextMaxInstructions = 64

// InstructionRef is one instruction file a session started with: its path
// (relative to the repository or ~), size and the first 12 hex digits of
// its SHA-256. The content is never recorded.
type InstructionRef struct {
	Path   string `json:"path"`
	Kind   string `json:"kind,omitempty"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	// Source is "transcript" when the harness's transcript recorded the
	// file, "derived" when capture read it from disk afterwards.
	Source string `json:"source"`
}

// StartContext is what a session was given at start, as far as the
// transcript and the working directory show it: sizes and hashes only.
type StartContext struct {
	// Source is "transcript", "derived" or "mixed" (some of each).
	Source           string           `json:"source"`
	Instructions     []InstructionRef `json:"instructions,omitempty"`
	InstructionBytes int64            `json:"instruction_bytes"`
	// Skills is how many skill descriptions the session was offered and
	// SkillBytes their total size, from SkillsSource.
	Skills        int    `json:"skills"`
	SkillBytes    int64  `json:"skill_bytes"`
	SkillsSource  string `json:"skills_source,omitempty"`
	ContextDigest string `json:"context_digest,omitempty"`
}

// Bytes is the starting context's measured size: instructions plus skill
// descriptions.
func (c *StartContext) Bytes() int64 {
	if c == nil {
		return 0
	}
	return c.InstructionBytes + c.SkillBytes
}

// UnmarshalJSON reads the attribute back from a JSON string, the form it
// takes in the store.
func (c *StartContext) UnmarshalJSON(raw []byte) error {
	b, err := unquoted(raw)
	if err != nil {
		return err
	}
	type plain StartContext
	if string(b) == "null" {
		*c = StartContext{}
		return nil
	}
	return json.Unmarshal(b, (*plain)(c))
}

// AddInstruction records one file, keeping the totals and the source in step.
func (c *StartContext) AddInstruction(path, kind, source string, content []byte) {
	if len(c.Instructions) >= StartContextMaxInstructions {
		return
	}
	for _, have := range c.Instructions {
		if have.Path == path {
			return
		}
	}
	c.Instructions = append(c.Instructions, InstructionRef{Path: path, Kind: kind, Bytes: int64(len(content)), SHA256: ShortHash(content), Source: source})
	c.InstructionBytes += int64(len(content))
	c.Source = mergeSource(c.Source, source)
}

// SetSkills records the skill catalogue the session was offered.
func (c *StartContext) SetSkills(count int, bytes int64, source string) {
	c.Skills, c.SkillBytes, c.SkillsSource = count, bytes, source
	c.Source = mergeSource(c.Source, source)
}

func mergeSource(have, add string) string {
	switch {
	case have == "" || have == add:
		return add
	default:
		return "mixed"
	}
}

// ShortHash is the 12-hex-digit SHA-256 prefix recorded for instruction files.
func ShortHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}

// claudeStartAttachment reads Claude Code's start-of-session attachments:
// "instructions" (the CLAUDE.md files loaded, with their content) and
// "skill_listing" (the skill catalogue, with a count).
func claudeStartAttachment(raw json.RawMessage, c *StartContext) {
	var a struct {
		Type  string `json:"type"`
		Files []struct {
			Path    string `json:"path"`
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"files"`
		Content    string   `json:"content"`
		SkillCount int      `json:"skillCount"`
		Names      []string `json:"names"`
		IsInitial  *bool    `json:"isInitial"`
	}
	if json.Unmarshal(raw, &a) != nil {
		return
	}
	switch a.Type {
	case "instructions":
		for _, f := range a.Files {
			c.AddInstruction(f.Path, f.Type, "transcript", []byte(f.Content))
		}
	case "skill_listing":
		if c.SkillsSource == "transcript" || (a.IsInitial != nil && !*a.IsInitial) {
			return
		}
		n := a.SkillCount
		if n == 0 {
			n = len(a.Names)
		}
		c.SetSkills(n, int64(len(a.Content)), "transcript")
	}
}

var (
	codexAgentsHeader = regexp.MustCompile(`\A# AGENTS\.md instructions(?: for (\S+))?`)
	codexInstructions = regexp.MustCompile(`(?s)<INSTRUCTIONS>\n?(.*?)\n?</INSTRUCTIONS>`)
	codexSkillLine    = regexp.MustCompile(`(?m)^- [^\n]*\(file: [^\n]*SKILL\.md\)`)
)

// codexStartMessage reads Codex's start-of-session messages: the developer
// message carrying the skill catalogue (<skills_instructions>) and the user
// message carrying AGENTS.md ("# AGENTS.md instructions for DIR"). Codex
// merges every AGENTS.md it loaded into one block, so the record is the
// merged block, under DIR/AGENTS.md.
func codexStartMessage(role, text string, c *StartContext) {
	switch role {
	case "developer":
		if i := strings.Index(text, "<skills_instructions>"); i >= 0 && c.SkillsSource != "transcript" {
			body := text[i:]
			if j := strings.Index(body, "</skills_instructions>"); j >= 0 {
				body = body[:j+len("</skills_instructions>")]
			}
			c.SetSkills(len(codexSkillLine.FindAllString(body, -1)), int64(len(body)), "transcript")
		}
	case "user":
		m := codexAgentsHeader.FindStringSubmatch(text)
		if m == nil {
			return
		}
		body := text
		if inner := codexInstructions.FindStringSubmatch(text); inner != nil {
			body = inner[1]
		}
		path := "AGENTS.md"
		if m[1] != "" {
			path = strings.TrimRight(m[1], "/") + "/AGENTS.md"
		}
		c.AddInstruction(path, "merged", "transcript", []byte(body))
	}
}
