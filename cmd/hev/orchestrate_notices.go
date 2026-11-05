package main

import (
	"encoding/json"
	"fmt"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

// Tests replace the remote call and the local inbox delivery.
var (
	orchestrateNoticeRPC = dispatch.RPC
	deliverPulledNotice  = dispatch.SendOrchestratorNotice
)

func newOrchestrateNoticesCommand() *cobra.Command {
	c := &cobra.Command{Use: "notices", Short: "Pull owner notices from hosts that cannot reach the owner"}
	var host, name string
	var deliver bool
	pull := &cobra.Command{Use: "pull --host HOST [--deliver]", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, e := dispatch.Load(dispatch.ConfigPath(""))
		if e != nil {
			return e
		}
		h, ok := cfg.Hosts[host]
		if !ok {
			return fmt.Errorf("unknown host %s", host)
		}
		r, e := orchestrateNoticeRPC(cmd.Context(), h, dispatch.Request{Action: "orchestrator-notices", Name: name})
		if e != nil {
			return e
		}
		if !deliver {
			b, e := json.MarshalIndent(r.Notices, "", "  ")
			if e != nil {
				return e
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		// Deliver before acking. A lost ack redelivers the same stable ID,
		// which the local inbox deduplicates.
		var acked []string
		var failed error
		for _, n := range r.Notices {
			if failed = deliverPulledNotice(cmd.Context(), n); failed != nil {
				failed = fmt.Errorf("deliver %s: %w", n.ID, failed)
				break
			}
			acked = append(acked, n.ID)
		}
		if len(acked) > 0 {
			if _, e = orchestrateNoticeRPC(cmd.Context(), h, dispatch.Request{Action: "orchestrator-notice-ack", IDs: acked}); e != nil {
				return fmt.Errorf("delivered %d notices; ack failed, the next pull redelivers them: %w", len(acked), e)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "delivered %d notices from %s\n", len(acked), host)
		return failed
	}}
	pull.Flags().StringVar(&host, "host", "", "Configured host running the orchestrators")
	pull.Flags().StringVar(&name, "name", "", "Only this orchestrator")
	pull.Flags().BoolVar(&deliver, "deliver", false, "Deliver into the local peers inbox, then acknowledge")
	_ = pull.MarkFlagRequired("host")
	c.AddCommand(pull)
	return c
}
