package daemon

import (
	"context"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/peers/compat"
)

func observePeerPresence(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	var previous map[string]compat.LocalPeer
	for {
		current, err := compat.LocalPeers(ctx)
		if err == nil {
			peerTransitions(previous, current, func(event string, peer compat.LocalPeer) {
				hooks.Meta(ctx, event, map[string]any{"peer": peer.Address, "agent": peer.Agent}, "")
			})
			previous = current
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func peerTransitions(previous, current map[string]compat.LocalPeer, emit func(string, compat.LocalPeer)) {
	// The first successful sample establishes a baseline after daemon startup.
	if previous == nil {
		return
	}
	for id, peer := range previous {
		if _, present := current[id]; !present {
			emit("peer-offline", peer)
		}
	}
	for id, peer := range current {
		if _, present := previous[id]; !present {
			emit("peer-online", peer)
		}
	}
}
