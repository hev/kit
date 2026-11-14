package daemon

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/peers/compat"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/update"
)

// captureObservation is updated only by real capture work. Reading it has no IO.
type captureObservation struct {
	sync.Mutex
	running    bool
	cycle      time.Time
	interval   time.Duration
	at, upload time.Time
	backlog    uint32
	class      string
	layer      cluster.CredentialFact
}

var captureHealth captureObservation

func (o *captureObservation) result(now time.Time, err error, remote bool) {
	o.Lock()
	defer o.Unlock()
	o.at = now
	if err == nil {
		o.class = ""
		if remote {
			o.upload = now
			o.layer = cluster.CredentialFact{At: now, State: "ok"}
		}
		return
	}
	o.class = errorClass(err)
	if remote && o.class == "auth" {
		o.layer = cluster.CredentialFact{At: now, State: "failing"}
	}
}
func errorClass(err error) string {
	var h *layer.HTTPError
	var n net.Error
	var path *os.PathError
	switch {
	case errors.As(err, &h):
		switch h.Status {
		case 401, 403:
			return "auth"
		case 429:
			return "rate-limit"
		default:
			return "other"
		}
	case errors.As(err, &n):
		return "network"
	case errors.As(err, &path), errors.Is(err, os.ErrPermission), errors.Is(err, os.ErrNotExist):
		return "storage"
	default:
		return "other"
	}
}

func credentialFacts(status secrets.Status, now time.Time) *cluster.CredentialFacts {
	c := &cluster.CredentialFacts{At: now, Items: map[string]cluster.CredentialFact{}}
	for _, p := range status.Providers {
		if !p.Configured || p.Scheme != "op" && p.Scheme != "azkv" {
			continue
		}
		v := cluster.CredentialFact{At: p.LastAt, State: "unknown"}
		switch p.LastOutcome {
		case "rate_limited":
			v.State = "rate-limited"
		case "ok":
			v.State = "ok"
		case "error":
			v.State = "failing"
		}
		c.Items[p.Scheme] = v
	}
	return c
}
func collectNodeFacts(cache *secrets.Cache, dispatchEnabled bool, now time.Time) cluster.NodeFacts {
	f := cluster.NodeFacts{Version: 1, At: now}
	if dispatchEnabled {
		cap, running, free, queue, e := dispatch.LocalCapacity()
		if e == nil {
			f.Dispatch = &cluster.DispatchFacts{At: now, Cap: uint32(cap), Running: uint32(running), Free: uint32(free), Queue: uint32(queue)}
		}
	}
	counts, sessionErr := compat.LocalCounts()
	if sessionErr == nil {
		f.Sessions = &cluster.SessionFacts{At: now, Harness: map[string]cluster.SessionCount{}, Role: map[string]cluster.SessionCount{}}
		for k, v := range counts.Harness {
			f.Sessions.Harness[k] = cluster.SessionCount{Working: v[0], Idle: v[1]}
		}
		for k, v := range counts.Role {
			f.Sessions.Role[k] = cluster.SessionCount{Working: v[0], Idle: v[1]}
		}
	}
	home, _ := os.UserHomeDir()
	u := update.Updater{Home: home}
	if o, e := u.Observe(); e == nil {
		f.Build = &cluster.BuildFacts{At: now, Commit: o.Commit, Staged: o.Staged, LastCheck: o.LastCheck.UTC(), Outcome: o.Outcome}
	}
	f.Credentials = credentialFacts(cache.LocalStatus(), now)
	if cfg, e := loadConfig(true); e == nil && (cfg.LayerAPIKey != "" || layerReferenceConfigured()) {
		f.Credentials.Items["layer"] = cluster.CredentialFact{State: "unknown"}
	}
	for _, h := range []string{"claude", "codex", "pi", "opencode"} {
		if _, e := exec.LookPath(h); e == nil {
			f.Credentials.Items[h] = cluster.CredentialFact{State: "unknown"}
		}
	}
	captureHealth.Lock()
	defer captureHealth.Unlock()
	if !captureHealth.at.IsZero() {
		f.Capture = &cluster.CaptureFacts{At: captureHealth.at, Running: captureHealth.running && captureHealth.recent(now), LastUpload: captureHealth.upload, Backlog: captureHealth.backlog, Error: captureHealth.class}
	}
	if captureHealth.layer.State != "" {
		f.Credentials.Items["layer"] = captureHealth.layer
	}
	finalizeFacts(&f)
	return f
}

func finalizeFacts(f *cluster.NodeFacts) {
	// Concurrent work can finish after sampling began. Keep the record timestamp
	// at least as recent as those observations without inventing a second clock.
	if f.Credentials != nil {
		for _, v := range f.Credentials.Items {
			if v.At.After(f.At) {
				f.At = v.At
			}
		}
	}
	if f.Capture != nil && f.Capture.At.After(f.At) {
		f.At = f.Capture.At
	}
	if f.Build != nil && f.Build.LastCheck.After(f.At) {
		f.At = f.Build.LastCheck
	}
	if f.Capture != nil && f.Capture.LastUpload.After(f.At) {
		f.At = f.Capture.LastUpload
	}
	normalizeFactsTimes(f)
}

func layerReferenceConfigured() bool {
	for _, k := range []string{"HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF", "LAYER_API_KEY_REF", "LAYER_API_KEY_OP_REF"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

func normalizeFactsTimes(f *cluster.NodeFacts) {
	f.At = f.At.UTC()
	if f.Dispatch != nil {
		f.Dispatch.At = f.Dispatch.At.UTC()
	}
	if f.Sessions != nil {
		f.Sessions.At = f.Sessions.At.UTC()
	}
	if f.Build != nil {
		f.Build.At = f.Build.At.UTC()
		f.Build.LastCheck = f.Build.LastCheck.UTC()
	}
	if f.Credentials != nil {
		f.Credentials.At = f.Credentials.At.UTC()
		for k, v := range f.Credentials.Items {
			v.At = v.At.UTC()
			f.Credentials.Items[k] = v
		}
	}
	if f.Capture != nil {
		f.Capture.At = f.Capture.At.UTC()
		f.Capture.LastUpload = f.Capture.LastUpload.UTC()
	}
}

// factsPublisher coalesces sampled changes and minute refreshes independently
// from the secrets usage publisher. Its clock is supplied by the caller.
type factsPublisher struct {
	previous  []byte
	published time.Time
}

func (p *factsPublisher) publish(f cluster.NodeFacts, write func(cluster.NodeFacts) error) error {
	comparable := f
	comparable.At = time.Time{}
	if comparable.Dispatch != nil {
		d := *comparable.Dispatch
		d.At = time.Time{}
		comparable.Dispatch = &d
	}
	if comparable.Sessions != nil {
		d := *comparable.Sessions
		d.At = time.Time{}
		comparable.Sessions = &d
	}
	if comparable.Build != nil {
		d := *comparable.Build
		d.At = time.Time{}
		comparable.Build = &d
	}
	if comparable.Credentials != nil {
		d := *comparable.Credentials
		d.At = time.Time{}
		comparable.Credentials = &d
	}
	if comparable.Capture != nil {
		d := *comparable.Capture
		d.At = time.Time{}
		comparable.Capture = &d
	}
	raw, _ := json.Marshal(comparable)
	if bytes.Equal(p.previous, raw) && f.At.Sub(p.published) < cluster.FactsRefresh {
		return nil
	}
	if e := write(f); e != nil {
		return e
	}
	p.previous = raw
	p.published = f.At
	return nil
}

func (o *captureObservation) recent(now time.Time) bool {
	return !o.cycle.IsZero() && o.interval > 0 && now.Sub(o.cycle) <= 3*o.interval
}
func (o *captureObservation) completed(now time.Time) {
	o.Lock()
	defer o.Unlock()
	o.cycle = now.UTC()
	o.running = true
}
