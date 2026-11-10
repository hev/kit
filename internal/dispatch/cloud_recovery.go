package dispatch

import (
	"context"
	"errors"
)

// RecoverCloudHosts is a single startup pass. A service can repeat it after
// process failure; detached sweepers serialize against normal submitters.
func RecoverCloudHosts(ctx context.Context, c Config) error {
	var result error
	for _, h := range c.Hosts {
		if !IsCloudHost(h) || h.Type == "hetzner" && h.Mode != "run" {
			continue
		}
		// Inventory reconciles confirmed terminal leftovers before queue admission.
		_, e := boatRPC(ctx, h, Request{Action: "list"})
		result = errors.Join(result, e)
		entries, e := cloudQueueEntries(h)
		result = errors.Join(result, e)
		if e == nil && len(entries) > 0 {
			result = errors.Join(result, cloudQueueLauncher(h))
		}
	}
	return result
}
