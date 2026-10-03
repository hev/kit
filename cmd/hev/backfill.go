package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"time"
)

type backfillCheckpoint struct {
	Namespace string `json:"namespace"`
	Identity  string `json:"identity"`
	Since     int64  `json:"since"`
	Until     int64  `json:"until"`
	Mode      string `json:"mode"`
	After     string `json:"after"`
	Complete  bool   `json:"complete"`
	Processed int    `json:"processed"`
}

func saveBackfill(path string, cp backfillCheckpoint) error { return saveBackfillJSON(path, cp) }
func saveBackfillJSON(path string, v any) error {
	b, e := json.Marshal(v)
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
	if e = os.Rename(name, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}

func lockBackfill(path string) (func(), error) {
	fd, err := unix.Open(path+".lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("backfill journal busy: %w", err)
	}
	return func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }, nil
}

// Tests can inject an acknowledged writer; production uses conditional patches.
type backfillRuntime struct {
	Client func(string) (*layer.Client, error)
	Enrich func(context.Context, *trace.SessionRow)
	Patch  func(context.Context, trace.SessionRow) error
	Save   func(string, backfillCheckpoint) error
}

func init() {
	git := index.NewGitEnricher()
	outcomes := index.NewOutcomeEnricher()
	// Reuse identical successful GitHub observations within a bounded invocation.
	// Git commands remain session/time-specific; failures are never cached as success.
	run := git.Run
	cache := map[string][]byte{}
	cachedRun := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if name != "gh" {
			return run(ctx, dir, name, args...)
		}
		key := backfillHash(args)
		if b, ok := cache[key]; ok {
			return append([]byte(nil), b...), nil
		}
		b, err := run(ctx, dir, name, args...)
		if err == nil {
			cache[key] = append([]byte(nil), b...)
		}
		return b, err
	}
	git.Run = cachedRun
	outcomes.Run = cachedRun

	rootCmd.AddCommand(newBackfillCommand(backfillRuntime{Client: client, Save: saveBackfill, Enrich: func(ctx context.Context, r *trace.SessionRow) { git.Enrich(ctx, r, nil); outcomes.Enrich(ctx, r) }}))
}

func newBackfillCommand(rt backfillRuntime) *cobra.Command {
	var namespace, checkpoint, linkage, cohort, export, account, sinceText, untilText, against, provenance string
	var apply, filterPlan, repairFilters bool
	var limit, retries int
	cmd := &cobra.Command{Use: "backfill", Short: "Export, preview and reconcile bounded session enrichment", RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || limit > 1000 || retries < 0 || retries > 5 {
			return fmt.Errorf("limit 1..1000, retries 0..5")
		}

		if against != "" {
			before, e := readBackfillManifest(cohort)
			if e != nil {
				report := backfillReconciliation{BaselineState: "not_compared"}
				if cohort == "" || os.IsNotExist(e) {
					report.BaselineState = "absent"
				}
				if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(report); encodeErr != nil {
					return encodeErr
				}
				return fmt.Errorf("baseline unavailable; preservation not verified: %w", e)
			}
			after, e := readBackfillManifest(against)
			if e != nil {
				return e
			}
			report, e := reconcileBackfill(before, after)
			if e != nil {
				return e
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		}
		if account == "" {
			return fmt.Errorf("--account requires the archive owner's nonsecret account identity; this declaration is not authenticated identity proof")
		}
		cl, e := rt.Client(namespace)
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()
		if filterPlan || repairFilters {
			plan, e := cl.SessionEnrichmentFilterPlan(ctx)
			if e != nil {
				return e
			}
			if repairFilters {
				if e = cl.ApplySessionEnrichmentFilterPlan(ctx); e != nil {
					return e
				}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
		}
		schema, e := cl.BackfillSchema(ctx)
		if e != nil {
			return e
		}
		target := backfillTarget{Endpoint: cl.Endpoint, Namespace: cl.Namespace, Store: cl.Caps.Store.Kind, Account: account, Schema: schema}
		var declared *backfillProvenance
		if provenance != "" {
			b, err := os.ReadFile(provenance)
			if err != nil {
				return err
			}
			if e = json.Unmarshal(b, &declared); e != nil {
				return e
			}
			if declared == nil {
				return fmt.Errorf("provenance must be an object")
			}
			if declared.EvidenceStatus != "declared-unverified" || declared.PublisherInventoryComplete {
				return fmt.Errorf("provenance must disclose unverified/incomplete publisher coverage")
			}
		}
		if export != "" {
			unlock, e := lockBackfill(export)
			if e != nil {
				return e
			}
			defer unlock()
			m := backfillManifest{Format: backfillFormat, Policy: backfillPolicy, Target: target, Provenance: declared}
			b, err := os.ReadFile(export)
			if err == nil {
				if e = json.Unmarshal(b, &m); e != nil {
					return e
				}
			} else if !os.IsNotExist(err) {
				return err
			} else {
				since, e := time.Parse(time.RFC3339, sinceText)
				if e != nil {
					return fmt.Errorf("export requires fixed --since RFC3339 UTC: %w", e)
				}
				until, e := time.Parse(time.RFC3339, untilText)
				if e != nil {
					return fmt.Errorf("export requires fixed --until RFC3339 UTC: %w", e)
				}
				m.Since = since.UnixMilli()
				m.Until = until.UnixMilli()
			}
			if e = m.validate(); e != nil {
				return e
			}
			if backfillHash(m.Provenance) != backfillHash(declared) {
				return fmt.Errorf("export provenance changed")
			}
			if backfillHash(m.Target) != backfillHash(target) {
				return fmt.Errorf("export target/schema/store/account changed")
			}
			for text, bound := range map[string]int64{sinceText: m.Since, untilText: m.Until} {
				if text != "" {
					t, e := time.Parse(time.RFC3339, text)
					if e != nil || t.UnixMilli() != bound {
						return fmt.Errorf("export selection changed")
					}
				}
			}
			if !m.Complete {
				rows, e := cl.BackfillPage(ctx, m.After, m.Since, m.Until, limit)
				if e != nil {
					return e
				}
				for _, row := range rows {
					if row.ID <= m.After {
						return fmt.Errorf("export cursor did not advance")
					}
					m.Rows = append(m.Rows, row)
					m.After = row.ID
				}
				m.Complete = len(rows) < limit
				m.seal()
				if e = m.validate(); e != nil {
					return e
				}
				if e = saveBackfillJSON(export, m); e != nil {
					return e
				}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"manifest": export, "hash": backfillHash(m), "rows": len(m.Rows), "next": m.After, "complete": m.Complete, "snapshot_consistent": false})
		}
		if checkpoint == "" || cohort == "" {
			return fmt.Errorf("preview requires --checkpoint and versioned --cohort; create with --export first")
		}
		m, e := readBackfillManifest(cohort)
		if e != nil {
			return e
		}
		if !m.Complete {
			return fmt.Errorf("cohort export incomplete")
		}
		if provenance != "" && backfillHash(m.Provenance) != backfillHash(declared) {
			return fmt.Errorf("cohort provenance mismatch")
		}

		if apply {
			if e = validateBackfillSchemaGrowth(m.Target.Schema, target.Schema); e != nil {
				return e
			}
			target.Schema = m.Target.Schema
		}
		if backfillHash(m.Target) != backfillHash(target) {
			return fmt.Errorf("cohort target/schema/store/account changed")
		}
		sources := map[string]trace.SessionRow{}
		if linkage != "" {
			b, e := os.ReadFile(linkage)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(b, &sources); e != nil {
				return e
			}
		}
		identity := backfillHash(struct {
			Manifest backfillManifest
			Linkage  map[string]trace.SessionRow
		}{m, sources})
		mode := "preview"
		if apply {
			mode = "apply"
		}
		unlock, e := lockBackfill(checkpoint)
		if e != nil {
			return e
		}
		defer unlock()
		cp := backfillCheckpoint{Namespace: m.Target.Namespace, Identity: identity, Since: m.Since, Until: m.Until, Mode: mode}
		if b, err := os.ReadFile(checkpoint); err == nil {
			if e = json.Unmarshal(b, &cp); e != nil {
				return e
			}
			if cp.Identity != identity || cp.Mode != mode || cp.Namespace != m.Target.Namespace || cp.Since != m.Since || cp.Until != m.Until {
				return fmt.Errorf("checkpoint target/cohort/linkage/schema/store/account/policy/selection/mode mismatch")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if rt.Save == nil {
			rt.Save = saveBackfill
		}
		if cp.Complete {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(cp)
		}
		if e = rt.Save(checkpoint, cp); e != nil {
			return e
		}

		processed := 0
		pending := []trace.SessionRow{}
		acknowledge := func(row trace.SessionRow) error {
			if e := json.NewEncoder(cmd.OutOrStdout()).Encode(row); e != nil {
				return e
			}
			cp.After = row.ID
			cp.Processed++
			return rt.Save(checkpoint, cp)
		}
		flush := func() error {
			if len(pending) == 0 {
				return nil
			}
			var err error
			for attempt := 0; attempt <= retries; attempt++ {
				err = cl.PatchUnknownBackfill(ctx, pending)
				if err == nil {
					break
				}
				if attempt < retries {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Duration(1<<attempt) * time.Second):
					}
				}
			}
			if err != nil {
				return fmt.Errorf("batch cursor %q remains safe: %w", cp.After, err)
			}
			for _, r := range pending {
				if err = acknowledge(r); err != nil {
					return err
				}
			}
			pending = nil
			return nil
		}
		for _, row := range m.Rows {
			if row.ID <= cp.After {
				continue
			}
			if processed == limit {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if apply && rt.Patch == nil {
				_, hasSource := sources[row.ID]
				if hasSource || row.PR != "" || len(row.Commits) > 0 {
					current, err := cl.CurrentBackfillRow(ctx, row.ID)
					if err != nil {
						return err
					}
					row = current
				}
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
			if rt.Enrich != nil {
				rt.Enrich(ctx, &row)
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if apply && rt.Patch == nil {
				_, hasSource := sources[row.ID]
				if !hasSource && len(row.Commits) == 0 && row.PR == "" && row.PRState == "unknown" && row.CIState == "unknown" && row.Reverted == "unknown" && len(row.WorkflowAttributes) == 0 {
					pending = append(pending, row)
					processed++
					if len(pending) == 1000 {
						if e = flush(); e != nil {
							return e
						}
					}
					continue
				}
				if e = flush(); e != nil {
					return e
				}
			}
			if apply {
				for attempt := 0; attempt <= retries; attempt++ {
					if rt.Patch != nil {
						e = rt.Patch(ctx, row)
					} else {
						e = cl.PatchBackfill(ctx, row)
					}
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
			}
			if e = acknowledge(row); e != nil {
				return e
			}
			processed++
		}
		if e = flush(); e != nil {
			return e
		}
		cp.Complete = len(m.Rows) == 0 || cp.After == m.Rows[len(m.Rows)-1].ID
		if e = rt.Save(checkpoint, cp); e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(cp)
	}}
	cmd.Flags().StringVar(&provenance, "provenance", "", "private declared revision/publisher evidence; never fence proof")
	cmd.Flags().StringVar(&namespace, "namespace", "", "configured archive namespace")
	cmd.Flags().StringVar(&account, "account", "", "owner-supplied nonsecret account identity")
	cmd.Flags().StringVar(&checkpoint, "checkpoint", "", "private durable preview/apply checkpoint")
	cmd.Flags().StringVar(&cohort, "cohort", "", "versioned private complete export manifest")
	cmd.Flags().StringVar(&export, "export", "", "create/resume private bounded source export")
	cmd.Flags().StringVar(&against, "against", "", "compare second complete export with --cohort, including behind-cursor IDs")
	cmd.Flags().StringVar(&sinceText, "since", "", "fixed export lower UTC timestamp (RFC3339)")
	cmd.Flags().StringVar(&untilText, "until", "", "fixed export upper UTC timestamp (RFC3339)")
	cmd.Flags().StringVar(&linkage, "linkage", "", "private verified linkage JSON map")
	cmd.Flags().BoolVar(&repairFilters, "repair-filters", false, "apply additive enrichment filterability schema repair")
	cmd.Flags().BoolVar(&filterPlan, "filter-plan", false, "report schema-only additive repair plan without writes")
	cmd.Flags().BoolVar(&apply, "apply", false, "conditionally patch owned enrichment fields with durable readback acknowledgments")
	cmd.Flags().IntVar(&limit, "limit", 10, "bounded page size 1..1000")
	cmd.Flags().IntVar(&retries, "retries", 2, "bounded write retry count 0..5")
	return cmd
}

func readBackfillManifest(path string) (backfillManifest, error) {
	var m backfillManifest
	b, e := os.ReadFile(path)
	if e != nil {
		return m, e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	return m, m.validate()
}
