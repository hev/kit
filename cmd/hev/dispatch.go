package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"
)

func init() { rootCmd.AddCommand(newDispatchCommand()) }
func newDispatchCommand() *cobra.Command {
	var configPath, host string
	var asJSON, all, follow, allowSame, fromValidation bool
	d := &cobra.Command{Use: "dispatch ORDER.md", Short: "Send agent work to configured machines", Args: cobra.ExactArgs(1)}
	d.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration")
	d.PersistentFlags().StringVar(&host, "host", "", "Select host")
	d.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	load := func() (dispatch.Config, error) {
		configPath = dispatch.ConfigPath(configPath)
		c, e := dispatch.Load(configPath)
		if e == nil && host != "" {
			if _, ok := c.Hosts[host]; !ok {
				return c, fmt.Errorf("unknown host %s", host)
			}
		}
		return c, e
	}
	emit := func(v any) error { return json.NewEncoder(d.OutOrStdout()).Encode(v) }
	d.RunE = func(cmd *cobra.Command, args []string) error {
		c, e := load()
		if e != nil {
			return e
		}
		b, e := os.ReadFile(args[0])
		if e != nil {
			return e
		}
		o, e := dispatch.Parse(string(b))
		if e != nil {
			return e
		}
		if host != "" {
			o.Host = host
		}
		h, ok := c.Hosts[o.Host]
		if !ok {
			return fmt.Errorf("unknown or missing host %s", o.Host)
		}
		q := dispatch.Request{Action: "start", Name: o.Name, Order: string(b), Meta: dispatch.Meta{Order: o, Namespace: h.Namespace}, Cap: h.Cap, Workspace: c.Workspace}
		q.Meta.ConfigPath = h.ConfigPath
		if h.Local {
			if configPath != "" {
				q.Meta.ConfigPath, e = filepath.Abs(configPath)
				if e != nil {
					return e
				}
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		r, e := dispatch.RPC(ctx, h, q)
		if e != nil {
			if strings.Contains(e.Error(), "host full") {
				inventory := dispatch.Inventory(c, "")
				for _, n := range dispatch.SortedHosts(inventory) {
					info := inventory[n]
					if info.Error != "" {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s: unreachable: %s\n", n, info.Error)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s: %d free\n", n, max(0, c.Hosts[n].Cap-info.Running))
					}
				}
			}
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), r.Message)
		if o.Validation == "browser" {
			fmt.Fprintln(cmd.OutOrStdout(), "Browser validation requested; inspect with hev dispatch show "+o.Name)
		}
		return nil
	}
	d.AddCommand(newDispatchRun(load, &configPath, &host))
	for _, action := range []string{"say", "attach", "ls", "hosts", "show", "logs", "stop", "rework", "clean", "validate"} {
		action := action
		c := &cobra.Command{Use: action, Args: cobra.NoArgs}
		if action != "ls" && action != "hosts" {
			c.Use += " NAME"
			c.Args = cobra.ExactArgs(1)
		}
		if action == "rework" || action == "say" {
			c.Use += " NOTE..."
			c.Args = func(cmd *cobra.Command, args []string) error {
				if len(args) == 0 || (len(args) == 1 && !fromValidation) {
					return fmt.Errorf("rework requires a note or --from-validation")
				}
				return nil
			}
		}
		c.RunE = func(cmd *cobra.Command, args []string) error {
			cfg, e := load()
			if e != nil {
				return e
			}
			if action == "validate" {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
				defer cancel()
				r, err := dispatch.Validate(ctx, cfg, args[0], host, allowSame)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				return nil
			}
			inv := dispatch.Inventory(cfg, host)
			if action == "ls" || action == "hosts" {
				if asJSON {
					return emit(inv)
				}
				w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
				defer w.Flush()
				if action == "hosts" {
					fmt.Fprintln(w, "HOST\tCAP\tRUNNING\tFREE\tREACHABLE\tVERSION")
				} else {
					fmt.Fprintln(w, "NAME\tHOST\tSTATE\tVALIDATION\tAGE\tPR\tTRACE")
				}
				for _, n := range dispatch.SortedHosts(inv) {
					r := inv[n]
					if r.Error != "" {
						fmt.Fprintf(w, "%s: unreachable: %s\n", n, r.Error)
						continue
					}
					if action == "hosts" {
						v := r.Version
						if v != version.Version {
							v += " (version mismatch)"
						}
						fmt.Fprintf(w, "%s\t%d\t%d\t%d\tyes\t%s\n", n, cfg.Hosts[n].Cap, r.Running, max(0, cfg.Hosts[n].Cap-r.Running), v)
					} else {
						for _, row := range r.Rows {
							if all || dispatch.VisibleByDefault(row) {
								if row.SameHost {
									row.Validation += " (same-host)"
								}
								fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", row.Name, n, row.State, row.Validation, time.Since(row.Start).Round(time.Second), row.PR, row.Trace)
							}
						}
					}
				}
				return nil
			}
			name := args[0]
			if !dispatch.ValidName(name) {
				return fmt.Errorf("invalid name")
			}
			matches := []string{}
			for n, r := range inv {
				for _, row := range r.Rows {
					if row.Name == name {
						matches = append(matches, n)
					}
				}
			}
			if len(matches) != 1 {
				return fmt.Errorf("order not found or ambiguous; select --host; inventory: %v", inv)
			}
			h := cfg.Hosts[matches[0]]
			if action == "attach" {
				if runtime.GOOS == "windows" {
					fmt.Fprintf(cmd.OutOrStdout(), "Log: ~/dispatch/%s/log\nUse hev dispatch logs %s -f\n", name, name)
					return nil
				}
				var c *exec.Cmd
				if h.Local {
					c = exec.CommandContext(cmd.Context(), "tmux", "attach", "-t", "d-"+name)
				} else {
					c = exec.CommandContext(cmd.Context(), "ssh", "-t", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", h.SSH, "tmux attach -t "+dispatch.Quote("d-"+name))
				}
				c.Stdin = cmd.InOrStdin()
				c.Stdout = cmd.OutOrStdout()
				c.Stderr = cmd.ErrOrStderr()
				return c.Run()
			}
			q := dispatch.Request{Action: action, Name: name, Cap: h.Cap}
			if action == "rework" || action == "say" {
				q.Note, e = dispatchMessage(cmd, args[1:])
				if e != nil {
					return e
				}
				q.FromValidation = fromValidation
			}

			for {
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
				r, e := dispatch.RPC(ctx, h, q)
				cancel()
				if e != nil {
					return e
				}
				if asJSON {
					return emit(r)
				}
				if action == "show" {
					for _, f := range []string{"meta.json", "status.md", "report.md", "result.json", "inbox.md", "validation.json", "verdict.json"} {
						fmt.Fprintf(cmd.OutOrStdout(), "--- %s ---\n%s\n", f, r.Files[f])
					}
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				} else if action == "logs" {
					fmt.Fprint(cmd.OutOrStdout(), r.Log)
					q.Offset = r.Offset
					if follow && r.Active {
						select {
						case <-cmd.Context().Done():
							return cmd.Context().Err()
						case <-time.After(time.Second):
							continue
						}
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				}
				return nil
			}
		}
		if action == "validate" {
			c.Flags().BoolVar(&allowSame, "allow-same-model", false, "Allow the implementer's harness and model")
		}
		if action == "rework" {
			c.Flags().BoolVar(&fromValidation, "from-validation", false, "Use latest failed browser verdict")
		}
		if action == "ls" {
			c.Flags().BoolVar(&all, "all", false, "Include completed runs")
		}
		if action == "logs" {
			c.Flags().BoolVarP(&follow, "follow", "f", false, "Follow worker output")
		}
		d.AddCommand(c)
	}
	d.AddCommand(&cobra.Command{Use: "notify-validation NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return dispatch.NotifyValidation(args[0]) }})
	d.AddCommand(&cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return dispatch.Supervise(args[0]) }})
	d.AddCommand(&cobra.Command{Use: "rpc", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var q dispatch.Request
		if e := json.NewDecoder(cmd.InOrStdin()).Decode(&q); e != nil {
			return e
		}
		r, e := dispatch.Handle(q)
		r.Version = version.Version
		if e != nil {
			r.Error = e.Error()
		}
		return emit(r)
	}})
	return d
}

func dispatchMessage(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		b, err := io.ReadAll(cmd.InOrStdin())
		return string(b), err
	}
	return strings.Join(args, " "), nil
}
