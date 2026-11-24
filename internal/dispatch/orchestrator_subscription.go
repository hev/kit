package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/hostlabel"
)

func init() { attention.DefaultForward = forwardOrchestratorEvent }

func forwardOrchestratorEvent(ctx context.Context, event attention.EventSummary, config attention.Config) {
	if !validEventSummary(event) {
		return
	}
	self := notificationHost()
	entries, _ := os.ReadDir(OrchestratorConfigDir())
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if name, ok := strings.CutSuffix(entry.Name(), ".md"); ok && !entry.IsDir() {
			_ = receiveOrchestratorEvent(ctx, name, event, false)
		}
	}

	var hosts map[string]Host
	// Bound the number of simultaneous SSH requests and share a single
	// deadline so a slow route cannot hold up another destination.
	const maxRemoteTargets = 16
	var remote sync.WaitGroup
	defer remote.Wait()
	type target struct{ host, name string }
	sent := make(map[target]bool)
	for _, route := range config.Orchestrators {
		if ctx.Err() != nil {
			return
		}
		if !ValidName(route.Name) || validateEventSubscription(route.Subscription) != nil || !wantsEvent(route.Subscription, event) {
			continue
		}
		if route.Host == "" || hostlabel.Same(route.Host, self) {
			// A matching local declaration already handled this event.
			if spec, err := LoadOrchestrator(route.Name); err == nil && spec.Subscribe == nil {
				_ = receiveOrchestratorEvent(ctx, route.Name, event, true)
			}
			continue
		}
		if !ValidLabel(route.Host) || isOwnEvent(route.Name, route.Host, event) {
			continue
		}
		if hosts == nil {
			cfg, err := Load(ConfigPath(""))
			if err != nil {
				return
			}
			hosts = cfg.Hosts
		}
		key := target{route.Host, route.Name}
		if host, ok := hosts[route.Host]; ok && !host.Local && host.SSH != "" && !sent[key] && len(sent) < maxRemoteTargets {
			sent[key] = true
			remote.Go(func() {
				_, _ = rpcTransport(ctx, host, Request{Action: "orchestrator-event", Name: "o-" + route.Name, Event: &event})
			})
		}
	}
}

func validEventSummary(event attention.EventSummary) bool {
	// The source fields are identifiers. Refuse arbitrary prose, paths,
	// control characters and oversized values before writing an inbox line.
	return attention.ValidKind(event.Event) && attention.ValidSource(event.Source) &&
		len(event.Host) <= 63 && ValidLabel(event.Host) &&
		len(event.Name) <= 128 && ValidLabel(event.Name) &&
		(event.State == "" || (len(event.State) <= 64 && ValidLabel(event.State)))
}

func wantsEvent(sub attention.Subscription, event attention.EventSummary) bool {
	if !sub.Wants(event) {
		return false
	}
	for _, host := range sub.Hosts {
		if hostlabel.Same(host, event.Host) {
			return true
		}
	}
	return len(sub.Hosts) == 0
}

func isOwnEvent(name, host string, event attention.EventSummary) bool {
	return event.Source == attention.SourceOrchestrator && event.Name == name && hostlabel.Same(event.Host, host)
}

func receiveOrchestratorEvent(ctx context.Context, name string, event attention.EventSummary, explicitLocal bool) error {
	spec, err := LoadOrchestrator(name)
	if err != nil || spec.Name != name {
		return errors.New("event subscription unavailable")
	}
	if !acceptsOrchestratorEvent(spec, event, explicitLocal) {
		return nil
	}
	// Use the same lock as inbox consumption. Delivery is best-effort when
	// it is busy, and never launches a supervisor or creates a steer receipt.
	unlock, err := lockEventInbox(ctx)
	if err != nil {
		return errors.New("event inbox unavailable")
	}
	defer unlock()
	if exists(filepath.Join(OrchestratorDir(name), "scope-owner.json")) {
		return errors.New("event subscription unavailable")
	}
	// Apply writes the new spec under this lock. Recheck in case it changed
	// after discovery, before committing the message to its inbox.
	spec, err = LoadOrchestrator(name)
	if err != nil || spec.Name != name {
		return errors.New("event subscription unavailable")
	}
	if !acceptsOrchestratorEvent(spec, event, explicitLocal) {
		return nil
	}
	line := fmt.Sprintf("[event] %s %s %s/%s", event.Event, event.Source, event.Host, event.Name)
	if event.State != "" {
		line += " " + event.State
	}
	if err := appendOrchestratorInbox(name, line); err != nil {
		return errors.New("event inbox unavailable")
	}
	return nil
}

func acceptsOrchestratorEvent(spec OrchestratorSpec, event attention.EventSummary, explicitLocal bool) bool {
	if isOwnEvent(spec.Name, noticeHost(spec), event) {
		return false
	}
	if spec.Subscribe == nil {
		return explicitLocal
	}
	return wantsEvent(*spec.Subscribe, event)
}

func lockEventInbox(ctx context.Context) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		unlock, err := tryLockRoot()
		if err == nil {
			return unlock, nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
	return nil, ctx.Err()
}
