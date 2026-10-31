package main

import (
	"context"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
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
		used := map[string]bool{}
		for _, r := range inv {
			for _, row := range r.Rows {
				used[row.Name] = true
			}
		}
		// Local records without metadata also reserve names.
		if cfg.Hosts[selected].Local {
			entries, _ := os.ReadDir(dispatch.Root())
			for _, entry := range entries {
				used[entry.Name()] = true
			}
		}
		o := inline
		o.Repo = args[0]
		o.Host = selected
		o, text, err := dispatch.Inline(o, task, accepts, used, time.Now())
		if err != nil {
			return err
		}
		h := cfg.Hosts[selected]
		q := dispatch.Request{Action: "start", Name: o.Name, Order: text, Meta: dispatch.Meta{Order: o, Namespace: h.Namespace}, Cap: h.Cap, Workspace: cfg.Workspace}
		if h.Local && *configPath != "" {
			q.Meta.ConfigPath, err = filepath.Abs(*configPath)
			if err != nil {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		response, err := dispatch.RPC(ctx, h, q)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), response.Message)
		return nil
	}
	return run
}
