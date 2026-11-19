package feedsend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// transcriptWindow bounds how much of a transcript's tail is read.
const transcriptWindow = 8 << 20

// Turn is the last exchange in a transcript: the person's prompt and what
// the agent wrote after it.
type Turn struct {
	Prompt string
	Reply  string
}

// LastTurn reads a Claude Code or Codex transcript and returns the last
// prompt before size and the agent's text after it. size is where the
// transcript ended when the turn stopped (0 for its current end). A harness
// may still be writing the turn's final reply when its Stop hook runs, so
// agent text past size is kept; a new prompt past size ends the turn.
// Tool calls and results are skipped; harness-injected user messages
// (system reminders, environment context) do not count as prompts.
func LastTurn(path string, size int64) (Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		return Turn{}, err
	}
	defer f.Close()
	end := int64(0)
	if info, err := f.Stat(); err == nil {
		end = info.Size()
	}
	if size <= 0 || size > end {
		size = end
	}
	start := int64(0)
	if size > transcriptWindow {
		start = size - transcriptWindow
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return Turn{}, err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	var t Turn
	var replies []string
	off := start
	first := start > 0
	for {
		line, err := r.ReadBytes('\n')
		at := off
		off += int64(len(line))
		if first {
			// A window that starts mid-file starts mid-line.
			first = false
		} else if len(line) > 0 {
			role, text := transcriptLine(line)
			switch {
			case role == "user" && at >= size:
				// The next turn has begun.
				t.Reply = strings.TrimSpace(strings.Join(replies, "\n\n"))
				return t, nil
			case role == "user":
				t.Prompt, replies = text, nil
			case role == "assistant":
				replies = append(replies, text)
			}
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			t.Reply = strings.TrimSpace(strings.Join(replies, "\n\n"))
			return t, err
		}
	}
}

// transcriptLine extracts a prompt ("user") or agent text ("assistant")
// from one JSONL line of either harness; anything else is "".
func transcriptLine(line []byte) (string, string) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return "", ""
	}
	var l struct {
		Type    string          `json:"type"`
		IsMeta  bool            `json:"isMeta"`
		Message json.RawMessage `json:"message"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(line, &l) != nil {
		return "", ""
	}
	switch l.Type {
	case "user", "assistant":
		// Claude Code: {"type":"user","message":{"role":"user","content":...}}
		if l.IsMeta {
			return "", ""
		}
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(l.Message, &m) != nil {
			return "", ""
		}
		return keep(l.Type, contentText(m.Content, l.Type == "user"))
	case "response_item":
		// Codex: {"type":"response_item","payload":{"type":"message","role":...,"content":[...]}}
		var p struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(l.Payload, &p) != nil || p.Type != "message" {
			return "", ""
		}
		if p.Role != "user" && p.Role != "assistant" {
			return "", ""
		}
		return keep(p.Role, contentText(p.Content, p.Role == "user"))
	}
	return "", ""
}

func keep(role, text string) (string, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if role == "user" && injected(text) {
		return "", ""
	}
	return role, text
}

// injected reports harness-made user messages: system reminders, command
// echoes, Codex environment context and instructions.
func injected(text string) bool {
	return strings.HasPrefix(text, "<") || strings.HasPrefix(text, "# AGENTS.md") || strings.HasPrefix(text, "Caveat: ")
}

// contentText joins the text blocks of a message's content (a string or a
// list of blocks). A user message made only of tool results has none.
func contentText(raw json.RawMessage, user bool) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text", "input_text", "output_text":
			if user && injected(strings.TrimSpace(b.Text)) {
				continue
			}
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
