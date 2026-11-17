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
// cmd (or command) property is a string literal written at the call site:
// tools.exec_command({cmd: "…"}). A template literal counts only when it has
// no ${…} substitution. Any other value, such as a name ({cmd} or
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

// literalCommand reads the object literal opening at token i and returns
// its cmd/command property when that is a lone string literal.
func literalCommand(t []jsSkillToken, i int) (string, bool) {
	if i >= len(t) || t[i].literal || t[i].text != "{" {
		return "", false
	}
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
		// [start, j) is one top-level property.
		if j-start >= 1 && !t[start].literal && (t[start].text == "cmd" || t[start].text == "command") {
			if j-start == 3 && !t[start+1].literal && t[start+1].text == ":" && t[start+2].literal {
				return t[start+2].text, true
			}
			return "", false
		}
		if end {
			return "", false
		}
		start = j + 1
	}
	return "", false
}
