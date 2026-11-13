package cluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/hev/kit/internal/secrets"
)

type KeyLoader func(context.Context, string) ([]byte, time.Time, error)

// CacheKeyLoader shares hevd's existing cache; no IPC recursion or direct fallback.
func CacheKeyLoader(cache *secrets.Cache) KeyLoader {
	return func(ctx context.Context, ref string) ([]byte, time.Time, error) {
		uri, e := secrets.ExpandRef(ref)
		if e != nil {
			return nil, time.Time{}, errors.New("cluster key reference unavailable")
		}
		values, e := cache.ResolveAs(ctx, "hevd cluster", []secrets.Ref{{Name: "HEV_CLUSTER_KEY", URI: uri, TTL: 24 * time.Hour}})
		if e != nil {
			reset := secrets.CooldownReset(e)
			for _, p := range cache.LocalStatus().Providers {
				if p.Reset.After(reset) {
					reset = p.Reset
				}
			}
			return nil, reset, errors.New("cluster key read failed")
		}
		b, e := base64.StdEncoding.DecodeString(values["HEV_CLUSTER_KEY"])
		if e != nil || len(b) != 32 {
			return nil, time.Time{}, errors.New("cluster key must be base64 encoding of 32 bytes")
		}
		return b, time.Time{}, nil
	}
}

type Status struct {
	CountersReady bool                                    `json:"counters_ready"`
	Rolling       map[string]map[string]map[string]uint64 `json:"rolling"`
	Enabled       bool                                    `json:"enabled"`
	Running       bool                                    `json:"running"`
	Bound         string                                  `json:"bound"`
	KeyLoaded     bool                                    `json:"key_loaded"`
	Error         string                                  `json:"error,omitempty"`
	KeyRetry      time.Time                               `json:"key_retry"`
	SeedRetry     time.Time                               `json:"seed_retry"`
	SeedState     string                                  `json:"seed_state"`
	Counts        map[string]int                          `json:"members"`
	Counters      json.RawMessage                         `json:"counters"`
}
type Service struct {
	mu        sync.Mutex
	cfg       *Config
	node      *Node
	facts     Facts
	loader    KeyLoader
	logger    *slog.Logger
	next      time.Time
	error     string
	keyLoaded bool
	stop      context.CancelFunc
	done      chan struct{}
}

// NewService starts no network or local socket when cfg is nil. Key failures
// are isolated in its worker and cannot stop capture, dispatch or secrets IPC.
func NewService(ctx context.Context, cfg *Config, loader KeyLoader, facts Facts, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{cfg: cfg, loader: loader, facts: facts, logger: logger, stop: cancel, done: make(chan struct{})}
	if cfg == nil {
		close(s.done)
		return s
	}
	go func() {
		defer close(s.done)
		for {
			next := s.attempt(ctx, time.Now())
			if next.IsZero() {
				return
			}
			timer := time.NewTimer(max(time.Millisecond, time.Until(next)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return s
}

// attempt is clock-injectable for cooldown tests. Errors are labels only.
func (s *Service) attempt(ctx context.Context, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg == nil || s.node != nil {
		return time.Time{}
	}
	if now.Before(s.next) {
		return s.next
	}
	s.error = "cluster key loading"
	s.mu.Unlock()
	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	key, reset, e := s.loader(loadCtx, s.cfg.KeyRef)
	cancel()
	s.mu.Lock()
	if ctx.Err() != nil {
		return time.Time{}
	}
	if e == nil {
		s.keyLoaded = true
		s.node, e = Start(*s.cfg, key, s.facts, s.logger)
	}
	if e != nil {
		s.error = "cluster unavailable: key or transport startup failed"
		s.next = now.Add(time.Minute)
		if reset.After(s.next) {
			s.next = reset
		}
		if s.logger != nil {
			s.logger.Warn(s.error)
		}
		return s.next
	}
	s.error = ""
	s.next = time.Time{}
	return time.Time{}
}
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Enabled: s.cfg != nil, KeyLoaded: s.keyLoaded, KeyRetry: s.next, Error: s.error, Counts: map[string]int{}, Counters: json.RawMessage(`{"v":1,"counts":{},"registers":{}}`)}
	if n := s.node; n != nil {
		st.Running = true
		n.Counters.mu.Lock()
		st.CountersReady = n.ready
		n.Counters.mu.Unlock()
		st.Bound = n.Address()
		for _, m := range n.Members() {
			st.Counts[m.State]++
		}
		n.mu.Lock()
		st.SeedRetry = n.next
		st.SeedState = n.lastJoin
		if n.paused {
			st.SeedState = "left"
		}
		n.mu.Unlock()
		st.Counters = n.snapshot()
		var facts counterState
		_ = json.Unmarshal(st.Counters, &facts)
		st.Rolling = map[string]map[string]map[string]uint64{}
		for name := range facts.Counts {
			st.Rolling[name] = map[string]map[string]uint64{"hour": n.RollingPerNode(name, time.Hour), "day": n.RollingPerNode(name, 24*time.Hour)}
		}
	}
	return st
}
func (s *Service) Members() []Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node == nil {
		return []Member{}
	}
	return s.node.Members()
}
func (s *Service) Join(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node == nil {
		return errors.New("cluster is down; inspect hev cluster status")
	}
	return s.node.Join(addr)
}
func (s *Service) Leave() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node == nil {
		return errors.New("cluster is down; inspect hev cluster status")
	}
	return s.node.Leave()
}
func (s *Service) Close() {
	s.stop()
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node != nil {
		_ = s.node.Close(true)
	}
}

// Counters exposes the in-process seam for future daemon consumers. It never
// exposes key material or grants access through local or network RPC.
func (s *Service) Counters() (*Counters, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.node == nil {
		return nil, errors.New("cluster counters unavailable")
	}
	return s.node.Counters, nil
}
