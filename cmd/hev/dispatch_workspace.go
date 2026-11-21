package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

func addDispatchWorkspaceCommands(parent *cobra.Command, load func() (dispatch.Config, error), host *string, asJSON *bool) {
	var all bool
	invoke := func(cmd *cobra.Command, action, name string) error {
		cfg, err := load()
		if err != nil && (!os.IsNotExist(err) || *host != "") {
			return err
		}
		target := *host
		var h dispatch.Host
		if name != "" {
			explicit, run, err := dispatch.SplitRunAddress(name)
			if err != nil {
				return err
			}
			if explicit != "" {
				if target != "" && explicit != target {
					return fmt.Errorf("run address and --host disagree")
				}
				target = explicit
			}
			name = run
			if target == "" && len(cfg.Hosts) == 0 {
				h = dispatch.Host{Local: true}
			} else if _, h, err = locateDispatchRun(cfg, target, name); err != nil {
				return err
			}
		} else if target == "" {
			h = dispatch.Host{Local: true}
		} else {
			var ok bool
			h, ok = cfg.Hosts[target]
			if !ok {
				return fmt.Errorf("unknown host %s", target)
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		response, err := dispatch.RPC(ctx, h, dispatch.Request{Action: action, Name: name})
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"message": response.Message})
		}
		fmt.Fprintln(cmd.OutOrStdout(), response.Message)
		return nil
	}
	compact := &cobra.Command{
		Use:   "compact [HOST/]NAME | --all",
		Short: "Remove safely recoverable Pro worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) == 0 {
				return invoke(cmd, "compact-all", "")
			}
			if !all && len(args) == 1 {
				return invoke(cmd, "compact", args[0])
			}
			return fmt.Errorf("specify one run name or --all")
		},
	}
	compact.Flags().BoolVar(&all, "all", false, "Compact eligible runs on the selected host")
	parent.AddCommand(compact, &cobra.Command{
		Use:   "gc",
		Short: "Evict unlocked workspace slots idle over seven days",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke(cmd, "workspace-gc", "")
		},
	})
}
