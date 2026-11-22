package metrics

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// UDPWriteTimeout bounds dial and each UDP datagram write (<= 2s).
	UDPWriteTimeout = 2 * time.Second
	// MaxDatagramBytes keeps DogStatsD UDP payloads inside Tailscale WireGuard MTU.
	MaxDatagramBytes = 1024

	scrapeTimeout      = 5 * time.Second
	httpShutdownBudget = 3 * time.Second
)

// SendDogStatsD emits allowlisted gauge samples to a validated loopback or
// Tailscale CGNAT UDP address with bounded write deadlines.
func SendDogStatsD(addr string, samples []Sample) error {
	if err := ValidatePrivateAddr(addr, false); err != nil {
		return err
	}
	lines := FormatDogStatsD(samples)
	if len(lines) == 0 {
		return nil
	}
	packets := BatchDogStatsD(lines, MaxDatagramBytes)
	conn, err := net.DialTimeout("udp", addr, UDPWriteTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, pkt := range packets {
		if err := conn.SetWriteDeadline(time.Now().Add(UDPWriteTimeout)); err != nil {
			return err
		}
		if _, err := conn.Write(pkt); err != nil {
			return err
		}
	}
	return nil
}

// PrometheusHandler serves Prometheus text format 0.0.4 on GET/HEAD /metrics.
func PrometheusHandler(collect func(context.Context) []Sample) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), scrapeTimeout)
		defer cancel()
		mu.Lock()
		samples := collect(ctx)
		mu.Unlock()
		body := FormatPrometheus(samples)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, body)
		}
	})
}

// Exporter runs the optional DogStatsD push loop and Prometheus HTTP listener.
type Exporter struct {
	cancel   context.CancelFunc
	server   *http.Server
	promAddr string
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// BoundPrometheusAddr returns the actual listen address when Prometheus is
// enabled (useful when configured with port 0 in tests), or "".
func (e *Exporter) BoundPrometheusAddr() string {
	if e == nil {
		return ""
	}
	return e.promAddr
}

// Start validates cfg and launches the configured DogStatsD loop and/or
// Prometheus /metrics HTTP server. When cfg.Enabled is false, Start returns a
// no-op exporter without starting goroutines or binding sockets.
func Start(ctx context.Context, cfg Config, src Sources, logger *slog.Logger) (*Exporter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return &Exporter{}, nil
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	expCtx, cancel := context.WithCancel(ctx)
	e := &Exporter{cancel: cancel}

	collect := func(c context.Context) []Sample {
		return Collect(c, cfg, src)
	}

	if cfg.PrometheusAddr != "" {
		ln, err := net.Listen("tcp", cfg.PrometheusAddr)
		if err != nil {
			cancel()
			return nil, err
		}
		e.promAddr = ln.Addr().String()
		e.server = &http.Server{
			Handler:           PrometheusHandler(collect),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
			MaxHeaderBytes:    4096,
		}
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			if err := e.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Warn("metrics prometheus server stopped", "err", err)
			}
		}()
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			<-expCtx.Done()
			shutCtx, shutCancel := context.WithTimeout(context.Background(), httpShutdownBudget)
			defer shutCancel()
			_ = e.server.Shutdown(shutCtx)
		}()
	}

	if cfg.DogStatsD != "" {
		interval := cfg.EmitInterval()
		emit := func() {
			emitCtx, emitCancel := context.WithTimeout(expCtx, scrapeTimeout)
			samples := collect(emitCtx)
			emitCancel()
			if expCtx.Err() != nil {
				return
			}
			if err := SendDogStatsD(cfg.DogStatsD, samples); err != nil {
				logger.Warn("metrics dogstatsd emit failed", "err", err)
			}
		}
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			emit()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-expCtx.Done():
					return
				case <-ticker.C:
					if expCtx.Err() != nil {
						return
					}
					emit()
				}
			}
		}()
	}

	return e, nil
}

// Stop gracefully shuts down the metrics exporter and waits for its goroutines.
func (e *Exporter) Stop() {
	if e == nil {
		return
	}
	e.stopOnce.Do(func() {
		if e.cancel != nil {
			e.cancel()
		}
		e.wg.Wait()
	})
}
