package dispatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

var expediteBoardPost = func(ctx context.Context, exe string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, exe, args...).CombinedOutput()
}

// Expediter handoffs run on the durable owner, including lifetime expiry and
// explicit stop between rounds. Board failure remains visible and never restarts
// a stopped order or silently drops the handoff.
func expediteHandoff(dir string, m Meta, reason string) error {
	if m.Until != "all followed runs merged or abandoned" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "handoff-posted")); err == nil {
		return nil
	}
	var cfg struct {
		Board struct {
			URL string `toml:"url"`
		} `toml:"board"`
	}
	configPath := os.Getenv("HEV_CONFIG")
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".hev", "config.toml")
	}
	_, err := toml.DecodeFile(configPath, &cfg)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if cfg.Board.URL == "" && os.Getenv("FACTORY_BOARD_URL") == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "order.md"))
	if err != nil {
		return err
	}
	notes, _ := os.ReadFile(filepath.Join(dir, "round-notes.md"))
	body := fmt.Sprintf("Expediter %s/%s stopped: %s. Reply: %s.\nSnapshot and authority:\n%s\nLatest evidence:\n%s", m.Host, m.Name, reason, m.Reply, b, notes)
	handoff := filepath.Join(dir, "handoff.md")
	if err = os.WriteFile(handoff, []byte(body), 0600); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := expediteBoardPost(ctx, exe, "board", "post", "--board", "floor", "--title", "Expediter "+m.Name+" stopped: "+reason, "--tags", "expedite,handoff", "-f", handoff)
	if err != nil {
		_ = os.WriteFile(filepath.Join(dir, "handoff-error"), out, 0600)
		return fmt.Errorf("board handoff: %w: %s", err, out)
	}
	_ = os.Remove(filepath.Join(dir, "handoff-error"))
	return os.WriteFile(filepath.Join(dir, "handoff-posted"), out, 0600)
}
