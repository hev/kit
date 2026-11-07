package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

// hev repos manages a dispatch host's [admission] repos over the same RPC as
// hev dispatch. Without --host it acts on this machine as its owner.
func newReposCommand() *cobra.Command {
	var configPath, host string
	var asJSON bool
	root := &cobra.Command{Use: "repos", Short: "List and add repositories admitted on a dispatch host"}
	root.PersistentFlags().StringVar(&configPath, "config", "", "Dispatch TOML configuration (selects --host)")
	root.PersistentFlags().StringVar(&host, "host", "", "Dispatch host; omit for this machine")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	call := func(cmd *cobra.Command, q dispatch.Request, timeout time.Duration) (dispatch.Response, error) {
		q.Meta.Sender = dispatch.LocalSender()
		h := dispatch.Host{Local: true}
		if host != "" {
			c, e := dispatch.Load(dispatch.ConfigPath(configPath))
			if e != nil {
				return dispatch.Response{}, e
			}
			var ok bool
			if h, ok = c.Hosts[host]; !ok {
				return dispatch.Response{}, fmt.Errorf("unknown host %s", host)
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		r, e := dispatch.RPC(ctx, h, q)
		// Hosts older than repos-add/ls answer with a generic refusal.
		if e != nil && (e.Error() == "invalid name" || strings.Contains(e.Error(), "action "+q.Action+" is unavailable")) {
			e = fmt.Errorf("%w (the host's hev may predate hev repos; ask the owner to upgrade)", e)
		}
		return r, e
	}
	add := &cobra.Command{Use: "add URL", Short: "Clone a repository into the host workspace and admit it", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, e := call(cmd, dispatch.Request{Action: "repos-add", URL: args[0]}, 15*time.Minute)
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
		}
		fmt.Fprintln(cmd.OutOrStdout(), r.Message)
		return nil
	}}
	ls := &cobra.Command{Use: "ls", Aliases: []string{"list"}, Short: "List repositories admitted on the host", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		r, e := call(cmd, dispatch.Request{Action: "repos-ls"}, time.Minute)
		if e != nil {
			return e
		}
		if r.RepoPolicy == nil {
			return fmt.Errorf("host does not support repos ls; upgrade hev on the host")
		}
		p := r.RepoPolicy
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(p)
		}
		out := cmd.OutOrStdout()
		if p.AdmitAll {
			fmt.Fprintln(out, "admission: every repository in the host workspace (no [admission] repos list)")
		}
		for _, repo := range p.Repos {
			name, by := repo.Name, ""
			if name == "" {
				name = "-"
			}
			if repo.AddedBy != "" {
				by = fmt.Sprintf(" added-by=%s added=%s", repo.AddedBy, repo.AddedAt.Format(time.RFC3339))
			}
			fmt.Fprintf(out, "%s %s%s\n", name, repo.Origin, by)
		}
		for _, prefix := range p.AddFrom {
			fmt.Fprintf(out, "add-from %s\n", prefix)
		}
		fmt.Fprintf(out, "can-add=%v\n", p.CanAdd)
		return nil
	}}
	root.AddCommand(add, ls)
	return root
}
