package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/hygiene"
	"github.com/spf13/cobra"
)

func newHygieneCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "hygiene",
		Short: "Run and inspect host disk-pressure and cache hygiene",
	}

	var dryRun, runJSON bool
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Execute one bounded disk-hygiene pass",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runHygienePass(ctx, cmd.OutOrStdout(), dryRun, runJSON)
		},
	}
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report planned hygiene actions without mutating files")
	runCmd.Flags().BoolVar(&runJSON, "json", false, "Print JSON")

	var statusJSON bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show last hygiene run state and configured thresholds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			return runHygieneStatus(cmd.OutOrStdout(), statusJSON)
		},
	}
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Print JSON")

	root.AddCommand(runCmd, statusCmd)
	return root
}

func init() { rootCmd.AddCommand(newHygieneCommand()) }

func runHygienePass(ctx context.Context, out io.Writer, dryRun, asJSON bool) error {
	cfg, err := hygiene.Load(daemon.DefaultConfigPath())
	if err != nil {
		return err
	}
	runner := hygiene.Runner{}
	rep, err := runner.Run(ctx, cfg, dryRun)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	if rep.DryRun {
		fmt.Fprintln(out, "Mode: dry-run (no files modified)")
	}
	fmt.Fprintf(out, "Disk free: %s -> %s (under pressure: %t)\n",
		formatBytes(rep.FreeBytesBefore), formatBytes(rep.FreeBytesAfter), rep.UnderPressure)
	if rep.DryRun {
		fmt.Fprintf(out, "Estimated bytes to free: %s\n", formatBytes(uint64(max(rep.BytesFreed, 0))))
	} else {
		fmt.Fprintf(out, "Bytes freed: %s\n", formatBytes(uint64(max(rep.BytesFreed, 0))))
	}
	compactVerb := "Compacted"
	pruneVerb := "Pruned"
	if rep.DryRun {
		compactVerb = "Would compact"
		pruneVerb = "Would prune"
	}
	fmt.Fprintf(out, "%s dispatch runs (%d): %s\n", compactVerb, len(rep.CompactedRuns), joinOrNone(rep.CompactedRuns))
	fmt.Fprintf(out, "%s tmp dirs (%d): %s\n", pruneVerb, len(rep.PrunedTmpDirs), joinOrNone(rep.PrunedTmpDirs))
	apfsLine := rep.APFS.Action
	if rep.APFS.Reason != "" {
		apfsLine += " (" + rep.APFS.Reason + ")"
	} else if rep.APFS.TargetBytes > 0 {
		apfsLine += " (target " + formatBytes(rep.APFS.TargetBytes) + ")"
	}
	if rep.APFS.Error != "" {
		apfsLine += ": " + rep.APFS.Error
	}
	fmt.Fprintf(out, "APFS snapshots: %s\n", apfsLine)
	if len(rep.Errors) > 0 {
		fmt.Fprintf(out, "Warnings: %s\n", strings.Join(rep.Errors, "; "))
	}
	return nil
}

func runHygieneStatus(out io.Writer, asJSON bool) error {
	cfg, err := hygiene.Load(daemon.DefaultConfigPath())
	if err != nil {
		return err
	}
	runner := hygiene.Runner{}
	st, err := runner.Status(cfg)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}
	fmt.Fprintf(out, "Enabled: %t  Every: %s  Min free: %d GiB\n",
		st.Config.Enabled, st.Config.Every, st.Config.MinFreeGBValue())
	fmt.Fprintf(out, "Compact after: %s  Prune tmp after: %s  Thin APFS: %t\n",
		st.Config.CompactAfter, st.Config.PruneTmpAfter, st.Config.ThinAPFSSnapshots != nil && *st.Config.ThinAPFSSnapshots)
	fmt.Fprintf(out, "Current free: %s (under pressure: %t)\n",
		formatBytes(st.CurrentFreeBytes), st.UnderPressure)
	if st.State.LastRun == nil {
		fmt.Fprintln(out, "Last run: never")
	} else {
		fmt.Fprintf(out, "Last run: %s  Free: %s -> %s  Freed: %s\n",
			st.State.LastRun.UTC().Format(time.RFC3339),
			formatBytes(st.State.FreeBytesBefore),
			formatBytes(st.State.FreeBytesAfter),
			formatBytes(uint64(max(st.State.BytesFreed, 0))))
		fmt.Fprintf(out, "Last actions: compacted=%d  pruned_tmp=%d  apfs_thinned=%t\n",
			st.State.CompactedRuns, st.State.PrunedTmpDirs, st.State.APFSThinned)
		if st.State.LastError != "" {
			fmt.Fprintf(out, "Last error: %s\n", st.State.LastError)
		}
	}
	if st.NextRun != nil {
		fmt.Fprintf(out, "Next run: %s\n", st.NextRun.UTC().Format(time.RFC3339))
	}
	return nil
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func formatBytes(b uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case b >= gib:
		return fmt.Sprintf("%.2f GiB", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(mib))
	case b >= kib:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(kib))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
