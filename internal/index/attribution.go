package index

import (
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/trace"
)

type attribution struct{ sender, order, issue, instance, plan, rfc, pr string }

// sessionContext joins a worker session row to the context bundle its
// dispatch attempt was given. It reads per session, since attempts share one
// worktree and each attempt may be given a different bundle.
func sessionContext(row *trace.SessionRow) {
	row.ContextDigest = dispatch.ContextAttribution(row.Workdir, row.SessionID)
}

// Cache path reads per unit, including misses. The optional attribution hook
// supplies additional fields; an order's explicit issue takes precedence.
func unitAttribution(hook func(trace.Turn) (string, string, string, string, string)) func(trace.Turn) attribution {
	paths := map[string]attribution{}
	return func(t trace.Turn) attribution {
		a, ok := paths[t.Workdir]
		if !ok {
			a.order, a.issue = dispatch.Attribution(t.Workdir)
			a.sender = dispatch.SenderAttribution(t.Workdir)
			paths[t.Workdir] = a
		}
		if hook != nil {
			var issue string
			a.instance, a.plan, a.rfc, issue, a.pr = hook(t)
			if a.issue == "" {
				a.issue = issue
			}
		}
		return a
	}
}
