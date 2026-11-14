package kitcapture

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

// Drainer empties the outbox in the background: at once, after every Kick,
// and every interval. After a failure it waits longer each time (up to
// MaxBackoff) before trying again, so a long outage costs one request per
// few minutes, not one per scan.
type Drainer struct {
	Outbox     *Outbox
	Client     *kitapi.Client
	Logger     *slog.Logger
	Interval   time.Duration
	MaxBackoff time.Duration

	kick chan struct{}
	once sync.Once
	mu   sync.Mutex
	last DrainResult
	err  error
}

func (d *Drainer) init() {
	d.once.Do(func() { d.kick = make(chan struct{}, 1) })
}

// Kick asks for a drain soon (a capture cycle just queued rows).
func (d *Drainer) Kick() {
	d.init()
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

// Last is the most recent drain's result and error.
func (d *Drainer) Last() (DrainResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last, d.err
}

// Run drains until ctx ends.
func (d *Drainer) Run(ctx context.Context) {
	d.init()
	interval := d.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	maxBackoff := d.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 5 * time.Minute
	}
	wait := time.Duration(0)
	failures := 0
	for {
		if wait > 0 && !d.sleep(ctx, wait, failures == 0) {
			return
		}
		res, err := d.Outbox.Drain(ctx, d.Client, nil)
		if ctx.Err() != nil {
			return
		}
		d.mu.Lock()
		d.last, d.err = res, err
		d.mu.Unlock()
		switch {
		case err == nil:
			if failures > 0 && d.Logger != nil {
				d.Logger.Info("kit API reachable again; outbox drained", "sent", res.Sent)
			}
			failures, wait = 0, interval
		case errors.Is(err, kitapi.ErrUnauthorized):
			if d.Logger != nil {
				d.Logger.Error("kit API rejected the kit key; captured rows stay queued until `hev login`", "pending", res.Pending)
			}
			failures++
			wait = maxBackoff
		default:
			failures++
			wait = min(maxBackoff, interval<<min(failures, 8))
			if d.Logger != nil {
				d.Logger.Warn("kit API unavailable; rows stay in the outbox", "err", err, "pending", res.Pending, "retry_in", wait)
			}
		}
	}
}

// sleep waits for wait, or less if kicked while healthy; it reports false
// when ctx ended. While backing off from a failure a kick is ignored.
func (d *Drainer) sleep(ctx context.Context, wait time.Duration, healthy bool) bool {
	t := time.NewTimer(wait)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		case <-d.kick:
			if healthy {
				return true
			}
		}
	}
}
