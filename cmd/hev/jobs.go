package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newJobsCommand()) }
func newJobsCommand() *cobra.Command {
	root := &cobra.Command{Use: "jobs", Short: "Schedule finite commands on this host", Args: cobra.NoArgs}
	var structured bool
	root.PersistentFlags().BoolVar(&structured, "json", false, "Print structured results")
	emit := func(cmd *cobra.Command, v any) error {
		// Human and JSON modes share a complete inspection record; JSON is stable
		// and avoids hiding pending or unknown state in a terse success line.
		enc := json.NewEncoder(cmd.OutOrStdout())
		if !structured {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(v)
	}
	var file string
	var paused, dry bool
	apply := &cobra.Command{Use: "apply -f FILE", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		s, e := dispatch.ParseJob(b)
		if e != nil {
			return e
		}
		state, e := dispatch.ApplyJob(s, time.Now(), paused, dry)
		if e != nil {
			return e
		}
		return emit(cmd, state)
	}}
	apply.Flags().StringVarP(&file, "file", "f", "", "Job Markdown file")
	_ = apply.MarkFlagRequired("file")
	apply.Flags().BoolVar(&paused, "paused", false, "Apply paused")
	apply.Flags().BoolVar(&dry, "dry-run", false, "Validate without changing state or reading secrets")
	root.AddCommand(apply)
	root.AddCommand(&cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, e := dispatch.Jobs()
		if e != nil {
			return e
		}
		return emit(cmd, s)
	}})
	root.AddCommand(&cobra.Command{Use: "show NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		s, e := dispatch.Job(a[0])
		if e != nil {
			return e
		}
		return emit(cmd, s)
	}})
	var limit int
	history := &cobra.Command{Use: "history NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		s, e := dispatch.Job(a[0])
		if e != nil {
			return e
		}
		h := s.History
		if limit > 0 && len(h) > limit {
			h = h[len(h)-limit:]
		}
		return emit(cmd, h)
	}}
	history.Flags().IntVar(&limit, "limit", 20, "Maximum occurrences (0 for all)")
	root.AddCommand(history)
	var occurrence string
	logs := &cobra.Command{Use: "logs NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		s, e := dispatch.Job(a[0])
		if e != nil {
			return e
		}
		for i := len(s.History) - 1; i >= 0; i-- {
			o := s.History[i]
			if o.Run == "" || (occurrence != "" && occurrence != o.ID) {
				continue
			}
			b, e := os.ReadFile(filepath.Join(dispatch.Root(), o.Run, "log"))
			if e != nil {
				return e
			}
			if structured {
				return emit(cmd, map[string]string{"occurrence": o.ID, "run": o.Run, "log": string(b)})
			}
			_, e = fmt.Fprint(cmd.OutOrStdout(), string(b))
			return e
		}
		return fmt.Errorf("no command occurrence found")
	}}
	logs.Flags().StringVar(&occurrence, "occurrence", "", "Occurrence ID (default latest command)")
	root.AddCommand(logs)
	for _, action := range []string{"run", "pause", "resume", "rm"} {
		control := action
		c := &cobra.Command{Use: control + " NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
			s, e := dispatch.ControlJob(a[0], control, time.Now())
			if e != nil {
				return e
			}
			return emit(cmd, s)
		}}
		if control == "rm" {
			c.Aliases = []string{"remove"}
		}
		root.AddCommand(c)
	}
	var reconcileID, reconcileOutcome, reconcileEvidence string
	reconcile := &cobra.Command{Use: "reconcile NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		s, e := dispatch.ReconcileJob(a[0], reconcileID, reconcileOutcome, reconcileEvidence, time.Now())
		if e != nil {
			return e
		}
		return emit(cmd, s)
	}}
	reconcile.Flags().StringVar(&reconcileID, "occurrence", "", "Held occurrence ID")
	reconcile.Flags().StringVar(&reconcileOutcome, "outcome", "", "Verified succeeded, failed or cancelled outcome")
	reconcile.Flags().StringVar(&reconcileEvidence, "evidence", "", "Owner's verification evidence (no secrets)")
	root.AddCommand(reconcile)
	root.AddCommand(&cobra.Command{Use: "supervise RUN", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error { return dispatch.SuperviseExec(a[0], cmd.InOrStdin()) }})
	return root
}
