package daemon

import (
	"context"
	"log/slog"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/cron"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/metrics"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/version"
)

func startMetrics(
	ctx context.Context,
	cfg *metrics.Config,
	cache *secrets.Cache,
	clusterSvc *cluster.Service,
	membership cron.Membership,
	cronRunner cron.Runner,
	jobs []cron.Job,
	logger *slog.Logger,
) func() {
	if cfg == nil || !cfg.Enabled {
		return func() {}
	}
	src := metrics.Sources{
		Self:           membership.Self,
		LocalVersion:   version.Version,
		ClusterEnabled: membership.Enabled,
		Orchestrators:  dispatch.ListOrchestrators,
	}
	if clusterSvc != nil {
		src.Members = func(context.Context) []cluster.Member { return clusterSvc.Members() }
		src.PlacementMembers = func(context.Context) []cluster.Member { return clusterSvc.CronMembers() }
	}
	if cache != nil {
		src.SecretsStatus = func() *secrets.Status {
			st := cache.LocalStatus()
			return &st
		}
	}
	src.CronFailing = func(ctx context.Context) (int, error) {
		failing := 0
		for _, j := range jobs {
			d, err := cronRunner.Inspect(ctx, j, false)
			if err != nil {
				continue
			}
			enabled := j.Enabled == nil || *j.Enabled
			if d.Status == "failing" || (d.Status == "running" && enabled && !d.State.Paused && d.Reason == "" && d.State.ConsecutiveFailures > 0) {
				failing++
			}
		}
		return failing, nil
	}
	exp, err := metrics.Start(ctx, *cfg, src, logger)
	if err != nil {
		logger.Warn("metrics exporter disabled; capture continues", "err", err)
		return func() {}
	}
	return exp.Stop
}
