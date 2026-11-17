package hooks

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Classify sorts a tool call into read, mutate, steer or other. It is a
// heuristic for holding mutations behind an unacknowledged urgent steer, not
// a permission system: anything it does not recognize is allowed.
func Classify(tool, command string) string {
	switch tool {
	case "Write", "Edit", "MultiEdit", "NotebookEdit", "apply_patch":
		return KindMutate
	case "Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch", "NotebookRead", "TodoWrite", "view_image":
		return KindRead
	case "Bash", "shell", "exec_command", "local_shell", "container.exec", "unified_exec", "PowerShell":
		return ClassifyShell(command)
	}
	return KindOther
}

var (
	segmentSplit = regexp.MustCompile(`&&|\|\||[;|\n&]|\$\(|` + "`" + `|\(|\)`)
	// Output redirection to a file. Descriptor duplication (2>&1) and
	// /dev/null are not writes.
	redirect     = regexp.MustCompile(`(?:^|[^<>&0-9])[0-9]?>>?\s*([^\s&|;<>()]+)`)
	fileWriters  = map[string]bool{"rm": true, "rmdir": true, "mv": true, "cp": true, "tee": true, "touch": true, "mkdir": true, "truncate": true, "dd": true, "install": true, "ln": true, "chmod": true, "chown": true, "patch": true, "unzip": true}
	gitWrites    = map[string]bool{"commit": true, "push": true, "merge": true, "rebase": true, "reset": true, "revert": true, "cherry-pick": true, "am": true, "apply": true, "tag": true, "checkout": true, "switch": true, "restore": true, "stash": true, "rm": true, "mv": true, "add": true, "clean": true, "pull": true, "worktree": true}
	ghPRWrites   = map[string]bool{"create": true, "merge": true, "edit": true, "close": true, "comment": true, "review": true, "ready": true, "reopen": true, "lock": true, "unlock": true}
	ghIssueWrite = map[string]bool{"create": true, "edit": true, "close": true, "comment": true, "reopen": true, "delete": true, "transfer": true, "lock": true, "unlock": true}
	hevReads     = map[string]map[string]bool{
		"dispatch":    {"ls": true, "show": true, "logs": true, "hosts": true, "notices": true, "attach": true, "": true, "help": true},
		"orchestrate": {"ls": true, "show": true, "logs": true, "": true, "help": true},
		"swarm":       {"ps": true, "logs": true, "": true, "help": true},
	}
)

// ClassifyShell classifies one shell command line.
func ClassifyShell(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return KindOther
	}
	steer := false
	for _, seg := range segmentSplit.Split(command, -1) {
		switch segmentKind(seg) {
		case KindMutate:
			return KindMutate
		case KindSteer:
			steer = true
		}
	}
	for _, m := range redirect.FindAllStringSubmatch(command, -1) {
		target := strings.Trim(m[1], `"'`)
		if target != "/dev/null" && target != "/dev/stderr" && target != "/dev/stdout" && !strings.HasPrefix(target, "&") {
			return KindMutate
		}
	}
	if steer {
		return KindSteer
	}
	return KindRead
}

func segmentKind(seg string) string {
	words := strings.Fields(seg)
	// Skip environment assignments and transparent prefixes.
	for len(words) > 0 {
		w := words[0]
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "=") {
			words = words[1:]
			continue
		}
		switch w {
		case "sudo", "env", "time", "nohup", "exec", "command", "timeout", "nice", "xargs", "then", "do", "else", "!", "{":
			words = words[1:]
			// timeout's duration argument.
			if w == "timeout" && len(words) > 0 && len(words[0]) > 0 && (words[0][0] >= '0' && words[0][0] <= '9') {
				words = words[1:]
			}
			continue
		}
		break
	}
	if len(words) == 0 {
		return KindRead
	}
	name := strings.TrimSuffix(filepath.Base(strings.Trim(words[0], `"'`)), ".exe")
	args := words[1:]
	switch {
	case name == "git":
		if gitWrites[subcommand(args, map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true})] {
			return KindMutate
		}
	case name == "gh":
		sub := subcommand(args, map[string]bool{"-R": true, "--repo": true})
		rest := after(args, sub)
		switch sub {
		case "pr":
			if ghPRWrites[subcommand(rest, nil)] {
				return KindMutate
			}
		case "issue":
			if ghIssueWrite[subcommand(rest, nil)] {
				return KindMutate
			}
		case "release", "repo", "workflow", "run", "secret", "variable", "label":
			switch subcommand(rest, nil) {
			case "create", "delete", "edit", "upload", "run", "rerun", "cancel", "set", "fork", "rename", "archive", "sync":
				return KindMutate
			}
		case "api":
			for i, a := range rest {
				up := strings.ToUpper(a)
				if (a == "-X" || a == "--method") && i+1 < len(rest) && strings.ToUpper(rest[i+1]) != "GET" {
					return KindMutate
				}
				if strings.HasPrefix(up, "-XP") || strings.HasPrefix(up, "-XD") || strings.HasPrefix(up, "--METHOD=P") || strings.HasPrefix(up, "--METHOD=D") {
					return KindMutate
				}
				if a == "-f" || a == "-F" || a == "--field" || a == "--raw-field" || a == "--input" {
					return KindMutate
				}
			}
		}
	case name == "hev" || name == "kit":
		sub := subcommand(args, map[string]bool{"--config": true, "--host": true})
		if sub == "steer" {
			return KindSteer
		}
		if reads, ok := hevReads[sub]; ok && !reads[subcommand(after(args, sub), map[string]bool{"--config": true, "--host": true})] {
			return KindMutate
		}
	case name == "sed" || name == "perl":
		for _, a := range args {
			if a == "-i" || strings.HasPrefix(a, "-i") || a == "--in-place" || strings.HasPrefix(a, "-pi") {
				return KindMutate
			}
		}
	case fileWriters[name]:
		return KindMutate
	}
	return KindRead
}

// subcommand is the first non-flag word, skipping the values of flags that
// take one.
func subcommand(args []string, withValue map[string]bool) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if withValue[a] {
				i++
			}
			continue
		}
		return strings.Trim(a, `"'`)
	}
	return ""
}

func after(args []string, word string) []string {
	for i, a := range args {
		if strings.Trim(a, `"'`) == word {
			return args[i+1:]
		}
	}
	return nil
}
