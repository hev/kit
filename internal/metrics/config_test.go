package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAbsentConfigReturnsNil(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(filepath.Join(dir, "missing.toml"))
	if err != nil || cfg != nil {
		t.Fatalf("missing file: cfg=%+v err=%v, want nil, nil", cfg, err)
	}

	emptyPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(emptyPath, []byte("[capture]\nscan_interval = \"5m\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(emptyPath)
	if err != nil || cfg != nil {
		t.Fatalf("absent [metrics] table: cfg=%+v err=%v, want nil, nil", cfg, err)
	}
}

func TestLoadValidConfigs(t *testing.T) {
	cases := []struct {
		name         string
		toml         string
		wantEnabled  bool
		wantInterval time.Duration
	}{
		{
			name:         "opt-in default disabled",
			toml:         "[metrics]\n",
			wantEnabled:  false,
			wantInterval: 60 * time.Second,
		},
		{
			name: "ipv4 loopback dogstatsd and prometheus",
			toml: `[metrics]
enabled = true
interval = "10s"
dogstatsd = "127.0.0.1:8125"
prometheus_addr = "127.0.0.1:9108"
placement = ["oven", "mini"]
`,
			wantEnabled:  true,
			wantInterval: 10 * time.Second,
		},
		{
			name: "ipv6 loopback and max interval",
			toml: `[metrics]
enabled = true
interval = "1h"
dogstatsd = "[::1]:8125"
prometheus_addr = "[::1]:9108"
`,
			wantEnabled:  true,
			wantInterval: time.Hour,
		},
		{
			name: "tailscale CGNAT boundaries and ephemeral prometheus port",
			toml: `[metrics]
enabled = true
dogstatsd = "100.64.0.1:8125"
prometheus_addr = "100.127.255.254:0"
placement = ["node.a-1_2"]
`,
			wantEnabled:  true,
			wantInterval: 60 * time.Second,
		},
		{
			name: "loopback 127/8 subnet",
			toml: `[metrics]
enabled = true
dogstatsd = "127.0.1.1:8125"
prometheus_addr = "127.1.2.3:9108"
`,
			wantEnabled:  true,
			wantInterval: 60 * time.Second,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("Load() returned nil config")
			}
			if cfg.Enabled != tc.wantEnabled {
				t.Fatalf("Enabled=%v, want %v", cfg.Enabled, tc.wantEnabled)
			}
			if got := cfg.EmitInterval(); got != tc.wantInterval {
				t.Fatalf("EmitInterval()=%v, want %v", got, tc.wantInterval)
			}
		})
	}
}

func TestConfigRefusesInvalidIntervalsAndAddresses(t *testing.T) {
	refused := []struct {
		name string
		toml string
		want string
	}{
		// Interval bounds
		{name: "interval below 10s", toml: "[metrics]\nenabled = true\ninterval = \"9s\"\n", want: "10s"},
		{name: "interval 9999ms", toml: "[metrics]\nenabled = true\ninterval = \"9999ms\"\n", want: "10s"},
		{name: "interval zero", toml: "[metrics]\nenabled = true\ninterval = \"0s\"\n", want: "10s"},
		{name: "interval negative", toml: "[metrics]\nenabled = true\ninterval = \"-30s\"\n", want: "10s"},
		{name: "interval above 1h", toml: "[metrics]\nenabled = true\ninterval = \"1h1s\"\n", want: "1h"},
		{name: "interval invalid syntax", toml: "[metrics]\nenabled = true\ninterval = \"soon\"\n", want: "duration"},

		// DogStatsD private bind guard
		{name: "dogstatsd public ipv4", toml: "[metrics]\ndogstatsd = \"8.8.8.8:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd public ipv6", toml: "[metrics]\ndogstatsd = \"[2001:4860:4860::8888]:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd wildcard ipv4", toml: "[metrics]\ndogstatsd = \"0.0.0.0:8125\"\n", want: "wildcard"},
		{name: "dogstatsd wildcard ipv6", toml: "[metrics]\ndogstatsd = \"[::]:8125\"\n", want: "wildcard"},
		{name: "dogstatsd empty host", toml: "[metrics]\ndogstatsd = \":8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd rfc1918 192.168", toml: "[metrics]\ndogstatsd = \"192.168.1.10:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd rfc1918 10/8", toml: "[metrics]\ndogstatsd = \"10.0.0.5:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd rfc1918 172.16", toml: "[metrics]\ndogstatsd = \"172.16.0.1:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd below cgnat", toml: "[metrics]\ndogstatsd = \"100.63.255.255:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd above cgnat", toml: "[metrics]\ndogstatsd = \"100.128.0.0:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd ipv4-mapped public", toml: "[metrics]\ndogstatsd = \"[::ffff:8.8.8.8]:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd ipv4-mapped wildcard", toml: "[metrics]\ndogstatsd = \"[::ffff:0.0.0.0]:8125\"\n", want: "wildcard"},
		{name: "dogstatsd ipv6 zone", toml: "[metrics]\ndogstatsd = \"[::1%lo0]:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd hostname", toml: "[metrics]\ndogstatsd = \"localhost:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd fqdn", toml: "[metrics]\ndogstatsd = \"metrics.example.com:8125\"\n", want: "dogstatsd"},
		{name: "dogstatsd port zero", toml: "[metrics]\ndogstatsd = \"127.0.0.1:0\"\n", want: "port"},
		{name: "dogstatsd missing port", toml: "[metrics]\ndogstatsd = \"127.0.0.1\"\n", want: "dogstatsd"},

		// Prometheus private bind guard
		{name: "prometheus wildcard ipv4", toml: "[metrics]\nprometheus_addr = \"0.0.0.0:9108\"\n", want: "wildcard"},
		{name: "prometheus wildcard ipv6", toml: "[metrics]\nprometheus_addr = \"[::]:9108\"\n", want: "wildcard"},
		{name: "prometheus empty host", toml: "[metrics]\nprometheus_addr = \":9108\"\n", want: "prometheus_addr"},
		{name: "prometheus public ipv4", toml: "[metrics]\nprometheus_addr = \"1.1.1.1:9108\"\n", want: "prometheus_addr"},
		{name: "prometheus rfc1918", toml: "[metrics]\nprometheus_addr = \"192.168.0.5:9108\"\n", want: "prometheus_addr"},
		{name: "prometheus below cgnat", toml: "[metrics]\nprometheus_addr = \"100.63.255.255:9108\"\n", want: "prometheus_addr"},
		{name: "prometheus above cgnat", toml: "[metrics]\nprometheus_addr = \"100.128.0.0:9108\"\n", want: "prometheus_addr"},
		{name: "prometheus ipv4-mapped wildcard", toml: "[metrics]\nprometheus_addr = \"[::ffff:0.0.0.0]:9108\"\n", want: "wildcard"},
		{name: "prometheus hostname", toml: "[metrics]\nprometheus_addr = \"localhost:9108\"\n", want: "prometheus_addr"},
		{name: "prometheus bad port", toml: "[metrics]\nprometheus_addr = \"127.0.0.1:70000\"\n", want: "port"},

		// Placement & unknown fields
		{name: "placement duplicate", toml: "[metrics]\nplacement = [\"oven\", \"oven\"]\n", want: "placement"},
		{name: "placement empty entry", toml: "[metrics]\nplacement = [\"\"]\n", want: "placement"},
		{name: "placement path injection", toml: "[metrics]\nplacement = [\"/etc/passwd\"]\n", want: "placement"},
		{name: "placement tag delimiter", toml: "[metrics]\nplacement = [\"oven,source:evil\"]\n", want: "placement"},
		{name: "unknown field", toml: "[metrics]\nenabled = true\nsecret_token = \"nope\"\n", want: "unknown field"},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load() succeeded for invalid config %q, want error", tc.name)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}

	t.Run("placement over 64 entries", func(t *testing.T) {
		var names []string
		for i := 0; i < 65; i++ {
			names = append(names, fmt.Sprintf("node-%d", i))
		}
		cfg := Config{Placement: names}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for >64 placement entries")
		}
	})
}

func TestLoadErrorsRedactSecretsAndLocalPaths(t *testing.T) {
	secretVal := "sk_live_SUPER_SECRET_DO_NOT_LEAK_987654"
	dir := filepath.Join(t.TempDir(), "private-user-home-marker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	raw := fmt.Sprintf("[metrics]\nenabled = \"not-a-bool-%s\"\n", secretVal)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML type")
	}
	msg := err.Error()
	if strings.Contains(msg, secretVal) {
		t.Fatalf("error leaked secret value: %q", msg)
	}
	if strings.Contains(msg, "private-user-home-marker") || strings.Contains(msg, path) {
		t.Fatalf("error leaked local path: %q", msg)
	}
}
