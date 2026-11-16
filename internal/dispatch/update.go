package dispatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/update"
)

// UpdateHost moves this host to another build (cmd/hev sets it). by names
// the authenticated caller for the host's update history.
var UpdateHost func(ctx context.Context, q update.Request, by string) (update.Report, error)

// HostChannel is this host's configured update channel (cmd/hev sets it).
var HostChannel func() string

// UpdateTimeout bounds one host's update: download, switch and restart.
const UpdateTimeout = 10 * time.Minute

// handleUpdate is the owner's "update" request. Only the channel, the
// version and the check flag cross from the request: the host reads where
// to fetch from, what to verify against and what to run afterwards from its
// own binary and config. A sender cannot supply a binary, a URL or a digest.
func handleUpdate(q Request) (Response, error) {
	r := Response{}
	if UpdateHost == nil {
		return r, fmt.Errorf("this hev cannot update itself")
	}
	req := update.Request{Channel: q.Channel, To: q.To, Check: q.Check, SaveChannel: q.Channel}
	if req.Channel != "" && !update.ValidChannel(req.Channel) {
		return r, fmt.Errorf("unknown channel %q (want release or edge)", req.Channel)
	}
	if req.To != "" {
		if _, err := update.NormalizeVersion(req.To); err != nil {
			return r, err
		}
	}
	by := q.Meta.Sender
	if by == "" {
		by = LocalSender()
	}
	ctx, cancel := context.WithTimeout(context.Background(), UpdateTimeout)
	defer cancel()
	rep, err := UpdateHost(ctx, req, "dispatch:"+by)
	r.Update = &rep
	return r, err
}

// UpdateResult is one host's answer to `hev dispatch hosts update`.
type UpdateResult struct {
	Before string         `json:"before,omitempty"`
	After  string         `json:"after,omitempty"`
	Report *update.Report `json:"report,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// UpdateHosts asks each selected host to update itself, in parallel.
func UpdateHosts(ctx context.Context, c Config, hosts []string, channel, to string, check bool) map[string]UpdateResult {
	out := map[string]UpdateResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range hosts {
		h, ok := c.Hosts[n]
		if !ok {
			out[n] = UpdateResult{Error: "unknown host"}
			continue
		}
		wg.Add(1)
		go func(n string, h Host) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, UpdateTimeout+time.Minute)
			defer cancel()
			r, err := RPC(ctx, h, Request{Action: "update", Channel: channel, To: to, Check: check})
			res := UpdateResult{Before: r.Version, Report: r.Update}
			if r.Update != nil {
				res.Before, res.After = r.Update.Before, r.Update.After
			}
			if err != nil {
				res.Error = UpdateError(n, r, err).Error()
			}
			mu.Lock()
			out[n] = res
			mu.Unlock()
		}(n, h)
	}
	wg.Wait()
	return out
}

// UpdateError explains a failed update request. A host whose hev predates
// the request answers it as an unnamed run ("invalid name") or an unknown
// action; that is said plainly, with the manual step.
func UpdateError(host string, r Response, err error) error {
	msg := strings.TrimSpace(err.Error())
	if r.Update == nil && (msg == "invalid name" || msg == "unknown action") {
		v := r.Version
		if v == "" {
			v = "an older version"
		}
		return fmt.Errorf("%s runs hev %s, which cannot update itself on request; move it by hand once: unpack a current release archive over the hev on %s's PATH and restart its daemon (hevkit.com/docs/updating, the first move); after that it answers this command", host, v, host)
	}
	return err
}
