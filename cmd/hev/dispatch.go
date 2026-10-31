package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

func init() { rootCmd.AddCommand(newDispatchCommand()) }
func newDispatchCommand() *cobra.Command {
	var configPath, host string
	var asJSON, all, follow bool
	d := &cobra.Command{Use: "dispatch ORDER.md", Short: "Send agent work to configured machines", Args: cobra.ExactArgs(1)}
	d.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration")
	d.PersistentFlags().StringVar(&host, "host", "", "Select host")
	d.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	load := func() (dispatch.Config, error) {
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
		q := dispatch.Request{Action: "start", Name: o.Name, Order: string(b), Meta: dispatch.Meta{Order: o, Notify: c.Notify, Namespace: h.Namespace}, Cap: h.Cap, Workspace: c.Workspace}
		ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
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
			fmt.Fprintln(cmd.OutOrStdout(), "Browser validation requested; follow up with hev dispatch validate "+o.Name)
		}
		return nil
	}
	for _, action := range []string{"ls", "hosts", "show", "logs", "stop", "rework", "clean", "validate"} {
		action := action
		c := &cobra.Command{Use: action, Args: cobra.NoArgs}
		if action != "ls" && action != "hosts" {
			c.Use += " NAME"
			c.Args = cobra.ExactArgs(1)
		}
		if action == "rework" {
			c.Use += " NOTE..."
			c.Args = cobra.MinimumNArgs(2)
		}
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if action == "validate" {
				fmt.Fprintln(cmd.OutOrStdout(), "not yet implemented: run browser acceptance on a configured validation host and attach evidence to the order record")
				return nil
			}
			cfg, e := load()
			if e != nil {
				return e
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
					fmt.Fprintln(w, "NAME\tHOST\tSTATE\tAGE\tPR\tTRACE")
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
							if all || row.State == "running" || row.State == "died" {
								fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", row.Name, n, row.State, time.Since(row.Start).Round(time.Second), row.PR, row.Trace)
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
			q := dispatch.Request{Action: action, Name: name, Cap: h.Cap}
			if action == "rework" {
				q.Note = strings.Join(args[1:], " ")
			}
			previous := ""
			for {
				ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
				r, e := dispatch.RPC(ctx, h, q)
				cancel()
				if e != nil {
					return e
				}
				if asJSON {
					return emit(r)
				}
				if action == "show" {
					for _, f := range []string{"meta.json", "status.md", "report.md", "result.json"} {
						fmt.Fprintf(cmd.OutOrStdout(), "--- %s ---\n%s\n", f, r.Files[f])
					}
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				} else if action == "logs" {
					delta := r.Log
					if strings.HasPrefix(delta, previous) {
						delta = delta[len(previous):]
					}
					fmt.Fprint(cmd.OutOrStdout(), delta)
					previous = r.Log
					if follow {
						info := dispatch.Inventory(dispatch.Config{Hosts: map[string]dispatch.Host{"selected": h}}, "")["selected"]
						running := false
						for _, row := range info.Rows {
							if row.Name == name && row.State == "running" {
								running = true
							}
						}
						if running {
							select {
							case <-cmd.Context().Done():
								return cmd.Context().Err()
							case <-time.After(time.Second):
								continue
							}
						}
						finalCtx, finalCancel := context.WithTimeout(cmd.Context(), 12*time.Second)
						final, err := dispatch.RPC(finalCtx, h, q)
						finalCancel()
						if err != nil {
							return err
						}
						tail := final.Log
						if strings.HasPrefix(tail, previous) {
							tail = tail[len(previous):]
						}
						fmt.Fprint(cmd.OutOrStdout(), tail)

					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), r.Message)
				}
				return nil
			}
		}
		if action == "ls" {
			c.Flags().BoolVar(&all, "all", false, "Include completed runs")
		}
		if action == "logs" {
			c.Flags().BoolVarP(&follow, "follow", "f", false, "Follow worker output")
		}
		d.AddCommand(c)
	}
	d.AddCommand(&cobra.Command{Use: "supervise NAME", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return dispatch.Supervise(args[0]) }})
	d.AddCommand(&cobra.Command{Use: "rpc PAYLOAD", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		b, e := base64.StdEncoding.DecodeString(args[0])
		if e != nil {
			return e
		}
		var q dispatch.Request
		if e = json.Unmarshal(b, &q); e != nil {
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
