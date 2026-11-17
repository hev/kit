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

func skillJSTokens(code string) []jsSkillToken { return jsTokens(code, false) }

// jsTokens tokenizes JavaScript. With templates, a template literal's
// ${…} substitutions become the shell parameter "${dyn}" and the literal
// stays a string, so a command built from one keeps its fixed words;
// without, such a literal is opaque.
func jsTokens(code string, templates bool) []jsSkillToken {
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
					if !templates {
						valid = false
					} else {
						depth := 0
						for i < len(code) {
							if code[i] == '{' {
								depth++
							} else if code[i] == '}' {
								depth--
								if depth == 0 {
									i++
									break
								}
							}
							i++
						}
						value.WriteString("${dyn}")
						continue
					}
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
