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
	"sync"
	"syscall"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/jobs"
	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
)

func newJobsCommand() *cobra.Command {
	root := &cobra.Command{Use: "jobs", Short: "List, inspect and run hevd scheduled jobs"}
	for _, action := range []string{"ls", "show", "run", "pause", "resume"} {
		use, args := action+" NAME", cobra.ExactArgs(1)
		if action == "ls" {
			use, args = action, cobra.NoArgs
		}
		c := &cobra.Command{Use: use, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			asJSON, _ := cmd.Flags().GetBool("json")
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runJobs(ctx, cmd.OutOrStdout(), action, args, asJSON)
		}}
		if action == "ls" || action == "show" {
			c.Flags().Bool("json", false, "Print JSON")
		}
		root.AddCommand(c)
	}
	return root
}

func init() { rootCmd.AddCommand(newJobsCommand()) }

func runJobs(ctx context.Context, out io.Writer, action string, args []string, asJSON bool) error {
	path := daemon.DefaultConfigPath()
	configured, err := jobs.Load(path)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.New("job home directory unavailable")
	}
	self, _ := os.Hostname()
	cfg, clusterErr := cluster.LoadConfig(path)
	if cfg != nil && cfg.Name != "" {
		self = cfg.Name
	}
	r := jobs.Runner{
		Home: home, Membership: jobs.RemoteMembership(self, cfg != nil || clusterErr != nil),
		Resolve: func(ctx context.Context, refs []secrets.Ref) (map[string]string, error) {
			return secrets.ResolveWith(ctx, refs, secrets.Options{CacheOnly: true, Caller: "hev jobs"})
		},
		Notice: func(ctx context.Context, n attention.Notice) error {
			return attention.Default().Emit(ctx, n, "")
		},
	}
	if clusterErr != nil {
		r.Membership.Members = nil
	}
	if action == "ls" {
		// One membership snapshot gives every row the same view.
		members := r.Membership.Members
		if members != nil {
			get := sync.OnceValue(func() []cluster.Member { return members(ctx) })
			r.Membership.Members = func(context.Context) []cluster.Member { return get() }
		}
		return listJobs(ctx, out, r, configured, asJSON)
	}

	i := slices.IndexFunc(configured, func(j jobs.Job) bool { return j.Name == args[0] })
	if i == -1 {
		return errors.New("job is not configured")
	}
	j := configured[i]
	switch action {
	case "show":
		d, err := r.Inspect(ctx, j, true)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(out).Encode(d)
		}
		fmt.Fprintf(out, "%s  %s  %s\n", d.Name, d.Schedule, jobStatus(d.Summary))
		fmt.Fprintf(out, "Last start: %s  Last end: %s  Exit: %s  Next tick: %s\n",
			jobTime(d.State.LastStart), jobTime(d.State.LastEnd), jobExit(d.State.ExitCode), jobTime(d.Next))
		fmt.Fprintf(out, "Last duration: %s  Consecutive failures: %d  Paused: %t\n",
			d.State.Duration.Round(time.Millisecond), d.State.ConsecutiveFailures, d.State.Paused)
		if d.State.LastPlacementWinner != "" {
			fmt.Fprintln(out, "Last placement winner:", d.State.LastPlacementWinner)
		}
		for _, run := range d.Runs {
			if err := printJobRun(out, run); err != nil {
				return err
			}
		}
		return nil
	case "run":
		run, err := r.Run(ctx, j)
		if err != nil {
			return err
		}
		if err := printJobRun(out, run); err != nil {
			return err
		}
		if run.Status != "ok" {
			return fmt.Errorf("job %s", run.Status)
		}
		return nil
	case "pause", "resume":
		if err := r.Pause(ctx, j, action == "pause"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, j.Name, action+"d")
		return err
	}
	return errors.New("unknown jobs command")
}

func jobTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func jobExit(code *int) string {
	if code == nil {
		return "-"
	}
	return fmt.Sprint(*code)
}

func jobStatus(s jobs.Summary) string {
	status := s.Status
	if status == "standby" && s.Winner != "" {
		status += " (" + s.Winner + ")"
	}
	if s.Reason != "" {
		status += " — " + s.Reason
	}
	return status
}

func listJobs(ctx context.Context, out io.Writer, r jobs.Runner, configured []jobs.Job, asJSON bool) error {
	rows := make([]jobs.Summary, 0, len(configured))
	for _, j := range configured {
		d, err := r.Inspect(ctx, j, false)
		if err != nil {
			return err
		}
		rows = append(rows, d.Summary)
	}
	if asJSON {
		return json.NewEncoder(out).Encode(rows)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCHEDULE\tSTATE\tLAST EXIT\tLAST START\tNEXT RUN")
	for _, s := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, s.Schedule, jobStatus(s),
			jobExit(s.State.ExitCode), jobTime(s.State.LastStart), jobTime(s.Next))
	}
	return w.Flush()
}

func printJobRun(out io.Writer, run jobs.Run) error {
	_, err := fmt.Fprintf(out, "%s  %s  exit %s  %s\n", jobTime(&run.Start), run.Status, jobExit(run.ExitCode), run.Duration.Round(time.Millisecond))
	if err != nil {
		return err
	}
	if run.Message != "" {
		if _, err := fmt.Fprintln(out, run.Message); err != nil {
			return err
		}
	}
	if run.Output == "" {
		return nil
	}
	safe := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return '\uFFFD'
		}
		return r
	}, run.Output)
	_, err = fmt.Fprintln(out, strings.TrimSuffix(safe, "\n"))
	return err
}
