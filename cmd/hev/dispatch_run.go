package main

import (
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
	"time"
)

func newDispatchRun(load func() (dispatch.Config, error), configPath, host *string) *cobra.Command {
	var inline dispatch.Order
	var accepts []string
	run := &cobra.Command{Use: "run REPO TASK...", Args: cobra.MinimumNArgs(2)}
	run.Flags().StringVar(&inline.Name, "name", "", "Run name")
	run.Flags().StringVar(&inline.Base, "base", "", "Base branch")
	run.Flags().StringVar(&inline.Harness, "harness", "", "Agent harness")
	run.Flags().StringVar(&inline.Model, "model", "", "Agent model")
	run.Flags().StringVar(&inline.Reply, "reply", "", "Notification destination")
	run.Flags().StringVar(&inline.Issue, "issue", "", "Issue key")
	run.Flags().IntVar(&inline.Timeout, "timeout", 0, "Timeout in minutes")
	run.Flags().StringArrayVar(&accepts, "accept", nil, "Acceptance item (repeatable)")
	run.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		inv := dispatch.Inventory(cfg, "")
		selected := *host
		if selected == "" {
			selected, err = dispatch.SelectHost(cfg, inv)
			if err != nil {
				return err
			}
		}
		task, err := dispatchMessage(cmd, args[1:])
		if err != nil {
			return err
		}
		used, err := inventoryNames(inv)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+err.Error()+"; name collisions on those hosts are unknown; target admission still checks the name")
		}

		o := inline
		o.Repo = args[0]
		o.Host = selected
		o, text, err := dispatch.Inline(o, task, accepts, used, time.Now())
		if err != nil {
			return err
		}
		h := cfg.Hosts[selected]
		fmt.Fprintln(cmd.OutOrStdout(), "Selected host: "+selected)
		return startDispatch(cmd, cfg, h, o, text, *configPath)
	}
	return run
}

func inventoryNames(inv map[string]dispatch.Response) (map[string]bool, error) {
	used := map[string]bool{}
	var unknown []string
	for _, n := range dispatch.SortedHosts(inv) {
		r := inv[n]
		if r.Error != "" {
			unknown = append(unknown, n)
			continue
		}
		for _, name := range r.Names {
			used[name] = true
		}
		for _, row := range r.Rows {
			used[row.Name] = true
		}
	}
	if len(unknown) > 0 {
		return used, fmt.Errorf("cannot verify run names on unreachable hosts: %v", unknown)
	}
	return used, nil
}
