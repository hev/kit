package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// A child run of this test binary is one kit process that needs the Layer key.
func TestMain(m *testing.M) {
	if os.Getenv("KIT_TEST_LAYER_KEY_CHILD") == "1" {
		key, err := LayerKey()
		if err != nil {
			os.Stderr.WriteString("layer key failed\n")
			os.Exit(1)
		}
		if key == fakeop.Value {
			os.Stdout.WriteString("key-ok\n")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func layerKeyEnv(t *testing.T) (fakeop.Fake, string) {
	t.Helper()
	f := fakeop.Install(t)
	home, err := os.MkdirTemp("/tmp", "kitlk") // short enough for a Unix socket path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(home, "op-reads.log"))
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(home, "no-token-file"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	for _, k := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY", "TPUF_API_KEY", "LAYER_API_KEY_OP_REF"} {
		t.Setenv(k, "")
	}
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://example/layer/credential")
	return f, home
}

func runLayerKeyProcess(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "KIT_TEST_LAYER_KEY_CHILD=1")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "key-ok" {
		t.Fatalf("kit process did not get the Layer key: %v", err)
	}
}

func TestLayerKeyTwoProcessesOneRequestWithDaemon(t *testing.T) {
	f, home := layerKeyEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	runLayerKeyProcess(t)
	runLayerKeyProcess(t)
	if calls := f.CallLines(t); len(calls) != 1 || calls[0] != "inject" {
		t.Fatalf("two kit processes made %d op requests (%q), want one inject", len(calls), calls)
	}
	log, err := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "\n") != 1 || !strings.Contains(string(log), "kit_layer-key") || strings.Contains(string(log), fakeop.Value) || strings.Contains(string(log), "op://") {
		t.Fatalf("read log should hold one labelled line without values")
	}
}

func TestLayerKeyWithoutDaemonReadsDirectlyAsBefore(t *testing.T) {
	f, home := layerKeyEnv(t)
	runLayerKeyProcess(t)
	runLayerKeyProcess(t)
	calls := f.CallLines(t)
	if len(calls) != 2 {
		t.Fatalf("got %d op requests without a daemon, want one per process", len(calls))
	}
	for _, c := range calls {
		if c != "read op://example/layer/credential" {
			t.Fatalf("direct path changed: op %q", c)
		}
	}
	log, _ := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if strings.Count(string(log), "kind=read") != 2 || strings.Contains(string(log), fakeop.Value) {
		t.Fatal("direct reads should be logged once each, without values")
	}
}
