package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layerenv"
)

func kitDaemonHome(t *testing.T, config string) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKitConfigRestoresPriorTarget(t *testing.T) {
	path := kitDaemonHome(t, "[layer]\nendpoint='http://localhost:8080'\napi_key='local'\nnamespace='local'\n")
	doc, err := ReadInviteConfig()
	if err != nil {
		t.Fatal(err)
	}
	kit := KitConfig{Service: "http://127.0.0.1:8787", API: "http://127.0.0.1:8788", Key: "hk_x", KeyID: "k", TeamID: "t", TeamSlug: "acme", AuthorID: "u", AuthorName: "Ada"}
	if err := WriteKitConfig(doc, kit); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatal("credential file must be private")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Kit.SignedIn() || cfg.Kit.TeamSlug != "acme" || cfg.LayerAPIKey != "" || !kitTarget(t, cfg) || !cfg.CaptureRedact {
		t.Fatalf("sign-in not loaded: %+v", cfg.Kit)
	}
	doc, _ = ReadInviteConfig()
	if err := ClearKitConfig(doc); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kit.SignedIn() || cfg.Kit.Service != kit.Service || cfg.LayerAPIKey != "local" || kitTarget(t, cfg) {
		t.Fatalf("prior target not restored: %+v", cfg)
	}
}

func kitTarget(t *testing.T, cfg *Config) bool {
	t.Helper()
	target, err := OpenTargetRead(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	return target.Kit != nil
}

// With [kit] and an unreachable kit API, the daemon must keep running,
// capture into its outbox, and say so instead of failing.
func TestDaemonFailsOpenWhenKitAPIDown(t *testing.T) {
	kitDaemonHome(t, "[kit]\nservice='http://127.0.0.1:1'\napi='http://127.0.0.1:1'\nkey='hk_x'\nkey_id='k'\nteam_slug='acme'\n[capture]\nscan_interval='1h'\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // one cycle, then return
	if err := Run(ctx); err != nil {
		t.Fatalf("signed-in daemon failed closed: %v", err)
	}
	s, err := ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if s.LastError != "" || s.LastRun.IsZero() || !strings.Contains(s.Notice, "kit API") {
		t.Fatalf("status %+v", s)
	}
	if strings.Contains(s.Notice, "not wired yet") {
		t.Fatal("notice still says hosted capture is unwired")
	}
}
