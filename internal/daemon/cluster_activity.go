package daemon

import (
	"context"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/peers/compat"
)

// Seams for fixture tests.
var (
	localDispatchActivity = dispatch.LocalActivity
	localAgentCounts      = compat.LocalAgentCounts
)

// collectActivity builds this node's gossiped summary. Each source that
// cannot answer leaves its fields out rather than reporting 0: Windows and
// tmux-less nodes carry no agent counts.
func collectActivity(now time.Time) cluster.Activity {
	a := cluster.Activity{At: now.UTC()}
	if d, e := localDispatchActivity(); e == nil {
		a.HeadlessRuns, a.Expediters, a.Orchestrators = &d.HeadlessRuns, &d.Expediters, &d.Orchestrators
	}
	if c, e := localAgentCounts(); e == nil {
		a.AgentsWorking, a.AgentsIdle = &c.Working, &c.Idle
	}
	return a
}

type activitySink interface {
	SetActivity(cluster.Activity) error
}

// publishActivity samples soon after start (retrying until the cluster is
// up) and then every interval. Failures are silent: the summary is advisory
// and its age already tells readers it stopped refreshing.
func publishActivity(ctx context.Context, s activitySink, every time.Duration, collect func(time.Time) cluster.Activity) {
	wait := time.Second
	published := false
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if s.SetActivity(collect(time.Now())) == nil {
			published = true
		}
		wait = every
		if !published {
			wait = min(every, 5*time.Second)
		}
		timer.Reset(wait)
	}
}
