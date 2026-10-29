package trace

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Codex exec can wrap shell commands in JavaScript orchestration.
func shellSkillPaths(command string) []string {
	if commands, wrapped := javascriptExecCommands(command); wrapped {
		var out []string
		for _, cmd := range commands {
			out = append(out, shellReadSkillPaths(cmd)...)
		}
		return out
	}
	return shellReadSkillPaths(command)
}

func shellReadSkillPaths(command string) []string {
	f, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return nil
	} // Incomplete or unsupported shell: do not guess.
	var out []string
	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name, ok := literalWord(call.Args[0])
		if !ok {
			return true
		}
		name = path.Base(name)
		switch name {
		case "cat", "sed", "head", "tail", "less", "more", "bat", "nl", "awk", "rg", "grep":
		default:
			return true
		}
		// sed/awk programs and grep patterns are not file operands. Options that
		// consume an argument are skipped before looking for literal file paths.
		needsProgram := name == "sed" || name == "awk" || name == "rg" || name == "grep"
		options := true
		for i := 1; i < len(call.Args); i++ {
			arg, literal := literalWord(call.Args[i])
			if !literal {
				continue
			}
			if options && arg == "--" {
				options = false
				continue
			}
			if options && strings.HasPrefix(arg, "-") {
				if name == "rg" && arg == "--files" {
					return true
				}
				switch arg {
				case "-e", "--regexp", "--expression", "-f", "--file":
					i++
					needsProgram = false
				case "-n", "-c":
					if name == "head" || name == "tail" {
						i++
					}
				case "-v", "-F":
					if name == "awk" {
						i++
					}
				case "-A", "-B", "-C", "--glob", "-g", "--include", "--exclude":
					if name == "awk" || name == "rg" || name == "grep" {
						i++
					}
				}
				continue
			}
			if needsProgram {
				needsProgram = false
				continue
			}
			out = append(out, skillPaths(arg)...)
		}
		return true
	})
	return out
}

// Reject dynamic expansions: a historical environment cannot be reconstructed.
func literalWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			value, ok := literalWord(&syntax.Word{Parts: p.Parts})
			if !ok {
				return "", false
			}
			b.WriteString(value)
		default:
			return "", false
		}
	}
	return b.String(), true
}
