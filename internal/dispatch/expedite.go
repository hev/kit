package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

var expediteBoardPost = func(ctx context.Context, exe string, args ...string) ([]byte, error) {
	return helperCommand(ctx, exe, args...).CombinedOutput()
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
		// Match the board CLI's retained endpoint lookup, including hosts migrated
		// before board.url was added to the HEV configuration.
		home, _ := os.UserHomeDir()
		var legacy struct {
			URL string `json:"board_url"`
		}
		if b, e := os.ReadFile(filepath.Join(home, ".local/state/factory-feedback/config.json")); e == nil {
			_ = json.Unmarshal(b, &legacy)
		}
		u, e := url.Parse(legacy.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return nil
		}
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
	// The board indexes body as a filterable attribute with a 4096-byte limit.
	// Keep the complete handoff locally and post a bounded summary with pointers.
	snapshot := string(b)
	if _, tail, ok := strings.Cut(snapshot, "<!-- hev-expedite-plan\n"); ok {
		snapshot, _, _ = strings.Cut(tail, "\n-->")
	}
	summary := fmt.Sprintf("Expediter %s/%s stopped: %s. Reply: %s.\n%s\nEvidence: %s (full snapshot and notes), %s.\nSnapshot:\n%s", m.Host, m.Name, reason, m.Reply, firstNeedsYou(string(notes)), handoff, filepath.Join(dir, "checkpoint.md"), snapshot)
	if len(summary) > 3500 {
		summary = summary[:3500]
		for !utf8.ValidString(summary) {
			summary = summary[:len(summary)-1]
		}
		summary += "\n[Full handoff retained at evidence path above.]"
	}
	post := filepath.Join(dir, "handoff-summary.md")
	if err = os.WriteFile(post, []byte(summary), 0600); err != nil {
		return err
	}
	out, err := expediteBoardPost(ctx, exe, "board", "post", "--board", "floor", "--title", "Expediter "+m.Name+" stopped: "+reason, "--tags", "expedite,handoff", "-f", post)
	if err != nil {
		_ = os.WriteFile(filepath.Join(dir, "handoff-error"), out, 0600)
		return fmt.Errorf("board handoff: %w: %s", err, out)
	}
	_ = os.Remove(filepath.Join(dir, "handoff-error"))
	return os.WriteFile(filepath.Join(dir, "handoff-posted"), out, 0600)
}
