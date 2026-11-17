package trace

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellToolCommand returns the shell script a shell-like tool call ran, from
// its block text: the tool name, then the rendered input (Claude's command,
// Codex's "cmd: …" lines, a JSON object for PI and others).
func shellToolCommand(toolName, text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(text, toolName))
	for _, key := range []string{"cmd: ", "command: ", "script: ", "code: "} {
		text = strings.TrimPrefix(text, key)
	}
	// Decode JSON escapes in command objects.
	if i := strings.Index(text, "{"); i >= 0 {
		var obj any
		if json.Unmarshal([]byte(text[i:]), &obj) == nil {
			if s := commandStrings(obj); s != "" {
				text = s
			}
		}
	}
	return strings.TrimSpace(codexTrailingArgs.ReplaceAllString(text, ""))
}

// codexTrailingArgs matches the argument lines Codex renders after cmd
// (keys sort alphabetically), which are not part of the script.
var codexTrailingArgs = regexp.MustCompile(`(?m)\n(?:justification|login|max_output_tokens|prefix_rule|sandbox_permissions|shell|timeout_ms|tty|with_escalated_permissions|workdir|yield_time_ms): [^\n]*$`)

// toolInvocations returns the argument vectors of the commands a shell-like
// tool call executed, and whether the call applied a patch from code-mode
// JavaScript. Only executed command nodes count: words inside quoted
// arguments, heredoc bodies fed to another program, comments and JavaScript
// string literals are data, not commands. A script that does not parse
// yields nothing rather than a guess.
func toolInvocations(toolName, text string) (argvs [][]string, jsPatch bool) {
	script := shellToolCommand(toolName, text)
	if commands, wrapped := javascriptCommandStrings(script); wrapped || javascriptCallsTool(script, "apply_patch") {
		for _, c := range commands {
			argvs = append(argvs, shellInvocations(c, 0)...)
		}
		return argvs, javascriptCallsTool(script, "apply_patch")
	}
	return shellInvocations(script, 0), false
}

// javascriptCommandStrings returns the shell scripts code-mode JavaScript
// passes to tools.exec_command: a literal cmd value (template literals keep
// their fixed words), and, when a cmd value is not a literal (cmd: c over a
// list, or tools.exec_command(o)), the string elements of the script's array
// literals and the literal cmd values of its objects, the way a command list
// is written. Comments and other strings are never commands.
func javascriptCommandStrings(code string) ([]string, bool) {
	tokens := jsTokens(code, true)
	var commands []string
	wrapped, indirect := false, false
	for i := 0; i+5 < len(tokens); i++ {
		is := func(offset int, value string) bool { t := tokens[i+offset]; return !t.literal && t.text == value }
		if !is(0, "tools") || !is(1, ".") || !is(2, "exec_command") || !is(3, "(") {
			continue
		}
		wrapped = true
		if !is(4, "{") {
			indirect = true // tools.exec_command(arg)
			continue
		}
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
			if t.literal || depth != 1 || (t.text != "cmd" && t.text != "command") || j+2 >= len(tokens) {
				continue
			}
			if prev := tokens[j-1]; prev.literal || (prev.text != "{" && prev.text != ",") || tokens[j+1].literal {
				continue
			}
			if tokens[j+1].text == "," || tokens[j+1].text == "}" {
				indirect = true // shorthand {cmd, …}
				continue
			}
			if tokens[j+1].text != ":" {
				continue
			}
			value := tokens[j+2]
			if value.literal && j+3 < len(tokens) && !tokens[j+3].literal && (tokens[j+3].text == "," || tokens[j+3].text == "}") {
				commands = append(commands, value.text)
			} else {
				indirect = true
			}
		}
	}
	if indirect {
		for i := 1; i+3 < len(tokens); i++ {
			// {cmd: "…"} objects built for a later tools.exec_command(o).
			t, prev := tokens[i], tokens[i-1]
			if !t.literal && (t.text == "cmd" || t.text == "command") && !prev.literal && (prev.text == "{" || prev.text == ",") && !tokens[i+1].literal && tokens[i+1].text == ":" && tokens[i+2].literal && !tokens[i+3].literal && (tokens[i+3].text == "," || tokens[i+3].text == "}") {
				commands = append(commands, tokens[i+2].text)
			}
		}
		for i := 1; i+1 < len(tokens); i++ {
			t, prev, next := tokens[i], tokens[i-1], tokens[i+1]
			if t.literal && !prev.literal && (prev.text == "[" || prev.text == ",") && !next.literal && (next.text == "," || next.text == "]") && inArray(tokens, i) {
				commands = append(commands, t.text)
			}
		}
	}
	return commands, wrapped
}

// inArray reports whether token i sits directly inside an array literal.
func inArray(tokens []jsSkillToken, i int) bool {
	depth := 0
	for j := i - 1; j >= 0; j-- {
		if tokens[j].literal {
			continue
		}
		switch tokens[j].text {
		case "]", "}", ")":
			depth++
		case "[", "{", "(":
			if depth == 0 {
				return tokens[j].text == "["
			}
			depth--
		}
	}
	return false
}

// javascriptCallsTool reports a call of tools.NAME( in code-mode JavaScript.
func javascriptCallsTool(code, name string) bool {
	if !strings.Contains(code, name) || !strings.Contains(code, "tools") {
		return false
	}
	tokens := skillJSTokens(code)
	for i := 0; i+3 < len(tokens); i++ {
		is := func(offset int, value string) bool { t := tokens[i+offset]; return !t.literal && t.text == value }
		if is(0, "tools") && is(1, ".") && is(2, name) && is(3, "(") {
			return true
		}
	}
	return false
}

// shellInvocations parses script and returns every simple command's literal
// argv, with launch wrappers (env, sudo, timeout, …) removed and the script
// of sh -c / bash -lc followed.
func shellInvocations(script string, depth int) [][]string {
	if depth > 3 || strings.TrimSpace(script) == "" {
		return nil
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return nil
	}
	var out [][]string
	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		var argv []string
		for _, w := range call.Args {
			s, ok := literalWord(w)
			if !ok {
				s = "\x00" // dynamic: never matches a command word
			}
			argv = append(argv, s)
		}
		argv = unwrapLauncher(argv)
		if len(argv) == 0 {
			return true
		}
		switch path.Base(argv[0]) {
		case "sh", "bash", "zsh", "dash":
			for i := 1; i+1 < len(argv); i++ {
				if strings.HasPrefix(argv[i], "-") && !strings.HasPrefix(argv[i], "--") && strings.Contains(argv[i], "c") {
					out = append(out, shellInvocations(argv[i+1], depth+1)...)
					break
				}
			}
		}
		out = append(out, argv)
		return true
	})
	return out
}

// unwrapLauncher strips commands that run their arguments as a command.
func unwrapLauncher(argv []string) []string {
	for len(argv) > 0 {
		switch path.Base(argv[0]) {
		case "sudo", "nohup", "time", "command", "exec", "with-op", "nice", "stdbuf":
			argv = argv[1:]
			for len(argv) > 0 && strings.HasPrefix(argv[0], "-") {
				argv = argv[1:]
			}
		case "env":
			argv = argv[1:]
			for len(argv) > 0 && (strings.HasPrefix(argv[0], "-") || strings.Contains(argv[0], "=")) {
				argv = argv[1:]
			}
		case "timeout":
			argv = argv[1:]
			for len(argv) > 0 && strings.HasPrefix(argv[0], "-") {
				argv = argv[1:]
			}
			if len(argv) > 0 {
				argv = argv[1:] // the duration
			}
		default:
			return argv
		}
	}
	return argv
}

// hevSubcommand returns the words of a hev invocation after the program
// name and any leading global flags; nil when argv is not hev.
func hevSubcommand(argv []string) []string {
	if len(argv) == 0 || path.Base(argv[0]) != "hev" {
		return nil
	}
	rest := argv[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	return rest
}

// commandKey classifies an executed command for command_counts.
func commandKey(argv []string) string {
	sub := hevSubcommand(argv)
	at := func(i int, v ...string) bool {
		if i >= len(sub) {
			return false
		}
		for _, x := range v {
			if sub[i] == x {
				return true
			}
		}
		return false
	}
	switch {
	case at(0, "board") && at(1, "search"):
		return "board_search"
	case at(0, "board") && at(1, "post", "reply"):
		return "board_post"
	case at(0, "traces") && at(1, "search"):
		return "traces_search"
	case at(0, "dispatch"):
		return "dispatch"
	}
	return ""
}

// editsFiles reports an executed command that changes the tree: a patch
// applied with apply_patch, or a git commit.
func editsFiles(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch path.Base(argv[0]) {
	case "apply_patch", "applypatch":
		return true
	case "git":
		for i := 1; i < len(argv); i++ {
			switch a := argv[i]; {
			case a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace":
				i++
			case strings.HasPrefix(a, "-"):
			default:
				return a == "commit"
			}
		}
	}
	return false
}
