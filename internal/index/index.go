// Package index drives a trace source into a Layer namespace.
//
// The loop is deliberately dull: enumerate units, skip the ones whose signature
// has not moved, parse the rest, chunk them, and upsert in batches. All of the
// interesting decisions were made elsewhere — the turn model in internal/trace,
// the wire in internal/layer — and this is what holds them together.
package index

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// State records which units have been indexed and at what signature. It is a
// cache, not a record of truth: deleting it re-indexes everything, and because
// chunk ids are content-addressed that is a no-op upsert rather than a
// duplicate archive. That property is why this file can stay this simple.
type State struct {
	Units map[string]string `json:"units"` // unit key -> signature
}

func statePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".hev-index-state.json"
	}
	return filepath.Join(home, ".hev", "index-state.json")
}

// ResetState forgets which units were indexed, so the next run sends them all
// again. It is for when the archive those signatures describe is gone.
func ResetState() error {
	if err := os.Remove(statePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func LoadState() *State {
	s := &State{Units: map[string]string{}}
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	if json.Unmarshal(b, s) != nil || s.Units == nil {
		s.Units = map[string]string{}
	}
	return s
}

func (s *State) Save() error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// Options tune one run.
type Options struct {
	// Tiers to index. Empty means text and tool_use — the two that are prose
	// and intent. tool_result is 44% of the chunks in a real corpus and is the
	// least of what anyone searches for, so it is opt-in rather than the
	// default someone discovers on their bill.
	Tiers []trace.Tier
	// BatchRows caps rows per upsert.
	BatchRows int
	// Limit caps units processed, for a first look at a big tree.
	Limit int
	// Force ignores recorded signatures.
	Force bool
	// DryRun parses and chunks but writes nothing.
	DryRun bool
	// ReadSide writes only the blocks and sessions namespaces and leaves the
	// chunk namespace alone. A chunk write is an embedding, so a backfill of
	// the read side over an already-searchable archive should not pay for
	// one; pair it with Force, since an unchanged unit is otherwise skipped.
	ReadSide bool
	// Workers bounds concurrent read-side units; zero or one is sequential.
	Workers int
	// Attribution is applied to every row — the factory join, when the caller
	// knows it.
	Attribution func(t trace.Turn) (instance, plan, rfc, issue, pr string)
	// Progress is called once per unit.
	Progress func(done, total int, unit string)
	// RepoURL resolves the origin for a working directory. Tests can replace
	// the git lookup; nil uses `git remote get-url origin`.
	RepoURL func(workdir string) string
}

// Report is what a run did.
type Report struct {
	UnitsSeen           int
	UnitsIndexed        int
	UnitsSkipped        int
	Turns               int
	Chunks              int
	Blocks              int
	Sessions            int
	RowsUpserted        int
	BlockRowsUpserted   int
	SessionRowsUpserted int
	EmbeddingTokens     int
	Errors              []string
}

// Summarize reads every source unit and writes only session rows. Keeping this
// separate from Run avoids rewriting chunks and whole blocks merely to fill a
// scalar session attribute (and lets it bypass the unchanged-unit cache).
func Summarize(src trace.Source, cl *layer.Client, batchRows int, repoURL func(string) string, sessionIDs map[string]bool, summarize func(trace.SessionRow) (string, error), progress func(done, total int, unit string)) (*Report, error) {
	if batchRows <= 0 {
		batchRows = 200
	}
	if repoURL == nil {
		repoURL = cachedRepoURL()
	}
	units, err := src.Units()
	if err != nil {
		return nil, fmt.Errorf("enumerate units: %w", err)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	rep := &Report{UnitsSeen: len(units)}
	for i, u := range units {
		if progress != nil {
			progress(i+1, len(units), u.Key)
		}
		turns, err := src.Read(u)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", u.Key, err))
			continue
		}
		sessions := trace.Sessions(turns, repoURL, layer.Hostname())
		if sessionIDs != nil {
			selected := sessions[:0]
			for _, session := range sessions {
				if sessionIDs[session.SessionID] {
					selected = append(selected, session)
				}
			}
			sessions = selected
		}
		failed := false
		for i := range sessions {
			if strings.TrimSpace(sessions[i].Summary) != "" {
				continue
			}
			summary, err := summarize(sessions[i])
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s summary: %v", sessions[i].SessionID, err))
				failed = true
				continue
			}
			sessions[i].Summary = summary
		}
		for start := 0; start < len(sessions); start += batchRows {
			end := start + batchRows
			if end > len(sessions) {
				end = len(sessions)
			}
			res, err := cl.PatchSessionSummaries(sessions[start:end])
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s sessions: %v", u.Key, err))
				failed = true
				break
			}
			rep.SessionRowsUpserted += res.RowsUpserted
		}
		rep.Sessions += len(sessions)
		if !failed && len(sessions) > 0 {
			rep.UnitsIndexed++
		}
	}
	return rep, nil
}

// Run indexes a source into a namespace.
func Run(src trace.Source, cl *layer.Client, st *State, opt Options) (*Report, error) {
	if opt.Workers < 0 || opt.Workers > 8 || (opt.Workers > 1 && !opt.ReadSide) {
		return nil, fmt.Errorf("workers must be 0–8 and parallel workers require read-side mode")
	}
	if opt.BatchRows <= 0 {
		opt.BatchRows = 200
	}
	if len(opt.Tiers) == 0 {
		opt.Tiers = []trace.Tier{trace.TierText, trace.TierToolUse}
	}
	want := map[string]bool{}
	for _, t := range opt.Tiers {
		want[t.String()] = true
	}
	if opt.RepoURL == nil {
		opt.RepoURL = cachedRepoURL()
	}

	units, err := src.Units()
	if err != nil {
		return nil, fmt.Errorf("enumerate units: %w", err)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	if opt.Limit > 0 && len(units) > opt.Limit {
		units = units[:opt.Limit]
	}

	if opt.Workers > 1 {
		return runReadSideWorkers(src, cl, st, opt, units)
	}
	rep := &Report{UnitsSeen: len(units)}
	for i, u := range units {
		if opt.Progress != nil {
			opt.Progress(i+1, len(units), u.Key)
		}
		if !opt.Force && u.Signature != "" && st.Units[u.Key] == u.Signature {
			rep.UnitsSkipped++
			continue
		}

		turns, err := src.Read(u)
		if err != nil {
			// One unreadable transcript must not sink a run over thousands.
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", u.Key, err))
			continue
		}

		var batch []layer.Row
		flush := func() error {
			if len(batch) == 0 || opt.DryRun || opt.ReadSide {
				batch = batch[:0]
				return nil
			}
			res, err := cl.Write(batch)
			if err != nil {
				return err
			}
			rep.RowsUpserted += res.RowsUpserted
			rep.EmbeddingTokens += res.EmbeddingTokens
			batch = batch[:0]
			return nil
		}

		failed := false
		// Content-addressed ids collapse identical blocks in the same turn
		// (two copies of one image placeholder, say) to a single row. That is
		// the intended dedup, but turbopuffer rejects an upsert that carries
		// the same id twice, which dropped the whole unit, so keep the first.
		seen := map[string]bool{}
		for _, t := range turns {
			rep.Turns++
			if opt.ReadSide {
				continue
			}
			for _, c := range trace.Chunks(t) {
				if !want[c.Tier] || seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				rep.Chunks++
				row := layer.RowOf(c)
				if opt.Attribution != nil {
					row.Instance, row.Plan, row.RFC, row.Issue, row.PR = opt.Attribution(t)
				}
				batch = append(batch, row)
				if len(batch) >= opt.BatchRows {
					if err := flush(); err != nil {
						rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", u.Key, err))
						failed = true
						break
					}
				}
			}
			if failed {
				break
			}
		}
		if !failed {
			if err := flush(); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", u.Key, err))
				failed = true
			}
		}
		if !failed {
			blocks := trace.Blocks(turns)
			rep.Blocks += len(blocks)
			for start := 0; start < len(blocks); start += opt.BatchRows {
				end := start + opt.BatchRows
				if end > len(blocks) {
					end = len(blocks)
				}
				if opt.DryRun {
					continue
				}
				res, err := cl.WriteBlocks(blocks[start:end])
				if err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s blocks: %v", u.Key, err))
					failed = true
					break
				}
				rep.BlockRowsUpserted += res.RowsUpserted
			}
		}
		if !failed {
			sessions := trace.Sessions(turns, opt.RepoURL, layer.Hostname())
			rep.Sessions += len(sessions)
			for start := 0; start < len(sessions); start += opt.BatchRows {
				end := start + opt.BatchRows
				if end > len(sessions) {
					end = len(sessions)
				}
				if opt.DryRun {
					continue
				}
				res, err := cl.WriteSessions(sessions[start:end])
				if err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s sessions: %v", u.Key, err))
					failed = true
					break
				}
				rep.SessionRowsUpserted += res.RowsUpserted
			}
		}

		// A unit is recorded only when all of it landed. A partially written
		// unit stays unrecorded and is retried next run, where the
		// content-addressed ids make the rows that did land free to rewrite.
		if !failed {
			rep.UnitsIndexed++
			if u.Signature != "" && !opt.DryRun {
				st.Units[u.Key] = u.Signature
			}
		}
	}
	return rep, nil
}

func cachedRepoURL() func(string) string {
	cache := map[string]string{}
	return func(workdir string) string {
		if url, ok := cache[workdir]; ok {
			return url
		}
		if workdir == "" {
			return ""
		}
		out, err := exec.Command("git", "-C", workdir, "remote", "get-url", "origin").Output()
		if err != nil {
			cache[workdir] = ""
			return ""
		}
		url := strings.TrimSpace(string(out))
		cache[workdir] = url
		return url
	}
}
