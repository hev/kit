package trace

import (
	"strconv"
	"strings"
)

// jsSkillToken keeps strings distinct from code. Comments never become tokens,
// so neither a quoted example nor a commented call is treated as an invocation.
type jsSkillToken struct {
	text    string
	literal bool
}

func javascriptExecCommands(code string) ([]string, bool) {
	tokens := skillJSTokens(code)
	var commands []string
	wrapped := false
	for i := 0; i+5 < len(tokens); i++ {
		is := func(offset int, value string) bool { t := tokens[i+offset]; return !t.literal && t.text == value }
		if !is(0, "tools") || !is(1, ".") || !is(2, "exec_command") || !is(3, "(") || !is(4, "{") {
			continue
		}
		wrapped = true
		depth := 1
		for j := i + 5; j < len(tokens) && depth > 0; j++ {
			t := tokens[j]
			if !t.literal {
				switch t.text {
				case "{", "[", "(":
					depth++
				case "}", "]", ")":
					depth--
				}
			}
			if depth != 1 || (t.text != "cmd" && t.text != "command") || j+3 >= len(tokens) {
				continue
			}
			prev := tokens[j-1]
			colon, value, end := tokens[j+1], tokens[j+2], tokens[j+3]
			if prev.literal || (prev.text != "{" && prev.text != ",") || colon.literal || colon.text != ":" || !value.literal || end.literal || (end.text != "," && end.text != "}") {
				continue
			}
			commands = append(commands, value.text)
		}
	}
	return commands, wrapped
}

func skillJSTokens(code string) []jsSkillToken {
	var out []jsSkillToken
	for i := 0; i < len(code); {
		c := code[i]
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			i++
			continue
		}
		if c == '/' && i+1 < len(code) {
			if code[i+1] == '/' {
				i += 2
				for i < len(code) && code[i] != '\n' {
					i++
				}
				continue
			}
			if code[i+1] == '*' {
				end := strings.Index(code[i+2:], "*/")
				if end < 0 {
					break
				}
				i += end + 4
				continue
			}
		}
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			i++
			var value strings.Builder
			valid, closed := true, false
			for i < len(code) {
				if code[i] == quote {
					i++
					closed = true
					break
				}
				if quote == '`' && strings.HasPrefix(code[i:], "${") {
					valid = false
				}
				if code[i] == '\\' {
					if i+1 < len(code) && code[i+1] == '\n' {
						i += 2
						continue
					}
					// Go and JavaScript share the common quoted escapes used in cmd strings.
					r, _, rest, err := strconv.UnquoteChar(code[i:], quote)
					if err != nil {
						valid = false
						i++
						continue
					}
					value.WriteRune(r)
					i = len(code) - len(rest)
				} else {
					value.WriteByte(code[i])
					i++
				}
			}
			if !closed {
				break
			}
			// Invalid/dynamic strings stay opaque and cannot be command values.
			if !valid {
				out = append(out, jsSkillToken{text: "<dynamic>", literal: false})
			} else {
				out = append(out, jsSkillToken{text: value.String(), literal: true})
			}
			continue
		}
		start := i
		if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			i++
			for i < len(code) {
				c = code[i]
				if !(c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
					break
				}
				i++
			}
		} else {
			i++
		}
		out = append(out, jsSkillToken{text: code[start:i]})
	}
	return out
}
