package trace

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Exit statuses are read only from what the harness itself wrote around a
// command's output, never from the output: a document that happens to start
// "Exit code: 2" is content, not a status.
//
//   - Claude Code prefixes a failed Bash result with "Exit code N".
//   - Codex code mode (tools exec and wait) heads its result with "Script
//     completed|failed|running", then one item per value the script emitted.
//     An exec_command result is a JSON envelope carrying wall_time_seconds or
//     chunk_id beside exit_code, possibly wrapped in orchestration envelopes
//     ({"status":"fulfilled","value":…}, {"session":…,"result":…}) or arrays.
//     The command's own output is a JSON string inside the envelope.
//   - Codex direct shell tools (exec_command, write_stdin, shell, …) head
//     their result with metadata lines ("Process exited with code N",
//     "Exit code: N") before an "Output:" line, or return
//     {"output":…,"metadata":{"exit_code":N}}.
//   - PI's bash tool appends "Command exited with code N" as the last line.
var (
	claudeExit = regexp.MustCompile(`\AExit code (-?\d+)\b`)
	headerExit = regexp.MustCompile(`^(?:Process exited with code|Exit code:) (-?\d+)\s*$`)
	piExit     = regexp.MustCompile(`\ACommand exited with code (-?\d+)\s*\z`)
	agyExit    = regexp.MustCompile(`^Command exited with code (-?\d+)\.?\s*$`)
)

func bareTool(name string) string {
	return strings.TrimPrefix(strings.ToLower(name), "functions.")
}

// shellTool reports whether a tool's results carry a command's exit status
// in the harness's own metadata.
func shellTool(harness, name string) bool {
	n := bareTool(name)
	switch harness {
	case "claude_code", "pi":
		return n == "bash"
	case "codex":
		return codeModeTool(n) || codexDirectShell(n)
	case "agy":
		return n == "run_command"
	}
	switch n {
	case "bash", "shell":
		return true
	}
	return false
}

func codeModeTool(n string) bool { return n == "exec" || n == "wait" }

func codexDirectShell(n string) bool {
	switch n {
	case "exec_command", "write_stdin", "shell", "shell_command", "local_shell", "unified_exec", "container.exec":
		return true
	}
	return false
}

// CaptureToolOutcomes sets each tool result's ResultBytes and, where the
// harness reports one, its ExitCode; a Codex result whose script failed or
// whose command exited non-zero is marked IsError, because Codex records no
// error flag of its own. A value already set (by a parser that reads a
// native exit status, or by an earlier pass) is kept. It reads only the
// result's head, never stores result text anywhere new, and is idempotent.
func CaptureToolOutcomes(turns []Turn) []Turn {
	tools := map[string]string{}
	for ti := range turns {
		for bi := range turns[ti].Blocks {
			b := &turns[ti].Blocks[bi]
			switch b.Type {
			case "tool_use":
				if b.ToolUseID != "" {
					tools[b.ToolUseID] = b.ToolName
				}
			case "tool_result":
				name := b.ToolName
				if name == "" {
					name = tools[b.ToolUseID]
				}
				if b.ResultBytes == 0 {
					b.ResultBytes = int64(len(b.Text))
				}
				harness := turns[ti].Harness
				if b.ExitCode == nil {
					b.ExitCode = exitCode(harness, name, b.Text, b.IsError)
				}
				if harness == "codex" && shellTool(harness, name) {
					if (codeModeTool(bareTool(name)) && strings.HasPrefix(b.Text, "Script failed")) || (b.ExitCode != nil && *b.ExitCode != 0) {
						b.IsError = true
					}
				}
			}
		}
	}
	return turns
}

func parseCode(s string) *int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func exitCode(harness, tool, text string, isError bool) *int {
	if !shellTool(harness, tool) {
		return nil
	}
	switch harness {
	case "claude_code":
		// Claude Code writes the marker only on a failed call; a
		// successful call's output starting the same way is content.
		if !isError {
			zero := 0
			return &zero
		}
		if m := claudeExit.FindStringSubmatch(head(text, 256)); m != nil {
			return parseCode(m[1])
		}
		return nil
	case "pi":
		return lastLineExit(text)
	case "codex":
		if codeModeTool(bareTool(tool)) {
			return codeModeExit(text)
		}
		return directShellExit(text)
	case "agy":
		return agyCommandExit(text)
	}
	if code := lastLineExit(text); code != nil {
		return code
	}
	return directShellExit(text)
}

func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// lastLineExit reads PI's trailing "Command exited with code N".
func lastLineExit(text string) *int {
	t := strings.TrimRight(text, " \t\r\n")
	if i := strings.LastIndexByte(t, '\n'); i >= 0 {
		t = t[i+1:]
	}
	if m := piExit.FindStringSubmatch(t); m != nil {
		return parseCode(m[1])
	}
	return nil
}

// agyCommandExit reads Antigravity's "Command exited with code N." header line
// before "Command output:" (scanning at most the first 12 header lines so
// command stdout is never interpreted as an exit status).
func agyCommandExit(text string) *int {
	lines := strings.SplitN(head(text, 2048), "\n", 13)
	for i, line := range lines {
		if i >= 12 {
			break
		}
		line = strings.TrimRight(line, "\r")
		if line == "Command output:" {
			break
		}
		if m := agyExit.FindStringSubmatch(line); m != nil {
			return parseCode(m[1])
		}
	}
	return nil
}

// metadataHeader returns the lines a Codex shell tool writes before its
// "Output:" line, or the first few lines when there is none.
func metadataHeader(text string) []string {
	lines := strings.SplitN(head(text, 4096), "\n", 9)
	if len(lines) > 8 {
		lines = lines[:8]
	}
	for i, l := range lines {
		if strings.TrimSpace(l) == "Output:" {
			return lines[:i]
		}
	}
	if len(lines) > 5 {
		lines = lines[:5]
	}
	return lines
}

func directShellExit(text string) *int {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		// The JSON shell form: {"output":…,"metadata":{"exit_code":N}}.
		var v struct {
			Metadata struct {
				ExitCode *int `json:"exit_code"`
			} `json:"metadata"`
		}
		if json.Unmarshal([]byte(trimmed), &v) == nil && v.Metadata.ExitCode != nil {
			return v.Metadata.ExitCode
		}
		return envelopeExit(trimmed)
	}
	for _, l := range metadataHeader(text) {
		if m := headerExit.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			return parseCode(m[1])
		}
	}
	return nil
}

// codeModeExit reads the exec envelopes a Codex code-mode script emitted
// and reports the first non-zero status, else zero, else none. Only items
// after the "Script …" header and "Output:" line are read, and only where
// an item begins a line, so command output (escaped inside an envelope's
// output string) is never parsed.
func codeModeExit(text string) *int {
	body := text
	if !strings.HasPrefix(text, "Script ") {
		// A bare envelope, with no script header.
		if c := firstNonSpace(text); c != '{' && c != '[' {
			return nil
		}
		return envelopeExit(text)
	}
	if i := strings.Index(body, "\nOutput:\n"); i >= 0 {
		body = body[i+len("\nOutput:\n"):]
	} else if i := strings.Index(body, "\nOutput:"); i >= 0 {
		body = body[i+len("\nOutput:"):]
	} else {
		return nil
	}
	return envelopeExit(body)
}

// envelopeExit scans the JSON items that begin lines of body.
func envelopeExit(body string) *int {
	body = head(body, 1<<20)
	var code *int
	note := func(n int) {
		if code == nil || (*code == 0 && n != 0) {
			c := n
			code = &c
		}
	}
	for off := 0; off < len(body); {
		line := body[off:]
		if c := firstNonSpace(line); c == '{' || c == '[' {
			consumed := scanEnvelopes(line, note)
			if consumed > 0 {
				off += consumed
				continue
			}
		}
		i := strings.IndexByte(line, '\n')
		if i < 0 {
			break
		}
		off += i + 1
	}
	return code
}

func firstNonSpace(s string) byte {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r':
			continue
		}
		return s[i]
	}
	return 0
}

// scanEnvelopes decodes one JSON value from the start of s, tolerating
// truncation, calls note for every exec envelope in it and returns how
// many bytes it consumed (0 when nothing decoded).
func scanEnvelopes(s string, note func(int)) int {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	walkJSON(dec, 0, note)
	return int(dec.InputOffset())
}

// walkJSON reads one value from dec. In an object, an exit_code number
// counts when the same object carries wall_time_seconds or chunk_id (an
// exec envelope); nested objects and arrays are walked, strings skipped.
// It stops quietly at a decode error, keeping what it saw.
func walkJSON(dec *json.Decoder, depth int, note func(int)) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return true
	}
	if depth > 16 {
		return skipValue(dec)
	}
	switch delim {
	case '[':
		for dec.More() {
			if !walkJSON(dec, depth+1, note) {
				return false
			}
		}
	case '{':
		var exit *int
		envelope := false
		complete := true
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				complete = false
				break
			}
			key, _ := kt.(string)
			switch key {
			case "wall_time_seconds", "chunk_id":
				envelope = true
				if !walkJSON(dec, depth+1, note) {
					complete = false
				}
			case "exit_code":
				vt, err := dec.Token()
				if err != nil {
					complete = false
					break
				}
				if n, ok := vt.(json.Number); ok {
					if i, err := strconv.Atoi(n.String()); err == nil {
						exit = &i
					}
				} else if _, ok := vt.(json.Delim); ok {
					if !skipValue(dec) {
						complete = false
					}
				}
			default:
				if !walkJSON(dec, depth+1, note) {
					complete = false
				}
			}
			if !complete {
				break
			}
		}
		if envelope && exit != nil {
			note(*exit)
		}
		if !complete {
			return false
		}
	default:
		return true
	}
	_, err = dec.Token() // closing delimiter
	return err == nil
}

func skipValue(dec *json.Decoder) bool {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			default:
				depth--
			}
		}
	}
	return true
}
