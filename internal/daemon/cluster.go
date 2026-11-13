package daemon

import (
	"context"
	"log/slog"
	"os"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
)

func startOptionalCluster(ctx context.Context, cache *secrets.Cache, logger *slog.Logger) func() {
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
	stop, e := cluster.Serve(ctx, s)
	if e != nil {
		logger.Warn("cluster local endpoint unavailable; daemon continues")
		s.Close()
		return func() {}
	}
	return func() { stop(); s.Close() }
}
