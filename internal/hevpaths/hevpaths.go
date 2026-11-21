// Package hevpaths resolves where kit keeps each of its six OSS data
// surfaces on disk. Every fresh install keeps them side by side under
// ~/.hev/<surface>/, one directory per surface of the Pro API
// (/v1/{traces,board,orders,peers,hooks,evidence}):
//
//	~/.hev/traces/    session archive         (legacy ~/.hev/archive/)
//	~/.hev/board/     board threads
//	~/.hev/orders/    dispatch run records    (legacy ~/dispatch/)
//	~/.hev/peers/     peers inboxes and logs  (legacy ~/.local/state/peers-v2/)
//	~/.hev/hooks/     hook event logs
//	~/.hev/evidence/  evidence packages
//
// A machine set up before the layout keeps working where its data already
// is. The rule, applied per surface, is:
//
//  1. an explicit environment variable (or, for the caller that has one, a
//     config field) wins over everything;
//  2. otherwise ~/.hev/<surface>/ when it exists;
//  3. otherwise the legacy directory when it exists;
//  4. otherwise ~/.hev/<surface>/ (a fresh install).
//
// Nothing is ever moved: a host on a legacy directory stays on it until its
// operator moves the data and the directory together. Because the new
// directory wins as soon as it exists, nothing in kit may create
// ~/.hev/<surface>/ for a surface that resolves to its legacy directory;
// EnsureAll creates only the resolved directories.
package hevpaths

import (
	"os"
	"path/filepath"
	"strings"
)

// Surface names one OSS data surface. The value is its directory name under
// ~/.hev and its route under /v1 in Pro.
type Surface string

const (
	Traces   Surface = "traces"
	Board    Surface = "board"
	Orders   Surface = "orders"
	Peers    Surface = "peers"
	Hooks    Surface = "hooks"
	Evidence Surface = "evidence"
)

// All lists every surface in display order.
var All = []Surface{Traces, Board, Orders, Peers, Hooks, Evidence}

// Env names the variable that overrides each surface's directory. A surface
// with no entry has no override.
var Env = map[Surface]string{
	Traces:   "HEV_ARCHIVE_DIR",
	Board:    "HEV_BOARD_DIR",
	Orders:   "HEV_ORDERS_DIR",
	Peers:    "HEV_PEERS_STATE",
	Evidence: "HEV_EVIDENCE_DIR",
}

// Source says which rule chose a directory.
type Source string

const (
	SourceEnv     Source = "env"
	SourceDefault Source = "default"
	SourceLegacy  Source = "legacy"
)

// Resolution is where one surface lives and why.
type Resolution struct {
	Surface Surface
	Dir     string
	Source  Source
}

// Base is ~/.hev under home.
func Base(home string) string { return filepath.Join(home, ".hev") }

// Default is ~/.hev/<surface> under home.
func Default(home string, s Surface) string { return filepath.Join(Base(home), string(s)) }

// Legacy is the directory a surface used before the unified layout, or "" if
// it never had another.
func Legacy(home string, s Surface) string {
	switch s {
	case Traces:
		return filepath.Join(home, ".hev", "archive")
	case Orders:
		return filepath.Join(home, "dispatch")
	case Peers:
		return filepath.Join(home, ".local", "state", "peers-v2")
	}
	return ""
}

// ResolveHome applies rules 2-4 under home, ignoring the environment: the
// caller that also has a config field checks env, then config, then this.
func ResolveHome(home string, s Surface) Resolution {
	def := Default(home, s)
	if isDir(def) {
		return Resolution{s, def, SourceDefault}
	}
	if legacy := Legacy(home, s); legacy != "" && isDir(legacy) {
		return Resolution{s, legacy, SourceLegacy}
	}
	return Resolution{s, def, SourceDefault}
}

// Resolve applies the whole rule for the current user: the surface's
// environment variable, then ResolveHome.
func Resolve(s Surface) Resolution {
	if name := Env[s]; name != "" {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return Resolution{s, v, SourceEnv}
		}
	}
	return ResolveHome(Home(), s)
}

// Dir is Resolve(s).Dir.
func Dir(s Surface) string { return Resolve(s).Dir }

// Home is the user's home directory, or "." when it cannot be found, so a
// broken environment resolves relative paths rather than "/" paths.
func Home() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

// EnsureAll creates every surface's resolved directory (mode 0700 for a new
// one) and returns the resolutions. It never creates ~/.hev/<surface> for a
// surface that resolves elsewhere, so a legacy host stays on its legacy
// directories and never creates the legacy directories on a fresh one.
func EnsureAll() ([]Resolution, error) {
	var out []Resolution
	for _, s := range All {
		r := Resolve(s)
		if err := os.MkdirAll(r.Dir, 0o700); err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
