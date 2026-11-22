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
	"unicode"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/services"
	"github.com/spf13/cobra"
)

func newServiceCommand() *cobra.Command {
	root := &cobra.Command{Use: "service", Short: "List, inspect and control hevd services"}
	for _, action := range []string{"ls", "show", "logs", "start", "stop", "restart"} {
		use, args := action+" NAME", cobra.ExactArgs(1)
		if action == "ls" {
			use, args = action, cobra.NoArgs
		}
		c := &cobra.Command{Use: use, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			asJSON, _ := cmd.Flags().GetBool("json")
			lines, _ := cmd.Flags().GetInt("lines")
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runServiceCLI(ctx, cmd.OutOrStdout(), action, args, asJSON, lines)
		}}
		if action == "ls" || action == "show" {
			c.Flags().Bool("json", false, "Print JSON")
		}
		if action == "logs" {
			c.Flags().IntP("lines", "n", 100, "Number of recent log lines to print (0 for all retained lines)")
		}
		root.AddCommand(c)
	}
	return root
}

func init() { rootCmd.AddCommand(newServiceCommand()) }

func runServiceCLI(ctx context.Context, out io.Writer, action string, args []string, asJSON bool, lines int) error {
	if action == "logs" && lines < 0 {
		return errors.New("lines must be nonnegative")
	}
	path := daemon.DefaultConfigPath()
	configured, err := services.Load(path)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.New("service home directory unavailable")
	}
	self, _ := os.Hostname()
	cfg, clusterErr := cluster.LoadConfig(path)
	if cfg != nil && cfg.Name != "" {
		self = cfg.Name
	}
	sup := services.Supervisor{
		Home:       home,
		Membership: services.RemoteMembership(self, cfg != nil || clusterErr != nil),
		Resolve: func(ctx context.Context, refs []secrets.Ref) (map[string]string, error) {
			return secrets.ResolveWith(ctx, refs, secrets.Options{CacheOnly: true, Caller: "hev service"})
		},
		Notice: func(ctx context.Context, n attention.Notice) error {
			return attention.Default().Emit(ctx, n, "")
		},
	}
	if clusterErr != nil {
		sup.Membership.Members = nil
	}
	if action == "ls" {
		members := sup.Membership.Members
		if members != nil {
			get := sync.OnceValue(func() []cluster.Member { return members(ctx) })
			sup.Membership.Members = func(context.Context) []cluster.Member { return get() }
		}
		return listServices(ctx, out, sup, configured, asJSON)
	}

	i := slices.IndexFunc(configured, func(s services.Service) bool { return s.Name == args[0] })
	if i == -1 {
		return errors.New("service is not configured")
	}
	svc := configured[i]
	switch action {
	case "show":
		d, err := sup.Inspect(ctx, svc, 20)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(out).Encode(d)
		}
		fmt.Fprintf(out, "%s  restart=%s  %s\n", d.Name, d.Restart, serviceStatusLabel(d.Summary))
		fmt.Fprintf(out, "PID: %s  Start: %s  Restarts: %d  Last exit: %s\n",
			servicePID(d.PID), jobTime(d.StartTime), d.Restarts, jobExit(d.LastExit))
		if d.State.LastStop != nil || d.NextRestart != nil {
			fmt.Fprintf(out, "Last stop: %s  Next restart: %s\n", jobTime(d.State.LastStop), jobTime(d.NextRestart))
		}
		if d.PlacementWinner != "" {
			fmt.Fprintln(out, "Placement winner:", d.PlacementWinner)
		}
		if d.Logs != "" {
			return printSafeServiceText(out, d.Logs)
		}
		return nil
	case "logs":
		logs, err := sup.Logs(svc, lines)
		if err != nil {
			return err
		}
		if logs == "" {
			return nil
		}
		return printSafeServiceText(out, logs)
	case "start", "stop", "restart":
		st, err := sup.Control(ctx, svc, action)
		if err != nil {
			return err
		}
		past := action + "ed"
		if action == "stop" {
			past = "stopped"
		}
		if st.PID > 0 {
			_, err = fmt.Fprintf(out, "%s %s (pid %d)\n", svc.Name, past, st.PID)
			return err
		}
		_, err = fmt.Fprintf(out, "%s %s\n", svc.Name, past)
		return err
	}
	return errors.New("unknown service command")
}

func servicePID(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return fmt.Sprint(pid)
}

func serviceStatusLabel(s services.Summary) string {
	status := s.Status
	if status == services.StatusStandby && s.PlacementWinner != "" {
		status += " (" + s.PlacementWinner + ")"
	}
	if s.Reason != "" {
		status += " — " + s.Reason
	}
	return status
}

func listServices(ctx context.Context, out io.Writer, sup services.Supervisor, configured []services.Service, asJSON bool) error {
	rows := make([]services.Summary, 0, len(configured))
	for _, svc := range configured {
		d, err := sup.Inspect(ctx, svc, 0)
		if err != nil {
			return err
		}
		rows = append(rows, d.Summary)
	}
	if asJSON {
		return json.NewEncoder(out).Encode(rows)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tPID\tRESTARTS\tLAST EXIT\tSTART TIME")
	for _, s := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			s.Name, serviceStatusLabel(s), servicePID(s.PID), s.Restarts, jobExit(s.LastExit), jobTime(s.StartTime))
	}
	return w.Flush()
}

func printSafeServiceText(out io.Writer, text string) error {
	safe := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return '\uFFFD'
		}
		return r
	}, text)
	_, err := fmt.Fprintln(out, strings.TrimSuffix(safe, "\n"))
	return err
}
