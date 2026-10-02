package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type backfillCheckpoint struct {
	Namespace string `json:"namespace"`
	Identity  string `json:"identity"`
	Since     int64  `json:"since"`
	After     string `json:"after"`
	Complete  bool   `json:"complete"`
	Processed int    `json:"processed"`
}

func saveBackfill(path string, cp backfillCheckpoint) error {
	b, e := json.Marshal(cp)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".backfill-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func init() {
	var namespace, checkpoint, manifest, cohort string
	var apply bool
	var limit, days, retries int
	cmd := &cobra.Command{Use: "backfill", Short: "Resume bounded session linkage and outcome enrichment", RunE: func(cmd *cobra.Command, args []string) error {
		if checkpoint == "" || limit < 1 || limit > 1000 || days < 1 || days > 36500 || retries < 0 || retries > 5 {
			return fmt.Errorf("checkpoint required; limit 1..1000, days 1..36500, retries 0..5")
		}
		cl, e := client(namespace)
		if e != nil {
			return e
		}
		cp := backfillCheckpoint{Namespace: cl.Namespace, Since: time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()}
		if b, err := os.ReadFile(checkpoint); err == nil {
			if e = json.Unmarshal(b, &cp); e != nil {
				return e
			}
			if cp.Namespace != cl.Namespace {
				return fmt.Errorf("checkpoint namespace mismatch")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		sources := map[string]trace.SessionRow{}
		if manifest != "" {
			b, err := os.ReadFile(manifest)
			if err != nil {
				return err
			}
			if e = json.Unmarshal(b, &sources); e != nil {
				return e
			}
		}
		var frozen []trace.SessionRow
		var rawCohort []byte
		if cohort != "" {
			rawCohort, e = os.ReadFile(cohort)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(rawCohort, &frozen); e != nil {
				return e
			}
			sort.Slice(frozen, func(i, j int) bool { return frozen[i].ID < frozen[j].ID })
			for i, r := range frozen {
				if r.ID == "" || (i > 0 && frozen[i-1].ID == r.ID) {
					return fmt.Errorf("invalid/duplicate cohort ID")
				}
			}
		}
		if apply && cohort == "" {
			return fmt.Errorf("apply requires immutable --cohort export and coordinated archive writers")
		}
		sourceBytes, _ := json.Marshal(sources)
		identity := fmt.Sprintf("%x", sha256.Sum256(append(append([]byte(cl.Endpoint+"\n"+cl.Namespace+"\nbackfill-v1\n"), rawCohort...), sourceBytes...)))
		if cp.Identity != "" && cp.Identity != identity {
			return fmt.Errorf("checkpoint target/cohort/linkage mismatch")
		}
		cp.Identity = identity
		if cp.Complete {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(cp)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()
		var rows []trace.SessionRow
		if cohort != "" {
			for _, r := range frozen {
				if r.ID > cp.After && r.End >= cp.Since {
					rows = append(rows, r)
					if len(rows) == limit {
						break
					}
				}
			}
		} else {
			rows, e = cl.OutcomePage(ctx, cp.After, cp.Since, limit)
			if e != nil {
				return e
			}
		}
		if apply && cp.Processed == 0 {
			if e = saveBackfill(checkpoint, cp); e != nil {
				return e
			}
		}
		git := index.NewGitEnricher()
		outcomes := index.NewOutcomeEnricher()
		for _, row := range rows {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if row.ID <= cp.After {
				return fmt.Errorf("nonadvancing cursor")
			}
			if src, ok := sources[row.ID]; ok {
				if row.Workdir == "" {
					row.Workdir = src.Workdir
				}
				if row.RepoURL == "" {
					row.RepoURL = src.RepoURL
				}
				if row.Branch == "" {
					row.Branch = src.Branch
				}
				if row.PR == "" {
					row.PR = src.PR
				}
				row.Commits = append(row.Commits, src.Commits...)
			}
			git.Enrich(ctx, &row, nil)
			outcomes.Enrich(ctx, &row)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if apply {
				for attempt := 0; attempt <= retries; attempt++ {
					e = cl.PatchBackfill(ctx, row)
					if e == nil {
						break
					}
					if attempt == retries {
						return fmt.Errorf("cursor %q remains safe: %w", cp.After, e)
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Duration(1<<attempt) * time.Second):
					}
				}
				cp.After = row.ID
				cp.Processed++
				if e = saveBackfill(checkpoint, cp); e != nil {
					return e
				}
			}
			if e = json.NewEncoder(cmd.OutOrStdout()).Encode(row); e != nil {
				return e
			}
		}
		if apply {
			cp.Complete = len(rows) < limit
			if e = saveBackfill(checkpoint, cp); e != nil {
				return e
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(cp)
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "configured archive namespace")
	cmd.Flags().StringVar(&checkpoint, "checkpoint", "", "private durable checkpoint file")
	cmd.Flags().StringVar(&cohort, "cohort", "", "immutable private JSON array of exported sessions (required for apply)")
	cmd.Flags().StringVar(&manifest, "linkage", "", "private JSON map of archive row IDs to verified source linkage")
	cmd.Flags().BoolVar(&apply, "apply", false, "patch agreed archive; default reports without writes")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum rows this invocation")
	cmd.Flags().IntVar(&days, "days", 60, "fixed lookback on checkpoint creation")
	cmd.Flags().IntVar(&retries, "retries", 2, "bounded patch retries")
	rootCmd.AddCommand(cmd)
}
