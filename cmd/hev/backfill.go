package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"time"
)

type backfillCheckpoint struct {
	Namespace string `json:"namespace"`
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
	var namespace, checkpoint, manifest string
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
		if cp.Complete {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(cp)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()
		rows, e := cl.OutcomePage(ctx, cp.After, cp.Since, limit)
		if e != nil {
			return e
		}
		git := index.NewGitEnricher()
		outcomes := index.NewOutcomeEnricher()
		for _, row := range rows {
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
	cmd.Flags().StringVar(&manifest, "linkage", "", "private JSON map of archive row IDs to verified source linkage")
	cmd.Flags().BoolVar(&apply, "apply", false, "patch agreed archive; default reports without writes")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum rows this invocation")
	cmd.Flags().IntVar(&days, "days", 60, "fixed lookback on checkpoint creation")
	cmd.Flags().IntVar(&retries, "retries", 2, "bounded patch retries")
	rootCmd.AddCommand(cmd)
}
