package daemon

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
)

// noticePullTimeout bounds one host's pull: an RPC, the local deliveries and
// the ack. A hung host costs this much and never delays the others.
const noticePullTimeout = 45 * time.Second

// noticePoll is hevd's half of the pull route. Hosts that cannot reach this
// machine keep owner and member notices queued; hevd pulls and delivers them
// with the same code and trust checks as `hev orchestrate notices pull
// --deliver` and `hev dispatch notices pull`.
type noticePoll struct {
	pull    dispatch.NoticePull
	load    func() (dispatch.Config, error)
	joined  func() ([]string, error)
	timeout time.Duration
	logger  *slog.Logger
	// last holds each host's last error, so a host that stays unreachable
	// is logged when it changes, not every interval.
	last map[string]string
}

func newNoticePoll(logger *slog.Logger) *noticePoll {
	return &noticePoll{
		pull:    dispatch.NewNoticePull(logWriter{logger}),
		load:    func() (dispatch.Config, error) { return dispatch.Load(dispatch.ConfigPath("")) },
		joined:  joinedHostNames,
		timeout: noticePullTimeout,
		logger:  logger,
		last:    map[string]string{},
	}
}

func joinedHostNames() ([]string, error) {
	joined, err := nodes.Default().Joined()
	names := make([]string, 0, len(joined))
	for _, j := range joined {
		names = append(names, j.Name)
	}
	return names, err
}

// once pulls every host once. The dispatch config is read each time, so a
// host added later is polled without a restart.
func (p *noticePoll) once(ctx context.Context) {
	cfg, err := p.load()
	if err != nil {
		p.report("dispatch config", err)
		return
	}
	p.report("dispatch config", nil)
	joined, err := p.joined()
	if err != nil {
		p.report("joined hosts", err)
	}
	for host, err := range p.pull.Sources(ctx, dispatch.NoticeSources(cfg, joined), p.timeout) {
		p.report(host, err)
	}
}

func (p *noticePoll) report(key string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if p.last[key] == msg {
		return
	}
	p.last[key] = msg
	if err != nil {
		p.logger.Warn("notice pull", "host", key, "err", err)
	} else {
		p.logger.Info("notice pull recovered", "host", key)
	}
}

// run pulls at once and then every interval until ctx ends.
func (p *noticePoll) run(ctx context.Context, interval time.Duration) {
	if interval < p.timeout {
		p.timeout = interval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		p.once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// startNoticePoll runs the poll in the background; the returned function
// waits for it to stop after ctx ends.
func startNoticePoll(ctx context.Context, interval time.Duration, logger *slog.Logger) func() {
	if interval <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		newNoticePoll(logger).run(ctx, interval)
	}()
	return func() { <-done }
}

// logWriter records the pull's report lines in hevd's log.
type logWriter struct{ logger *slog.Logger }

func (w logWriter) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line != "" {
			w.logger.Info("notice pull", "msg", line)
		}
	}
	return len(b), nil
}
