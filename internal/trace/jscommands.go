package trace

// UnknownCommandKey is the command_counts key for code-mode JavaScript
// calls of tools.exec_command whose command is not a string literal at the
// call site. Their commands are unknown: they count toward no other
// command fact and never mark an edit.
const UnknownCommandKey = "unknown"

// javascriptCommandStrings returns the shell scripts code-mode JavaScript
// passes to tools.exec_command, how many calls passed a command it cannot
// read, and whether the code calls tools.exec_command at all.
//
// A command counts only when the call's argument is an object literal whose
// cmd (or command) property appears exactly once, as a string literal
// written at the call site, and the object has no spread and no computed
// key anywhere: tools.exec_command({cmd: "…", yield_time_ms: 1000}). A
// template literal counts only when it has no ${…} substitution. A spread
// or a second cmd/command before or after the literal could replace it, so
// either makes the call unknown. Any other value, such as a name ({cmd} or
// {cmd: c}), a member (o.cmd), a callback parameter, a template with
// substitutions, a computed expression or a whole object passed by name
// (tools.exec_command(o)), is unknown: the call is counted in unknown and
// nothing else. Names are never resolved, so a string elsewhere in the
// script, whatever it says, is never taken for a command.
func javascriptCommandStrings(code string) (commands []string, unknown int, wrapped bool) {
	t := skillJSTokens(code) // a template with ${…} is opaque
	punct := func(i int, v string) bool { return i >= 0 && i < len(t) && !t[i].literal && t[i].text == v }
	for i := 0; i+4 < len(t); i++ {
		if !punct(i, "tools") || !punct(i+1, ".") || !punct(i+2, "exec_command") || !punct(i+3, "(") {
			continue
		}
		wrapped = true
		if c, ok := literalCommand(t, i+4); ok {
			commands = append(commands, c)
		} else {
			unknown++
		}
	}
	return commands, unknown, wrapped
}

// literalCommand reads the whole argument that opens at token i and returns
// its command when the rule holds for the entire object: the argument is an
// object literal, closed and followed by ")" or ",", with no spread, no
// computed key, and exactly one property named cmd or command (by any
// spelling: name, quoted key, shorthand, method or accessor), written as
// cmd: "literal". cmd and command count as one property, since a call that
// gives both is ambiguous. Anything else is unknown.
func literalCommand(t []jsSkillToken, i int) (string, bool) {
	if i >= len(t) || t[i].literal || t[i].text != "{" {
		return "", false
	}
	punct := func(k int, v string) bool { return k < len(t) && !t[k].literal && t[k].text == v }
	isCommand := func(k int) bool { return k < len(t) && (t[k].text == "cmd" || t[k].text == "command") }
	var command string
	occurrences, plain := 0, false
	depth, start := 0, i+1
	for j := i; j < len(t); j++ {
		if t[j].literal {
			continue
		}
		end := false
		switch t[j].text {
		case "{", "[", "(":
			depth++
			continue
		case "}", "]", ")":
			depth--
			if depth > 0 {
				continue
			}
			end = true
		case ",":
			if depth != 1 {
				continue
			}
		default:
			continue
		}
		if depth < 0 || (end && t[j].text != "}") {
			return "", false
		}
		// [start, j) is one top-level property; empty is a trailing comma.
		if j > start {
			key := start
			if punct(key, ".") || punct(key, "[") {
				return "", false // spread or computed key
			}
			// A method or accessor (get cmd() {}, async *cmd() {}) names the
			// property after its modifiers.
			for key+1 < j && !t[key].literal && (t[key].text == "get" || t[key].text == "set" || t[key].text == "async" || t[key].text == "*") &&
				!punct(key+1, ":") && !punct(key+1, "(") {
				key++
			}
			if punct(key, "[") {
				return "", false
			}
			if isCommand(key) {
				occurrences++
				if key == start && j-start == 3 && punct(start+1, ":") && t[start+2].literal {
					command, plain = t[start+2].text, true
				}
			}
		}
		if end {
			if occurrences != 1 || !plain || !(punct(j+1, ")") || punct(j+1, ",")) {
				return "", false
			}
			return command, true
		}
		start = j + 1
	}
	return "", false
}
