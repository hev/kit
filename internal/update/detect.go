package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PackageManager names the package manager that owns the hev at path, or "".
// Kit never replaces a file a package manager owns: the next upgrade there
// would fight it, and the manager's own records would be wrong.
func PackageManager(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	for _, p := range []string{path, resolved} {
		s := filepath.ToSlash(p)
		switch {
		case strings.Contains(s, "/Cellar/") || strings.Contains(s, "/homebrew/") || strings.Contains(s, "/linuxbrew/"):
			return "Homebrew"
		case strings.HasPrefix(s, "/nix/store/"):
			return "Nix"
		case strings.Contains(strings.ToLower(s), "/scoop/apps/"):
			return "Scoop"
		case strings.HasPrefix(s, "/usr/bin/") || strings.HasPrefix(s, "/bin/") || strings.HasPrefix(s, "/usr/sbin/"):
			return "the system package manager"
		}
	}
	return ""
}

func managedError(pm, path string) error {
	switch pm {
	case "Homebrew":
		return fmt.Errorf("%w: %s is managed by Homebrew, so hev update leaves it alone. For releases run `brew upgrade hev/tap/hev`. "+
			"Homebrew installs releases only; to follow edge, install hev from a release archive into a directory you own "+
			"(for example ~/.local/bin, ahead of Homebrew on PATH) and run `hev update --channel edge` from there", ErrManaged, path)
	default:
		return fmt.Errorf("%w: %s is managed by %s, so hev update leaves it alone; upgrade it there, "+
			"or install hev from a release archive into a directory you own and run hev update from that copy", ErrManaged, path, pm)
	}
}

// Invoked is the path this process was started as, before symbolic links
// are resolved (os.Executable resolves them on Linux).
func Invoked() string {
	arg := os.Args[0]
	if !strings.ContainsRune(arg, os.PathSeparator) && !strings.ContainsRune(arg, '/') {
		if p, err := exec.LookPath(arg); err == nil {
			arg = p
		} else {
			return ""
		}
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return ""
	}
	return abs
}
