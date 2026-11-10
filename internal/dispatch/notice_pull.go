package dispatch

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// NoticePull is the owner side of the pull route, shared by `hev orchestrate
// notices pull`, `hev dispatch notices pull` and hevd's poll. Tests replace
// the remote call and the local inbox deliveries.
type NoticePull struct {
	RPC           func(context.Context, Host, Request) (Response, error)
	DeliverOwner  OrchestratorNotifier
	DeliverMember func(context.Context, string, OrchestratorNotice) error
	Out           io.Writer
}

// NewNoticePull delivers into the local peers inbox and reports to out.
func NewNoticePull(out io.Writer) NoticePull {
	return NoticePull{RPC: RPC, DeliverOwner: SendOrchestratorNotice, DeliverMember: DeliverMemberNotice, Out: out}
}

// Owner delivers pending orchestrator owner notices from host, then acks the
// delivered ones. Delivery comes before the ack: a lost ack redelivers the
// same stable ID, which the local inbox deduplicates. Like the push route,
// each recipient gets its notices oldest first and stops at its first failure,
// so one undeliverable recipient does not hold the others.
func (p NoticePull) Owner(ctx context.Context, host string, h Host, name string) (int, error) {
	r, e := p.RPC(ctx, h, Request{Action: "orchestrator-notices", Name: name})
	if e != nil {
		return 0, e
	}
	var acked []string
	var failed error
	stale := 0
	blocked := map[string]bool{}
	for _, n := range r.Notices {
		if blocked[n.To] {
			continue
		}
		if e := p.DeliverOwner(ctx, n); e != nil {
			blocked[n.To] = true
			if failed == nil {
				failed = fmt.Errorf("deliver %s: %w", n.ID, e)
			}
			continue
		}
		if n.Status == "stale" {
			stale++
		}
		acked = append(acked, n.ID)
	}
	if len(acked) > 0 {
		if _, e = p.RPC(ctx, h, Request{Action: "orchestrator-notice-ack", IDs: acked}); e != nil {
			return 0, fmt.Errorf("delivered %d notices; ack failed, the next pull redelivers them: %w", len(acked), e)
		}
	}
	if len(acked) > 0 || failed == nil {
		note := ""
		if stale > 0 {
			note = fmt.Sprintf(" (%d stale replies)", stale)
		}
		fmt.Fprintf(p.Out, "delivered %d notices from %s%s\n", len(acked), host, note)
	}
	return len(acked), failed
}

// Member delivers before acking. A lost ack redelivers the same stable ID,
// which the local inbox deduplicates. Host data is validated by
// LocalMemberNotice and addressed from NAME/d-RUN, NAME being the local
// alias. A notice without a reply session, or whose session has no
// registered inbox here, is printed instead.
func (p NoticePull) Member(ctx context.Context, name string, h Host) error {
	for {
		r, e := p.RPC(ctx, h, Request{Action: "member-notices"})
		if e != nil {
			return e
		}
		var acked []string
		var failed error
		for _, raw := range r.Notices {
			n, from, e := LocalMemberNotice(name, raw)
			if e != nil {
				// Acked so a malformed or hostile entry cannot block the queue.
				fmt.Fprintf(p.Out, "%s: rejected notice: %v\n", name, e)
				acked = append(acked, raw.ID)
				continue
			}
			if n.To != "" {
				failed = p.DeliverMember(ctx, from, n)
			}
			if n.To == "" || failed != nil && noLocalInbox(failed) {
				reason := "no reply session"
				if raw.To != "" && n.To == "" {
					reason = fmt.Sprintf("reply %q is not a local session", raw.To)
				} else if failed != nil {
					reason = "no inbox for " + n.To
				}
				fmt.Fprintf(p.Out, "%s: %s (%s)\n", name, n.Message, reason)
				failed = nil
			}
			if failed != nil {
				failed = fmt.Errorf("deliver %s: %w", n.ID, failed)
				break
			}
			acked = append(acked, raw.ID)
		}
		if len(acked) > 0 {
			if _, e = p.RPC(ctx, h, Request{Action: "member-notice-ack", IDs: acked}); e != nil {
				return fmt.Errorf("delivered %d notices; ack failed, the next pull redelivers them: %w", len(acked), e)
			}
			fmt.Fprintf(p.Out, "%s: delivered %d notices\n", name, len(acked))
		}
		// The host returns bounded batches; continue only after a full one.
		if failed != nil || len(acked) == 0 || len(r.Notices) < memberNoticeBatch {
			return failed
		}
	}
}

// noLocalInbox matches peers send refusals that no retry here can fix.
func noLocalInbox(e error) bool {
	s := e.Error()
	return strings.Contains(s, "requires a registered inbox") || strings.Contains(s, "not found or ambiguous")
}

// NoticeSource is one host the poll pulls from. A joined host holds this
// machine's member notices; any other configured host is reached with owner
// access and holds orchestrator owner notices.
type NoticeSource struct {
	Name   string
	Host   Host
	Member bool
}

// NoticeSources lists the hosts hevd polls: joined hosts for member notices
// and every other remote host in the dispatch config for owner notices. A
// local host is skipped; its orchestrators reach this machine's inbox
// directly.
func NoticeSources(cfg Config, joined []string) []NoticeSource {
	out := []NoticeSource{}
	for n, h := range cfg.Hosts {
		member := false
		for _, j := range joined {
			member = member || j == n
		}
		if h.Local && !member {
			continue
		}
		out = append(out, NoticeSource{Name: n, Host: h, Member: member})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Sources pulls every source concurrently, each under its own timeout, so a
// host that hangs costs only its own deadline. It returns each source's
// error by host name.
func (p NoticePull) Sources(ctx context.Context, sources []NoticeSource, timeout time.Duration) map[string]error {
	out := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	w := &syncWriter{w: p.Out}
	for _, s := range sources {
		wg.Add(1)
		go func(s NoticeSource) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			q := p
			q.Out = w
			var e error
			if s.Member {
				e = q.Member(ctx, s.Name, s.Host)
			} else {
				_, e = q.Owner(ctx, s.Name, s.Host, "")
			}
			if e == nil && ctx.Err() != nil {
				e = ctx.Err()
			}
			mu.Lock()
			out[s.Name] = e
			mu.Unlock()
		}(s)
	}
	wg.Wait()
	return out
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return len(b), nil
	}
	return s.w.Write(b)
}
