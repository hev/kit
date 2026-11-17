package daemon

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/peers/compat"
)

func TestCollectActivityLeavesUnknownFieldsOut(t *testing.T) {
	oldD, oldA := localDispatchActivity, localAgentCounts
	t.Cleanup(func() { localDispatchActivity, localAgentCounts = oldD, oldA })
	now := time.Date(2026, 11, 17, 17, 0, 0, 0, time.UTC)

	localDispatchActivity = func() (dispatch.Activity, error) { return dispatch.Activity{HeadlessRuns: 2, Expediters: 1}, nil }
	localAgentCounts = func() (compat.AgentCounts, error) { return compat.AgentCounts{Working: 3, Idle: 4}, nil }
	if a := collectActivity(now); a.Short() != "3w/4i 2r 1x" {
		t.Fatal(a.Short())
	}
	// Windows and tmux-less nodes: no agent fields at all.
	localAgentCounts = func() (compat.AgentCounts, error) { return compat.AgentCounts{}, compat.ErrNoTmux }
	a := collectActivity(now)
	if a.AgentsWorking != nil || a.AgentsIdle != nil || a.Short() != "2r 1x" || !a.At.Equal(now) {
		t.Fatalf("%+v", a)
	}
	localDispatchActivity = func() (dispatch.Activity, error) { return dispatch.Activity{}, errors.New("tmux broken") }
	if a = collectActivity(now); a.HeadlessRuns != nil || a.Expediters != nil {
		t.Fatalf("%+v", a)
	}
}

type sink struct {
	mu    sync.Mutex
	calls int
	fail  int
}

func (s *sink) SetActivity(cluster.Activity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls <= s.fail {
		return errors.New("cluster not running")
	}
	return nil
}

func TestPublishActivityRetriesUntilClusterRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &sink{fail: 1}
	done := make(chan struct{})
	go func() {
		defer close(done)
		publishActivity(ctx, s, 50*time.Millisecond, func(t time.Time) cluster.Activity { return cluster.Activity{At: t} })
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		calls := s.calls
		s.mu.Unlock()
		if calls >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d publishes", calls)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
}
