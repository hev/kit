package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
	"golang.org/x/sys/unix"
)

type migrationState struct {
	Identity string   `json:"identity"`
	Policy   string   `json:"policy"`
	Sessions []string `json:"sessions"`
	Complete bool     `json:"complete"`
}

func sourceScope(src trace.Source) (root, harness string) {
	switch s := src.(type) {
	case *trace.ClaudeSource:
		return s.Root, "claude_code"
	case *trace.CodexSource:
		return s.Root, "codex"
	}
	return "", ""
}

// Lock all current-version writers for the whole migration/write interval.
// Older binaries must be stopped before upgrading; they do not honor this lock.
func archiveLock() (func(), error) {
	config, err := redact.ConfigPath()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		return nil, err
	}
	fd, err := unix.Open(config+".archive.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(fd, unix.LOCK_EX); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}

func Run(src trace.Source, cl *layer.Client, st *State, opt Options) (*Report, error) {
	root, _ := sourceScope(src)
	if root == "" || opt.DryRun {
		return run(src, cl, st, opt)
	}
	unlock, err := archiveLock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	rep, migrated, err := migrateArchive(src, cl, st, opt)
	if err != nil {
		return nil, err
	}
	if migrated {
		return rep, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return &Report{}, nil
	}
	// Scope signatures to the archive, too: a different endpoint/namespace must
	// not inherit another archive's unchanged-unit cache.
	scoped := scopedState(st, cl, root)
	rep, err = run(src, cl, scoped, opt)
	mergeScopedState(st, scoped, cl, root)
	return rep, err
}

func Summarize(src trace.Source, cl *layer.Client, batchRows int, repoURL func(string) string, sessionIDs map[string]bool, summarize func(trace.SessionRow) (string, error), progress func(done, total int, unit string)) (*Report, error) {
	root, _ := sourceScope(src)
	if root == "" {
		return summarizeSource(src, cl, batchRows, repoURL, sessionIDs, summarize, progress)
	}
	unlock, err := archiveLock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	_, _, err = migrateArchive(src, cl, &State{Units: map[string]string{}}, Options{BatchRows: batchRows, RepoURL: repoURL, Progress: progress})
	if err != nil {
		return nil, err
	}
	return summarizeSource(src, cl, batchRows, repoURL, sessionIDs, summarize, progress)
}

func archiveKey(cl *layer.Client, root string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(cl.Endpoint, "/") + "\x00" + cl.Namespace + "\x00" + cl.Caps.Store.Kind + "\x00" + layer.Hostname() + "\x00" + root))
	return hex.EncodeToString(sum[:])
}
func scopedState(st *State, cl *layer.Client, root string) *State {
	out := &State{Units: map[string]string{}}
	prefix := archiveKey(cl, root) + ":"
	for key, sig := range st.Units {
		if strings.HasPrefix(key, prefix) {
			out.Units[strings.TrimPrefix(key, prefix)] = sig
		}
	}
	return out
}
func mergeScopedState(st, scoped *State, cl *layer.Client, root string) {
	if st.Units == nil {
		st.Units = map[string]string{}
	}
	for key, sig := range scoped.Units {
		st.Units[archiveKey(cl, root)+":"+key] = sig
	}
}

func saveMigration(path string, state migrationState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".archive-migration-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

type migrationSource struct {
	units []trace.Unit
	turns map[string][]trace.Turn
}

func (s migrationSource) Describe() string                        { return "upgrade snapshot" }
func (s migrationSource) Units() ([]trace.Unit, error)            { return s.units, nil }
func (s migrationSource) Read(u trace.Unit) ([]trace.Turn, error) { return s.turns[u.Key], nil }

func migrateArchive(src trace.Source, cl *layer.Client, st *State, opt Options) (*Report, bool, error) {
	root, harness := sourceScope(src)
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, false, err
	}
	scrubber, err := redact.Load()
	if err != nil {
		return nil, false, err
	}
	config, err := redact.ConfigPath()
	if err != nil {
		return nil, false, err
	}
	identity := archiveKey(cl, root)
	path := filepath.Join(filepath.Dir(config), "archive-redaction-"+identity+".json")
	state := migrationState{Identity: identity}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(raw, &state); err != nil {
			return nil, false, fmt.Errorf("read migration journal: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}
	if state.Identity != identity {
		return nil, false, fmt.Errorf("archive migration identity mismatch")
	}
	if scrubber == nil {
		// An opted-out write invalidates completion, including after a previous
		// successful migration. Preserve pending ownership coordinates for retries.
		state.Complete = false
		return nil, false, saveMigration(path, state)
	}
	policy := redact.Version + ":" + scrubber.Identity()
	if state.Complete && state.Policy == policy {
		return nil, false, nil
	}
	state.Complete = false
	state.Policy = policy
	units, err := src.Units()
	if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	snapshot := migrationSource{units: units, turns: map[string][]trace.Turn{}}
	selected := map[string]bool{}
	for _, id := range state.Sessions {
		selected[id] = true
	}
	// Parse everything before cleanup. Unreadable existing sources fail closed;
	// genuinely missing transcripts are removed rather than retaining secrets.
	for _, u := range units {
		turns, err := src.Read(u)
		if err != nil {
			return nil, false, fmt.Errorf("migration source %s: %w", u.Key, err)
		}
		snapshot.turns[u.Key] = turns
		for _, turn := range turns {
			selected[turn.SessionID] = true
		}
	}
	owners := map[string]bool{}
	chunks, err := cl.ArchiveIdentityRows("")
	if err != nil {
		return nil, false, err
	}
	for _, row := range chunks {
		if row.SourcePath == "" {
			continue
		}
		owners[row.SessionID] = true
		p := filepath.Clean(row.SourcePath)
		if (row.Host == layer.Hostname() || row.Host == "") && row.Harness == harness && (p == root || strings.HasPrefix(p, root+string(os.PathSeparator))) {
			selected[row.SessionID] = true
		}
	}

	for _, row := range chunks {
		p := filepath.Clean(row.SourcePath)
		inScope := (row.Host == layer.Hostname() || row.Host == "") && row.Harness == harness && (p == root || strings.HasPrefix(p, root+string(os.PathSeparator)))
		if selected[row.SessionID] && row.SourcePath != "" && !inScope {
			return nil, false, fmt.Errorf("session %s also belongs to another archive source; refusing shared-session deletion", row.SessionID)
		}
		if row.SourcePath == "" && !selected[row.SessionID] && (row.Host == layer.Hostname() || row.Host == "") && row.Harness == harness {
			return nil, false, fmt.Errorf("cannot attribute orphan chunks for session %s; restore source or explicitly remove its archive rows", row.SessionID)
		}
	}
	sessions, err := cl.ArchiveIdentityRows("-sessions")
	if err != nil {
		return nil, false, err
	}
	for _, row := range sessions {
		if !selected[row.SessionID] && !owners[row.SessionID] && (row.Host == layer.Hostname() || row.Host == "") && row.Harness == harness {
			return nil, false, fmt.Errorf("cannot attribute orphan session %s to source root %s; restore its source or explicitly remove its archive rows; migration is incomplete", row.SessionID, root)
		}
	}
	blocks, err := cl.ArchiveIdentityRows("-blocks")
	if err != nil {
		return nil, false, err
	}
	for _, row := range blocks {
		if !selected[row.SessionID] && !owners[row.SessionID] {
			known := false
			for _, session := range sessions {
				if session.SessionID == row.SessionID {
					known = true
					break
				}
			}
			if !known {
				return nil, false, fmt.Errorf("cannot attribute orphan blocks for session %s; restore source or explicitly remove its archive rows", row.SessionID)
			}
		}
	}
	state.Sessions = nil
	for id := range selected {
		if id == "" {
			return nil, false, fmt.Errorf("migration encountered empty session identity")
		}
		state.Sessions = append(state.Sessions, id)
	}
	sort.Strings(state.Sessions)
	// Durable journal precedes any destructive request. Interrupted retries delete
	// these IDs again even when cleanup removed their only ownership coordinates.
	if err := saveMigration(path, state); err != nil {
		return nil, false, err
	}
	for _, id := range state.Sessions {
		if err := cl.DeleteSessionArchive(id); err != nil {
			return nil, false, fmt.Errorf("cleanup session %s: %w", id, err)
		}
	}
	// First upgrade always rebuilds every tier and both read sides, ignoring
	// signatures, limits, read-side-only mode and parallelism.
	opt.Force, opt.ReadSide, opt.Workers, opt.Limit = true, false, 0, 0
	opt.Tiers = trace.AllTiers[:]
	rebuilt := &State{Units: map[string]string{}}
	rep, err := run(snapshot, cl, rebuilt, opt)
	if err != nil {
		return nil, false, err
	}
	if len(rep.Errors) != 0 {
		return rep, true, fmt.Errorf("archive redaction incomplete: %s", strings.Join(rep.Errors, "; "))
	}
	mergeScopedState(st, rebuilt, cl, root)
	state.Complete = true
	if err := saveMigration(path, state); err != nil {
		return rep, true, err
	}
	return rep, true, nil
}
