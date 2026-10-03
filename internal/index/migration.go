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

func Run(src trace.Source, cl *layer.Client, st *State, opt Options) (*Report, error) {
	if opt.Workers < 0 || opt.Workers > 8 || (opt.Workers > 1 && !opt.ReadSide) {
		return nil, fmt.Errorf("workers must be 0–8 and parallel workers require read-side mode")
	}

	root, _ := sourceScope(src)
	if root == "" || opt.DryRun {
		return run(src, cl, st, opt)
	}
	unlock, err := redact.LockArchive()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if !opt.MigrateArchive {
		pending, err := archiveMigrationPending(cl, root)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			return &Report{MigrationPending: pending}, nil
		}
		scoped := scopedState(st, cl, root)
		if pending {
			// Legacy signatures describe this already-configured archive. Keep
			// unchanged sources skipped rather than silently rebuilding them.
			units, err := src.Units()
			if err != nil {
				return nil, err
			}
			for _, unit := range units {
				if _, exists := scoped.Units[unit.Key]; !exists {
					if sig, exists := st.Units[unit.Key]; exists {
						scoped.Units[unit.Key] = sig
					}
				}
			}
		}
		rep, err := run(src, cl, scoped, opt)
		mergeScopedState(st, scoped, cl, root)
		if rep != nil {
			rep.MigrationPending = pending
		}
		return rep, err
	}
	rep, migrated, err := migrateArchive(src, cl, st, opt)
	if err != nil {
		return rep, err
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

func Summarize(src trace.Source, cl *layer.Client, batchRows int, repoURL func(string) string, sessionIDs map[string]bool, summarize func(trace.SessionRow) (string, error), progress func(done, total int, unit string), allowSourceLoss bool) (*Report, error) {
	root, _ := sourceScope(src)
	if root == "" {
		return summarizeSource(src, cl, batchRows, repoURL, sessionIDs, summarize, progress)
	}
	unlock, err := redact.LockArchive()
	if err != nil {
		return nil, err
	}
	defer unlock()
	upgrade, _, err := migrateArchive(src, cl, &State{Units: map[string]string{}}, Options{BatchRows: batchRows, RepoURL: repoURL, Progress: progress, AllowSourceLoss: allowSourceLoss})
	if err != nil {
		return nil, err
	}
	rep, err := summarizeSource(src, cl, batchRows, repoURL, sessionIDs, summarize, progress)
	if rep != nil && upgrade != nil {
		rep.Redactions.Add(upgrade.Redactions)
		rep.RowsUpserted += upgrade.RowsUpserted
		rep.BlockRowsUpserted += upgrade.BlockRowsUpserted
		rep.EmbeddingTokens += upgrade.EmbeddingTokens
		rep.RemovedMissingSessions = upgrade.RemovedMissingSessions
	}
	return rep, err
}

func archiveKey(cl *layer.Client, root string) string {
	root, _ = filepath.Abs(root)
	sum := sha256.Sum256([]byte(strings.TrimRight(cl.Endpoint, "/") + "\x00" + cl.Namespace + "\x00" + cl.APIKey + "\x00" + cl.Model + "\x00" + cl.Caps.Store.Kind + "\x00" + layer.Hostname() + "\x00" + root))
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
	if err != nil {
		if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
			return nil, false, err
		}
		units = nil
	}
	snapshot := migrationSource{units: units, turns: map[string][]trace.Turn{}}
	selected := map[string]bool{}
	available := map[string]bool{}
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
			available[turn.SessionID] = true
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
		p, pathErr := filepath.Abs(row.SourcePath)
		if pathErr != nil {
			return nil, false, pathErr
		}
		if (row.Host == layer.Hostname() || row.Host == "") && (row.Harness == harness || row.Harness == "") && (p == root || strings.HasPrefix(p, root+string(os.PathSeparator))) {
			selected[row.SessionID] = true
		}
	}

	for _, row := range chunks {
		p, pathErr := filepath.Abs(row.SourcePath)
		if pathErr != nil {
			return nil, false, pathErr
		}
		inScope := (row.Host == layer.Hostname() || row.Host == "") && (row.Harness == harness || row.Harness == "") && (p == root || strings.HasPrefix(p, root+string(os.PathSeparator)))
		if selected[row.SessionID] && row.SourcePath != "" && !inScope {
			return nil, false, fmt.Errorf("session %s also belongs to another archive source; refusing shared-session deletion", row.SessionID)
		}
		if row.SourcePath == "" && !selected[row.SessionID] && (row.Host == layer.Hostname() || row.Host == "") && (row.Harness == harness || row.Harness == "") {
			return nil, false, fmt.Errorf("cannot attribute orphan chunks for session %s; restore source or explicitly remove its archive rows", row.SessionID)
		}
	}
	sessions, err := cl.ArchiveIdentityRows("-sessions")
	if err != nil {
		return nil, false, err
	}
	for _, row := range sessions {
		if !selected[row.SessionID] && !owners[row.SessionID] && (row.Host == layer.Hostname() || row.Host == "") && (row.Harness == harness || row.Harness == "") {
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
	var lost []string
	for id := range selected {
		if !available[id] {
			lost = append(lost, id)
		}
	}
	sort.Strings(lost)
	if len(lost) > 0 && !opt.AllowSourceLoss {
		// Nothing has been deleted or journaled yet; the rows stay unscrubbed.
		return nil, false, fmt.Errorf("archive migration would permanently remove %d session(s) whose source transcripts are missing; restore the sources or pass --allow-source-loss to accept the loss", len(lost))
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
	for _, id := range state.Sessions {
		if !available[id] {
			rep.RemovedMissingSessions = append(rep.RemovedMissingSessions, id)
		}
	}
	mergeScopedState(st, rebuilt, cl, root)
	state.Complete = true
	if err := saveMigration(path, state); err != nil {
		return rep, true, err
	}
	return rep, true, nil
}

// archiveMigrationPending observes the journal without creating or acknowledging
// one. A daemon is not authorization to delete and rebuild historical rows.
func archiveMigrationPending(cl *layer.Client, root string) (bool, error) {
	scrubber, err := redact.Load()
	if err != nil {
		return false, err
	}
	if scrubber == nil {
		return false, nil
	}
	config, err := redact.ConfigPath()
	if err != nil {
		return false, err
	}
	identity := archiveKey(cl, root)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(config), "archive-redaction-"+identity+".json"))
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var state migrationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return false, fmt.Errorf("read migration journal: %w", err)
	}
	if state.Identity != identity {
		return false, fmt.Errorf("archive migration identity mismatch")
	}
	return !state.Complete || state.Policy != redact.Version+":"+scrubber.Identity(), nil
}
