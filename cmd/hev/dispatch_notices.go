package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
	"github.com/spf13/cobra"
)

// Tests replace the remote call, the local inbox delivery and joined hosts.
var (
	memberNoticeRPC     = dispatch.RPC
	deliverMemberNotice = dispatch.DeliverMemberNotice
	joinedHosts         = func() ([]nodes.Joined, error) { return nodes.Default().Joined() }
)

// newDispatchNoticesCommand mirrors `hev orchestrate notices pull`: the
// teammate's laptop initiates delivery because the host cannot reach it.
func newDispatchNoticesCommand(load func() (dispatch.Config, error), host *string) *cobra.Command {
	c := &cobra.Command{Use: "notices", Short: "Pull your run notices from shared hosts that cannot reach you"}
	var watch time.Duration
	pull := &cobra.Command{Use: "pull [--host HOST] [--watch[=1m]]", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, e := load()
		if e != nil {
			return e
		}
		names := []string{}
		if *host != "" {
			if _, ok := cfg.Hosts[*host]; !ok {
				return fmt.Errorf("host %q is not in the dispatch config; see hev nodes ls", *host)
			}
			names = append(names, *host)
		} else {
			joined, e := joinedHosts()
			if e != nil {
				return e
			}
			for _, j := range joined {
				if _, ok := cfg.Hosts[j.Name]; ok {
					names = append(names, j.Name)
				}
			}
			sort.Strings(names)
			if len(names) == 0 {
				return fmt.Errorf("no joined hosts; use --host or hev nodes join")
			}
		}
		for {
			var failed error
			for _, n := range names {
				if e := pullMemberNotices(cmd.Context(), n, cfg.Hosts[n], cmd.OutOrStdout()); e != nil {
					failed = fmt.Errorf("%s: %w", n, e)
					fmt.Fprintln(cmd.ErrOrStderr(), failed)
				}
			}
			if watch <= 0 {
				return failed
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(watch):
			}
		}
	}}
	pull.Flags().DurationVar(&watch, "watch", 0, "Keep pulling at this interval")
	pull.Flags().Lookup("watch").NoOptDefVal = "1m"
	c.AddCommand(pull)
	return c
}

func pullMemberNotices(ctx context.Context, name string, h dispatch.Host, out io.Writer) error {
	return dispatch.NoticePull{RPC: memberNoticeRPC, DeliverMember: deliverMemberNotice, Out: out}.Member(ctx, name, h)
}
