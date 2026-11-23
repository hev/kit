package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/preview"
	"github.com/spf13/cobra"
)

func newPreviewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "preview",
		Short: "Build and serve private-network static site previews",
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List configured private previews",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			asJSON, _ := cmd.Flags().GetBool("json")
			return runPreviewLS(cmd.OutOrStdout(), asJSON)
		},
	}
	lsCmd.Flags().Bool("json", false, "Print JSON")

	showCmd := &cobra.Command{
		Use:   "show NAME",
		Short: "Inspect a configured private preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			asJSON, _ := cmd.Flags().GetBool("json")
			return runPreviewShow(cmd.OutOrStdout(), args[0], asJSON)
		},
	}
	showCmd.Flags().Bool("json", false, "Print JSON")

	syncCmd := &cobra.Command{
		Use:   "sync NAME",
		Short: "Fetch, build and atomically publish a private preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			asJSON, _ := cmd.Flags().GetBool("json")
			force, _ := cmd.Flags().GetBool("force")
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runPreviewSync(ctx, cmd.OutOrStdout(), args[0], preview.SyncOptions{Force: force}, asJSON)
		},
	}
	syncCmd.Flags().Bool("json", false, "Print JSON")
	syncCmd.Flags().Bool("force", false, "Rebuild even when SHA and env fingerprint are unchanged")

	serveCmd := &cobra.Command{
		Use:   "serve NAME",
		Short: "Serve a private preview strictly on its configured private address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runPreviewServe(ctx, cmd.ErrOrStderr(), args[0])
		},
	}

	root.AddCommand(lsCmd, showCmd, syncCmd, serveCmd)
	return root
}

func init() { rootCmd.AddCommand(newPreviewCommand()) }

func loadPreviewConfigs() ([]preview.Config, string, error) {
	path := daemon.DefaultConfigPath()
	configured, err := preview.Load(path)
	if err != nil {
		return nil, "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", errors.New("preview home directory unavailable")
	}
	return configured, home, nil
}

func findPreviewConfig(configured []preview.Config, name string) (preview.Config, error) {
	i := slices.IndexFunc(configured, func(c preview.Config) bool { return c.Name == name })
	if i == -1 {
		return preview.Config{}, fmt.Errorf("preview %q is not configured", name)
	}
	return configured[i], nil
}

func runPreviewLS(out io.Writer, asJSON bool) error {
	configured, home, err := loadPreviewConfigs()
	if err != nil {
		return err
	}
	r := preview.Runner{Home: home}
	rows := make([]preview.Summary, 0, len(configured))
	for _, cfg := range configured {
		d, err := r.Inspect(cfg)
		if err != nil {
			return err
		}
		rows = append(rows, d.Summary)
	}
	if asJSON {
		return json.NewEncoder(out).Encode(rows)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tMODE\tSTATE\tSHA\tBUILT AT")
	for _, s := range rows {
		sha := "-"
		if len(s.SHA) >= 7 {
			sha = s.SHA[:7]
		} else if s.SHA != "" {
			sha = s.SHA
		}
		built := s.BuiltAt
		if built == "" {
			built = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, s.URL, s.Mode, s.State, sha, built)
	}
	return w.Flush()
}

func runPreviewShow(out io.Writer, name string, asJSON bool) error {
	configured, home, err := loadPreviewConfigs()
	if err != nil {
		return err
	}
	cfg, err := findPreviewConfig(configured, name)
	if err != nil {
		return err
	}
	d, err := (preview.Runner{Home: home}).Inspect(cfg)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(out).Encode(d)
	}
	fmt.Fprintf(out, "%s  %s  %s  %s\n", d.Name, d.URL, d.Mode, d.State)
	if d.Mode == "repo" {
		fmt.Fprintf(out, "Repo: %s  Ref: %s\n", d.Repo, d.Ref)
		if d.Subdir != "" {
			fmt.Fprintf(out, "Subdir: %s  OutDir: %s\n", d.Subdir, d.OutDir)
		} else {
			fmt.Fprintf(out, "OutDir: %s\n", d.OutDir)
		}
		if len(d.BuildCmd) > 0 {
			fmt.Fprintf(out, "BuildCmd: %s\n", strings.Join(d.BuildCmd, " "))
		} else {
			fmt.Fprintln(out, "BuildCmd: pnpm install --frozen-lockfile && pnpm build")
		}
		fmt.Fprintf(out, "Builds: %d/%d  Timeout: %s\n", d.Builds, d.KeepBuilds, d.Timeout)
	}
	if len(d.EnvKeys) > 0 {
		fmt.Fprintf(out, "Env keys: %s  Fingerprint: %s\n", strings.Join(d.EnvKeys, ", "), d.ConfigEnvDigest)
	}
	if d.Current != nil {
		fmt.Fprintf(out, "Current SHA: %s  BuiltAt: %s\n", d.Current.SHA, d.Current.BuiltAt)
		fmt.Fprintf(out, "Current EnvFingerprint: %s\n", d.Current.EnvFingerprint)
		if d.EnvChangedSinceSync {
			fmt.Fprintln(out, "Note: configured env changed since last sync")
		}
	}
	return nil
}

func runPreviewSync(ctx context.Context, out io.Writer, name string, opts preview.SyncOptions, asJSON bool) error {
	configured, home, err := loadPreviewConfigs()
	if err != nil {
		return err
	}
	cfg, err := findPreviewConfig(configured, name)
	if err != nil {
		return err
	}
	res, err := (preview.Runner{Home: home}).Sync(ctx, cfg, opts)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(out).Encode(res)
	}
	shortSHA := res.SHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	fmt.Fprintf(out, "%s  %s  %s  %s\n", res.Site, res.Status, shortSHA, res.Ref)
	return nil
}

func runPreviewServe(ctx context.Context, errOut io.Writer, name string) error {
	configured, home, err := loadPreviewConfigs()
	if err != nil {
		return err
	}
	cfg, err := findPreviewConfig(configured, name)
	if err != nil {
		return err
	}
	return preview.Serve(ctx, cfg, home, func(addr string) {
		fmt.Fprintf(errOut, "serving preview %s on http://%s\n", cfg.Name, addr)
	})
}
