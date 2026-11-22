package daemon

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cron"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/metrics"
	"github.com/hev/kit/internal/secrets"
)

func TestDaemonMetricsIntegrationAndConfigGuard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("HEV_DISPATCH_ROOT", filepath.Join(home, "dispatch"))
	t.Setenv("HEV_SECRETS_POLICY", filepath.Join(home, ".hev", "secrets", "secrets-policy.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(home, ".hev", "secrets", "op-reads.log"))

	// Invalid [metrics] config is refused cleanly by daemon.Run before starting capture.
	badCfgPath := filepath.Join(home, ".hev", "config.toml")
	if err := os.MkdirAll(filepath.Dir(badCfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badCfgPath, []byte("[metrics]\nenabled = true\nprometheus_addr = \"0.0.0.0:9108\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", badCfgPath)
	if err := Run(context.Background()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "wildcard") {
		t.Fatalf("daemon.Run with wildcard prometheus_addr: err=%v, want wildcard refusal", err)
	}

	// Set up real local orchestrator and failing cron job state under isolated temp HOME.
	orchCfgDir := dispatch.OrchestratorConfigDir()
	if err := os.MkdirAll(orchCfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orchCfgDir, "alpha.md"), []byte("# alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	alphaDir := dispatch.OrchestratorDir("alpha")
	if err := os.MkdirAll(alphaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alphaDir, "state.json"), []byte(`{"name":"alpha","state":"running"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orchCfgDir, "broken-orch.md"), []byte("# broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cronStateDir := filepath.Join(home, ".hev", "cron", "nightly")
	if err := os.MkdirAll(cronStateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cronStateDir, "state.json"), []byte(`{"consecutive_failures":2}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cronCfgPath := filepath.Join(home, ".hev", "cron-config.toml")
	if err := os.WriteFile(cronCfgPath, []byte("[[cron]]\nname = \"nightly\"\nevery = \"1m\"\ncmd = [\"true\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	jobs, err := cron.Load(cronCfgPath)
	if err != nil {
		t.Fatalf("cron.Load: %v", err)
	}
	membership := cron.Membership{Self: "testbox", Enabled: false}
	cronRunner := cron.Runner{Home: home, Membership: membership}
	cache := secrets.New()

	// Start metrics exporter via metrics.Start using daemon's wiring helper.
	cfg := metrics.Config{
		Enabled:        true,
		Interval:       "30s",
		PrometheusAddr: "127.0.0.1:0",
	}
	src := metrics.Sources{
		Self:           membership.Self,
		ClusterEnabled: membership.Enabled,
		Orchestrators:  dispatch.ListOrchestrators,
		SecretsStatus: func() *secrets.Status {
			st := cache.LocalStatus()
			return &st
		},
		CronFailing: func(ctx context.Context) (int, error) {
			failing := 0
			for _, j := range jobs {
				d, err := cronRunner.Inspect(ctx, j, false)
				if err == nil && d.Status == "failing" {
					failing++
				}
			}
			return failing, nil
		},
	}
	exp, err := metrics.Start(context.Background(), cfg, src, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("metrics.Start: %v", err)
	}
	defer exp.Stop()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + exp.BoundPrometheusAddr() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	text := string(body)

	for _, want := range []string{
		`hev_secrets_op_used_hourly{member="testbox"} 0`,
		`hev_secrets_op_used_daily{member="testbox"} 0`,
		`hev_orchestrator_by_state{member="testbox",state="running"} 1`,
		`hev_orchestrator_by_state{member="testbox",state="broken"} 1`,
		`hev_orchestrator_by_state{member="testbox",state="waiting"} 0`,
		`hev_cron_failing{member="testbox"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in /metrics:\n%s", want, text)
		}
	}
	if strings.Contains(text, home) || strings.Contains(text, "alpha") || strings.Contains(text, "nightly") {
		t.Fatalf("emitted metrics leaked local path or item name:\n%s", text)
	}

	// Verify daemon.startMetrics handles nil/disabled config as clean no-op.
	startMetrics(context.Background(), nil, cache, nil, membership, cronRunner, jobs, slog.New(slog.NewTextHandler(io.Discard, nil)))()
	startMetrics(context.Background(), &metrics.Config{Enabled: false}, cache, nil, membership, cronRunner, jobs, slog.New(slog.NewTextHandler(io.Discard, nil)))()
}
