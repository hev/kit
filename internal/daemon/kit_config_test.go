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
	if !cfg.Kit.SignedIn() || cfg.Kit.TeamSlug != "acme" || cfg.LayerAPIKey != "" || !kitOnly(cfg) || !cfg.CaptureRedact {
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
	if cfg.Kit.SignedIn() || cfg.Kit.Service != kit.Service || cfg.LayerAPIKey != "local" || kitOnly(cfg) {
		t.Fatalf("prior target not restored: %+v", cfg)
	}
}

// With [kit] and no [layer], the daemon must keep running and say why it is
// not uploading, instead of exiting on a missing archive target.
func TestDaemonFailsOpenWithKitOnly(t *testing.T) {
	kitDaemonHome(t, "[kit]\nservice='http://127.0.0.1:1'\napi='http://127.0.0.1:1'\nkey='hk_x'\nkey_id='k'\nteam_slug='acme'\n[capture]\nscan_interval='1h'\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // one cycle, then return
	if err := Run(ctx); err != nil {
		t.Fatalf("kit-only daemon failed closed: %v", err)
	}
	s, err := ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if s.Notice != KitCaptureNotice || s.LastError != "" || s.LastRun.IsZero() {
		t.Fatalf("status %+v", s)
	}
	if !strings.Contains(s.Notice, "not wired yet") {
		t.Fatal("notice does not say hosted capture is unwired")
	}
}
