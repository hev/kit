package nativehost

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// verifyInterpreters binds the OS's first PATH choice for each declared script
// to declared resolved bytes. Merely hashing a later interpreter does not bind
// an env shebang to it. Dependency aliases may be symlinks; managed targets may
// not. This check is called by Profile.verify before every probe/worker spawn.
func verifyInterpreters(p Profile, path string) error {
	pins := map[string]bool{}
	for _, pin := range p.Runtime.Files {
		resolved, e := filepath.EvalSymlinks(pin.Path)
		if e != nil || resolved != pin.Path {
			return ErrHeld
		}
		pins[pin.Path] = true
	}
	visiting := map[string]bool{}
	var check func(string, int) error
	var dependency func(string, int) error
	dependency = func(selected string, depth int) error {
		resolved, e := filepath.EvalSymlinks(selected)
		if e != nil || !pins[resolved] {
			return ErrHeld
		}
		s, e := os.Stat(resolved)
		if e != nil || !s.Mode().IsRegular() || s.Mode().Perm()&0111 == 0 {
			return ErrHeld
		}
		return check(resolved, depth+1)
	}
	check = func(file string, depth int) error {
		if depth > 16 || visiting[file] {
			return ErrHeld
		}
		visiting[file] = true
		defer delete(visiting, file)
		f, e := os.Open(file)
		if e != nil {
			return ErrHeld
		}
		b, e := io.ReadAll(io.LimitReader(f, 4097))
		ce := f.Close()
		if e != nil || ce != nil {
			return ErrHeld
		}
		if !bytes.HasPrefix(b, []byte("#!")) {
			return nil
		}
		end := bytes.IndexByte(b, '\n')
		if end < 0 {
			end = len(b)
		}
		if end > 4096 {
			return ErrHeld
		}
		line := string(b[2:end])
		if strings.ContainsAny(line, "\x00\r'\"\\$#") {
			return ErrHeld
		}
		for _, char := range line {
			if char > 126 || (char < 32 && char != '\t') {
				return ErrHeld
			}
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !canonical(fields[0]) {
			return ErrHeld
		}
		if e = dependency(fields[0], depth); e != nil {
			return e
		}
		resolvedInterpreter, e := filepath.EvalSymlinks(fields[0])
		if e != nil {
			return ErrHeld
		}
		if filepath.Base(fields[0]) != "env" && filepath.Base(resolvedInterpreter) != "env" {
			return nil
		}
		args := fields[1:]
		// GNU env -S is supported only for plain whitespace-separated arguments.
		// Env options, assignments and quoting could choose another PATH/command;
		// reject those rather than implementing a different shell parser.
		if len(args) > 0 && args[0] == "-S" {
			args = args[1:]
		} else if len(args) != 1 {
			return ErrHeld
		}
		if len(args) == 0 || strings.HasPrefix(args[0], "-") || strings.Contains(args[0], "=") {
			return ErrHeld
		}
		command := args[0]
		if strings.ContainsRune(command, os.PathSeparator) {
			if !canonical(command) {
				return ErrHeld
			}
			return dependency(command, depth)
		}
		for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
			selected := filepath.Join(dir, command)
			s, e := os.Stat(selected)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return ErrHeld
			}
			if !s.Mode().IsRegular() || s.Mode().Perm()&0111 == 0 {
				continue
			}
			// The FIRST executable is authoritative, including an unpinned shadow.
			// Never keep searching for a later pinned binary.
			return dependency(selected, depth)
		}
		return ErrHeld
	}
	if e := check(p.SupervisorRef, 0); e != nil {
		return e
	}
	for _, pin := range p.Runtime.Files {
		if e := check(pin.Path, 0); e != nil {
			return e
		}
	}
	return nil
}
