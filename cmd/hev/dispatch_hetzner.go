package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

func newDispatchHetzner(load func() (dispatch.Config, error), host *string) *cobra.Command {
	root := &cobra.Command{Use: "hetzner", Short: "Manage billable Hetzner hosts explicitly"}
	for _, action := range []string{"provision", "show", "destroy"} {
		action := action
		var confirm bool
		c := &cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			defer cancel()
			cfg, e := load()
			if e != nil {
				return e
			}
			h, ok := cfg.Hosts[*host]
			if !ok || h.Type != "hetzner" {
				return fmt.Errorf("select a Hetzner host with --host")
			}
			p, e := dispatch.NewHetzner(h)
			if e != nil {
				return e
			}
			switch action {
			case "provision":
				if h.Mode == "run" {
					return fmt.Errorf("provision requires persistent mode")
				}
				m, e := dispatch.ProvisionHetzner(ctx, h)
				if e != nil {
					return e
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(m)
			case "show":
				result, e := p.Show(ctx)
				if e != nil {
					return e
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			case "destroy":
				if !confirm {
					return fmt.Errorf("destroy deletes all servers for this host; pass --confirm")
				}
				return p.Destroy(ctx)
			}
			return nil
		}}
		if action == "destroy" {
			c.Flags().BoolVar(&confirm, "confirm", false, "Confirm deletion of every labelled server and related resource for this host")
		}
		root.AddCommand(c)
	}
	return root
}
