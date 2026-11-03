package index

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Route pins the destination at first indexing. The machine allowlist is
// rechecked before every write, including retries of held sessions.
// It is deliberately separate from signatures: growing transcripts and force
// indexing must keep their original destination.
type Route struct {
	Namespace string `json:"namespace,omitempty"`
	Blocked   bool   `json:"blocked,omitempty"`
	Skip      bool   `json:"skip,omitempty"`
	Project   bool   `json:"project,omitempty"`
	Warned    bool   `json:"warned,omitempty"`
}

type projectFile struct {
	Capture struct {
		Namespace string `toml:"namespace"`
		Archive   *bool  `toml:"archive"`
	} `toml:"capture"`
}
type projectStamp struct {
	path string
	mod  time.Time
	size int64
	mode os.FileMode
}
type projectEntry struct {
	stamps  []projectStamp
	config  projectFile
	invalid bool
}

var routeMu sync.Mutex

var projectCache = struct {
	sync.Mutex
	dirs   map[string]projectEntry
	warned map[string]bool
}{dirs: map[string]projectEntry{}, warned: map[string]bool{}}

func stamp(path string) projectStamp {
	s := projectStamp{path: path}
	if info, err := os.Stat(path); err == nil {
		s.mod = info.ModTime()
		s.size = info.Size()
		s.mode = info.Mode()
	}
	return s
}

// projectConfig caches the complete search per directory, validating mtimes so
// adding a nearer file, editing, or removing the winner invalidates the entry.
func projectConfig(dir, home string) (projectFile, bool) {
	var empty projectFile
	if dir == "" || !filepath.IsAbs(dir) {
		return empty, false
	}
	info, err := os.Open(dir)
	if err != nil {
		return empty, false
	}
	_, err = info.Readdirnames(1)
	info.Close()
	if err != nil && err != io.EOF {
		return empty, false
	}
	dir = filepath.Clean(dir)
	home = filepath.Clean(home)
	key := home + "\x00" + dir
	projectCache.Lock()
	defer projectCache.Unlock()
	if old, ok := projectCache.dirs[key]; ok {
		valid := true
		for _, s := range old.stamps {
			if stamp(s.path) != s {
				valid = false
				break
			}
		}
		if valid {
			return old.config, old.invalid
		}
	}
	entry := projectEntry{}
	for current := dir; ; current = filepath.Dir(current) {
		path := filepath.Join(current, ".hev.toml")
		s := stamp(path)
		entry.stamps = append(entry.stamps, s)
		if !s.mod.IsZero() {
			md, err := toml.DecodeFile(path, &entry.config)
			if err != nil {
				entry.invalid = true
			}
			if (err != nil || len(md.Undecoded()) > 0) && !projectCache.warned[path] {
				// Do not print untrusted values (a forbidden key might contain a secret).
				log.Printf("warning: project config %s has invalid TOML or ignored keys", path)
				projectCache.warned[path] = true
			}
			break
		}
		if current == home || filepath.Dir(current) == current {
			break
		}
	}
	projectCache.dirs[key] = entry
	return entry.config, entry.invalid
}

func chooseRoute(workdir, namespace string, allowed []string, home string) Route {
	cfg, invalid := projectConfig(workdir, home)
	r := Route{Namespace: namespace}
	if invalid {
		r.Blocked = true
		return r
	}
	if cfg.Capture.Archive != nil && !*cfg.Capture.Archive {
		r.Skip = true
		return r
	}
	if cfg.Capture.Namespace == "" {
		return r
	}
	r.Namespace = cfg.Capture.Namespace
	r.Project = true
	r.Blocked = true
	for _, name := range allowed {
		if name == r.Namespace {
			r.Blocked = false
			break
		}
	}
	return r
}

func sessionRoute(st *State, cl *layer.Client, turns []trace.Turn, opt Options, legacy bool) Route {
	routeMu.Lock()
	defer routeMu.Unlock()
	namespace := ""
	if cl != nil {
		namespace = cl.Namespace
	}
	t := turns[0]
	key := namespace + "\x00" + t.Harness + "\x00" + t.SessionID
	if cl != nil {
		key = cl.Endpoint + "\x00" + key
	}
	if r, ok := st.Routes[key]; ok {
		if r.Project {
			r.Blocked = !namespaceAllowed(r.Namespace, opt.ProjectNamespaces)
		}
		recordRoute(st, key, t.SessionID, &r, opt.DryRun)
		return r
	}
	home, _ := os.UserHomeDir()
	workdir := ""
	for _, turn := range turns {
		if turn.Workdir != "" {
			workdir = turn.Workdir
			break
		}
	}
	r := Route{Namespace: namespace}
	if !legacy && !opt.existingSessions[t.SessionID] {
		r = chooseRoute(workdir, namespace, opt.ProjectNamespaces, home)
	}
	recordRoute(st, key, t.SessionID, &r, opt.DryRun)
	return r
}

func namespaceAllowed(namespace string, allowed []string) bool {
	for _, name := range allowed {
		if namespace == name {
			return true
		}
	}
	return false
}

// Caller holds routeMu. Warned persists so retries and restarts stay quiet.
func recordRoute(st *State, key, sessionID string, r *Route, dryRun bool) {
	if dryRun {
		return
	}
	if r.Blocked && !r.Warned {
		log.Printf("session %s: blocked by project config", sessionID)
		r.Warned = true
	}
	if st.Routes == nil {
		st.Routes = map[string]Route{}
	}
	st.Routes[key] = *r
}
