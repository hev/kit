package hooks

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/secrets"
	"github.com/oklog/ulid/v2"
)

type Activity struct {
	Schema string    `json:"schema"`
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	Host   string    `json:"host"`
	Cwd    string    `json:"cwd"`
}

type PeerMessage struct {
	Activity
	From      string  `json:"from"`
	To        string  `json:"to"`
	Chat      bool    `json:"chat"`
	Kind      string  `json:"kind"`
	Text      *string `json:"text,omitempty"`
	TextBytes int     `json:"text_bytes"`
	ReplyTo   string  `json:"reply_to,omitempty"`
}

type BoardPost struct {
	Activity
	Board     string   `json:"board"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	Body      *string  `json:"body,omitempty"`
	BodyBytes int      `json:"body_bytes"`
	URL       string   `json:"url"`
	ThreadID  int64    `json:"thread_id,omitempty"`
	Author    string   `json:"author"`
}

type withoutPeerMessageKey struct{}

// WithoutPeerMessage suppresses the per-recipient events of a chat fan-out.
func WithoutPeerMessage(ctx context.Context) context.Context {
	return context.WithValue(ctx, withoutPeerMessageKey{}, true)
}

func ObservePeerMessage(ctx context.Context, ev PeerMessage, text string) {
	if ctx.Value(withoutPeerMessageKey{}) != nil {
		return
	}
	ev.Activity.complete("hev.peer.message/v1", "peer-message")
	ev.TextBytes = len(text)
	observeActivity(ctx, ev.Event, text, func(_ context.Context, safe *string) any {
		ev.Text = safe
		return ev
	})
}

// ObserveBoardPost optionally resolves metadata absent from the reply response.
// The lookup and handlers share a single hook timeout.
func ObserveBoardPost(ctx context.Context, ev BoardPost, body string, thread func(context.Context) (string, string, []string)) {
	ev.Activity.complete("hev.board.post/v1", "board-post")
	ev.BodyBytes = len(body)
	observeActivity(ctx, ev.Event, body, func(ctx context.Context, safe *string) any {
		if thread != nil {
			// Reserve most of the budget for the handlers if the board is slow.
			if deadline, ok := ctx.Deadline(); ok {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Until(deadline)/4)
				defer cancel()
			}
			ev.Board, ev.Title, ev.Tags = thread(ctx)
			thread = nil
		}
		if ev.Tags == nil {
			ev.Tags = []string{}
		}
		ev.Body = safe
		return ev
	})
}

func (a *Activity) complete(schema, event string) {
	a.Schema, a.Event = schema, event
	if a.ID == "" {
		a.ID = strings.ToLower(ulid.Make().String())
	}
	if a.Time.IsZero() {
		a.Time = time.Now().UTC()
	}
	if a.Host == "" {
		a.Host = hostlabel.Self(os.Hostname)
	}
	if a.Cwd == "" {
		a.Cwd, _ = os.Getwd()
	}
}

func observeActivity(ctx context.Context, event, text string, payload func(context.Context, *string) any) {
	if Disabled(os.Getenv("HEV_HOOKS")) {
		return
	}
	path := secrets.ConfigPath()
	cfg, warning := LoadConfig(path)
	if !cfg.Enabled {
		return
	}
	log := DefaultLog()
	if warning != "" {
		lifecycleWarn(log, event, warning)
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	metadata := payload(ctx, nil)
	if err := log.Append("events.jsonl", metadata); err != nil {
		lifecycleWarn(log, event, err.Error())
	}
	body := metadata
	if warning == "" && allowsActivityBody(path) {
		// Hook redaction is mandatory and needs no persistent capture identity.
		var salt [32]byte
		_, err := rand.Read(salt[:])
		if err == nil {
			var scrubber *redact.Scrubber
			scrubber, err = redact.New(salt[:])
			if err == nil {
				safe, _ := scrubber.Text(text)
				body = payload(ctx, &safe)
			}
		}
		if err != nil {
			lifecycleWarn(log, event, "could not redact activity content; omitting it")
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		lifecycleWarn(log, event, err.Error())
		return
	}
	RunLifecycle(ctx, event, raw, os.Getenv("PATH"), cfg, log)
}

// A malformed or unreadable host policy cannot authorize exporting content.
func allowsActivityBody(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	var config struct {
		Notify struct {
			Client bool `toml:"client"`
		} `toml:"notify"`
	}
	_, err = toml.DecodeFile(path, &config)
	return err == nil && !config.Notify.Client
}
