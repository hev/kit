package daemon

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
)

// StartCluster starts the optional membership sidecar using the daemon cache.
func StartCluster(ctx context.Context, cache *secrets.Cache, logger *slog.Logger) func() {
	cfg, e := cluster.LoadConfig("")
	if e != nil {
		logger.Warn("cluster disabled; daemon continues", "err", e)
		return func() {}
	}
	if cfg == nil {
		return func() {}
	}
	host, _ := os.Hostname()
	enabled, cap := false, 0
	if _, e := os.Stat(dispatch.ConfigPath("")); e == nil {
		if d, e := dispatch.Load(""); e == nil {
			enabled = true
			cap = d.Cap
			if h, ok := d.Hosts[host]; ok && h.Cap > 0 {
				cap = h.Cap
			}
		}
	}
	s := cluster.NewService(ctx, cfg, cluster.CacheKeyLoader(cache), cluster.LocalFacts(cfg.Name, enabled, cap), logger)
	name := cfg.Name
	if name == "" {
		name = host
	}
	cache.AttachCluster(&usageBridge{service: s, node: name})
	publishCtx, cancelPublish := context.WithCancel(ctx)
	publishDone := make(chan struct{})
	go func() {
		defer close(publishDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-publishCtx.Done():
				return
			case <-ticker.C:
				cache.PublishCluster()
			}
		}
	}()
	factsDone := make(chan struct{})
	go func() {
		defer close(factsDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		publisher := factsPublisher{}
		for {
			select {
			case <-publishCtx.Done():
				return
			case <-ticker.C:
				f := collectNodeFacts(cache, enabled, time.Now().UTC())
				if c, e := s.Counters(); e == nil {
					_ = publisher.publish(f, c.SetNodeFacts)
				}
			}
		}
	}()

	stop, e := cluster.Serve(ctx, s)
	if e != nil {
		logger.Warn("cluster local endpoint unavailable; daemon continues")
		cancelPublish()
		<-publishDone
		<-factsDone
		s.Close()
		return func() {}
	}
	return func() { cancelPublish(); <-publishDone; <-factsDone; stop(); s.Close() }
}
