// Package feed is kit's Slack activity feed (docs/feed.md): what agents on
// this host are doing, posted to one channel as one thread per session, run
// or orchestrator, with turns summarized by a small model.
//
// The work is split so a hook stays far inside kit's 2s handler budget. The
// hook side (this package) only decides whether an event is wanted, writes a
// spool record and wakes the sender. The sender (`hev feed send`, package
// feedsend) summarizes, formats and posts, one record at a time, and exits
// when the spool is idle.
package feed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Event kinds the feed understands. Harness events come from `hev hook`,
// notices from attention, board posts from the board CLI, and evidence from
// an acceptance-evidence recording.
const (
	KindSessionStart = "session-start"
	KindPromptSubmit = "prompt-submit"
	KindPreTool      = "pre-tool"
	KindPostTool     = "post-tool"
	KindStop         = "stop"
	KindPreCompact   = "pre-compact"
	KindNeedsYou     = "needs-you"
	KindReady        = "ready"
	KindDone         = "done"
	KindPeerMessage  = "peer-message"
	KindBoardPost    = "board-post"
	KindBoardReply   = "board-reply" // normalized hook delivered on the board-post feed
	KindEvidence     = "evidence"
)

// AllKinds lists every kind, in the order docs/feed.md describes them.
var AllKinds = []string{KindSessionStart, KindPromptSubmit, KindPreTool, KindPostTool, KindStop, KindPreCompact,
	KindNeedsYou, KindReady, KindDone, KindPeerMessage, KindBoardPost, KindEvidence}

// DefaultEvents is what a [feed] table without `events` posts: a summary at
// session start and per turn, every attention notice, board feedback and
// acceptance evidence. Prompts are summarized with the turn they start, and
// tool calls are too loud for a channel.
var DefaultEvents = []string{KindSessionStart, KindStop, KindNeedsYou, KindReady, KindDone, KindBoardPost, KindEvidence}

// Defaults.
const (
	DefaultModel        = "claude-haiku-5-5"
	DefaultMaxUploadMB  = 50
	DefaultClientRoot   = "~/workspace/clients"
	DefaultFeedbackName = "feedback"
)

// Config is the [feed] table in kit's config.toml.
type Config struct {
	// Enabled turns the feed on. A [feed] table defaults to on; no table is
	// off.
	Enabled bool
	// Channel is a channel ID (C...) or #name. The bot must be a member.
	Channel string
	// Token is the bot token's secret reference (secret://, op://, azkv://).
	Token string
	// Model and ModelKey name the summarizing model and its API key's
	// secret reference. Without ModelKey every post is the compact
	// fallback.
	Model    string
	ModelKey string
	// Events are the kinds posted; Skip removes kinds from them.
	Events []string
	Skip   []string
	// Boards are the hev board boards whose threads and replies are posted.
	Boards []string
	// ClientRoots are directories whose sessions are client work: nothing
	// but a bare active/ended line leaves the host.
	ClientRoots []string
	// Client is [notify] client: every session on this host is client work.
	Client bool
	// MaxUploadBytes caps each attached file; larger files are linked.
	MaxUploadBytes int64
	// Tag prefixes every top-level post (for example "[test]").
	Tag string
	// Path is the file read.
	Path string
}

// Wants reports whether kind is posted.
func (c Config) Wants(kind string) bool {
	if !c.Enabled {
		return false
	}
	for _, s := range c.Skip {
		if s == kind {
			return false
		}
	}
	for _, k := range c.Events {
		if k == kind {
			return true
		}
	}
	return false
}

// WantsBoard reports whether posts on board are fed.
func (c Config) WantsBoard(board string) bool {
	for _, b := range c.Boards {
		if strings.EqualFold(b, board) {
			return true
		}
	}
	return false
}

// IsClientPath reports whether dir lies under a client root.
func (c Config) IsClientPath(dir string) bool {
	if dir == "" {
		return false
	}
	dir = filepath.Clean(expand(dir))
	for _, r := range c.ClientRoots {
		r = filepath.Clean(expand(r))
		if r == "." || r == "" {
			continue
		}
		if dir == r || strings.HasPrefix(dir, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type fileConfig struct {
	Feed *struct {
		Enabled     *bool     `toml:"enabled"`
		Channel     string    `toml:"channel"`
		Token       string    `toml:"token"`
		Model       string    `toml:"model"`
		ModelKey    string    `toml:"model_key"`
		Events      []string  `toml:"events"`
		Skip        []string  `toml:"skip"`
		Boards      *[]string `toml:"boards"`
		ClientRoots *[]string `toml:"client_roots"`
		MaxUploadMB *int64    `toml:"max_upload_mb"`
		Tag         string    `toml:"tag"`
	} `toml:"feed"`
	Notify struct {
		Client bool `toml:"client"`
	} `toml:"notify"`
}

// Load reads [feed] from path. A missing file or table is a disabled feed.
// An invalid table is an error and a disabled feed.
func Load(path string) (Config, error) {
	c := Config{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if !info.Mode().IsRegular() {
		return c, fmt.Errorf("config %s is not a regular file", path)
	}
	var f fileConfig
	if _, err := toml.DecodeFile(path, &f); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	c.Client = f.Notify.Client
	if f.Feed == nil {
		return c, nil
	}
	t := f.Feed
	c.Enabled = t.Enabled == nil || *t.Enabled
	c.Channel, c.Token = strings.TrimSpace(t.Channel), strings.TrimSpace(t.Token)
	c.Model, c.ModelKey = strings.TrimSpace(t.Model), strings.TrimSpace(t.ModelKey)
	c.Events, c.Skip, c.Tag = t.Events, t.Skip, strings.TrimSpace(t.Tag)
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if len(c.Events) == 0 {
		c.Events = DefaultEvents
	}
	c.Boards = []string{DefaultFeedbackName}
	if t.Boards != nil {
		c.Boards = *t.Boards
	}
	c.ClientRoots = []string{DefaultClientRoot}
	if t.ClientRoots != nil {
		c.ClientRoots = *t.ClientRoots
	}
	c.MaxUploadBytes = DefaultMaxUploadMB << 20
	if t.MaxUploadMB != nil {
		c.MaxUploadBytes = *t.MaxUploadMB << 20
	}
	if err := c.validate(); err != nil {
		c.Enabled = false
		return c, fmt.Errorf("config %s [feed]: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Channel == "" {
		return errors.New("channel is required (a channel ID such as C0123ABCD, or #name)")
	}
	if c.Token == "" || !isSecretRef(c.Token) {
		// Never echo the value: it may be a pasted token.
		return errors.New("token must be a secret reference (secret://NAME, op://... or azkv://...)")
	}
	if c.ModelKey != "" && !isSecretRef(c.ModelKey) {
		return errors.New("model_key must be a secret reference (secret://NAME, op://... or azkv://...)")
	}
	for _, list := range [][]string{c.Events, c.Skip} {
		for _, k := range list {
			if !validKind(k) {
				return fmt.Errorf("unknown event %q (want one of %s)", k, strings.Join(AllKinds, ", "))
			}
		}
	}
	if c.MaxUploadBytes < 0 {
		return errors.New("max_upload_mb must not be negative")
	}
	return nil
}

func validKind(k string) bool {
	for _, v := range AllKinds {
		if v == k {
			return true
		}
	}
	return false
}

func isSecretRef(s string) bool {
	for _, p := range []string{"secret://", "op://", "azkv://"} {
		if strings.HasPrefix(s, p) && len(s) > len(p) {
			return true
		}
	}
	return false
}

func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
