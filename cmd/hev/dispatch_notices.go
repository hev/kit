package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
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

// pullMemberNotices delivers before acking. A lost ack redelivers the same
// stable ID, which the local inbox deduplicates. Host data is validated by
// dispatch.LocalMemberNotice and addressed from NAME/d-RUN, NAME being the
// local alias. A notice without a reply
// session, or whose session has no registered inbox here, is printed instead.
func pullMemberNotices(ctx context.Context, name string, h dispatch.Host, out io.Writer) error {
	for {
		r, e := memberNoticeRPC(ctx, h, dispatch.Request{Action: "member-notices"})
		if e != nil {
			return e
		}
		var acked []string
		var failed error
		for _, raw := range r.Notices {
			n, from, e := dispatch.LocalMemberNotice(name, raw)
			if e != nil {
				// Acked so a malformed or hostile entry cannot block the queue.
				fmt.Fprintf(out, "%s: rejected notice: %v\n", name, e)
				acked = append(acked, raw.ID)
				continue
			}
			if n.To != "" {
				failed = deliverMemberNotice(ctx, from, n)
			}
			if n.To == "" || failed != nil && noLocalInbox(failed) {
				reason := "no reply session"
				if raw.To != "" && n.To == "" {
					reason = fmt.Sprintf("reply %q is not a local session", raw.To)
				} else if failed != nil {
					reason = "no inbox for " + n.To
				}
				fmt.Fprintf(out, "%s: %s (%s)\n", name, n.Message, reason)
				failed = nil
			}
			if failed != nil {
				failed = fmt.Errorf("deliver %s: %w", n.ID, failed)
				break
			}
			acked = append(acked, raw.ID)
		}
		if len(acked) > 0 {
			if _, e = memberNoticeRPC(ctx, h, dispatch.Request{Action: "member-notice-ack", IDs: acked}); e != nil {
				return fmt.Errorf("delivered %d notices; ack failed, the next pull redelivers them: %w", len(acked), e)
			}
			fmt.Fprintf(out, "%s: delivered %d notices\n", name, len(acked))
		}
		// The host returns bounded batches; continue only after a full one.
		if failed != nil || len(acked) == 0 || len(r.Notices) < 50 {
			return failed
		}
	}
}

// noLocalInbox matches peers send refusals that no retry here can fix.
func noLocalInbox(e error) bool {
	s := e.Error()
	return strings.Contains(s, "requires a registered inbox") || strings.Contains(s, "not found or ambiguous")
}
