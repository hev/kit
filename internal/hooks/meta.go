package hooks

import (
	"context"
	"crypto/rand"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/secrets"
	"github.com/oklog/ulid/v2"
)

// MetaEvents use the normalized kit envelope and never accept message bodies.
var MetaEvents = []string{
	"order-queued", "order-started", "order-rework",
	"steer-sent", "steer-acked", "steer-escalated",
	"admission-check", "admission-denied",
	"cluster-join", "cluster-invite", "cluster-remove",
	"peer-online", "peer-offline",
	"board-reply", "board-pin",
}

// Meta raises a kit event on the node that owns the action. Supply metadata
// only: prompt, message and post bodies, credentials and paths stay at source.
// Only admission-check uses handler responses.
func Meta(ctx context.Context, event string, data map[string]any, reason string) Response {
	return meta(ctx, event, data, reason, nil)
}

// activityBody is optional, already scrubbed, and never written to the log.
func meta(ctx context.Context, event string, data map[string]any, reason string, activityBody *string) Response {
	if ctx.Err() != nil || Disabled(os.Getenv("HEV_HOOKS")) {
		return Response{Decision: Continue}
	}
	cfg, warning := LoadConfig(secrets.ConfigPath())
	if !cfg.Enabled {
		return Response{Decision: Continue}
	}
	return deliverMeta(ctx, cfg, warning, newMetaEvent(event, data, reason), activityBody, false, UserHandlers(os.Getenv("PATH")))
}

// RecordMetaBatch records a set of observer events before any delivery starts.
// Its returned function delivers the batch under the caller's shared deadline.
func RecordMetaBatch(event string, data []map[string]any) func(context.Context) {
	if Disabled(os.Getenv("HEV_HOOKS")) {
		return nil
	}
	cfg, warning := LoadConfig(secrets.ConfigPath())
	if !cfg.Enabled {
		return nil
	}
	log := DefaultLog()
	events := make([]Event, 0, len(data))
	for _, fields := range data {
		ev := newMetaEvent(event, fields, "")
		if err := log.Event(ev, Response{Decision: Continue}, 0); err != nil {
			_ = log.Warn(ev, "could not record hook event")
		}
		events = append(events, ev)
	}
	return func(ctx context.Context) {
		handlers := UserHandlers(os.Getenv("PATH"))
		var deliveries sync.WaitGroup
		for _, ev := range events {
			deliveries.Go(func() { deliverMeta(ctx, cfg, warning, ev, nil, true, handlers) })
		}
		deliveries.Wait()
	}
}

func newMetaEvent(event string, data map[string]any, reason string) Event {
	start := time.Now().UTC()
	ev := Event{
		Schema: Schema, ID: strings.ToLower(ulid.Make().String()), Time: start,
		Event: event, Harness: "kit", Host: hostlabel.Self(os.Hostname),
		Data: make(map[string]any, len(data)), Reason: scrubMeta(reason),
	}
	for key, value := range data {
		switch value := value.(type) {
		case string:
			if event == feed.KindBoardReply && key == "board_origin" {
				ev.Data[key] = feed.BoardOrigin(value)
			} else {
				ev.Data[key] = scrubMeta(value)
			}
		case bool, int, int64:
			ev.Data[key] = value
		}
	}
	return ev
}

func deliverMeta(ctx context.Context, cfg Config, warning string, ev Event, activityBody *string, recorded bool, handlers []Handler) Response {
	log := DefaultLog()
	warn := func(message string) {
		// Handler stderr, parser errors and config diagnostics can include
		// secrets and paths. Record the failure without those details.
		safe := "hook handler or configuration failed; continuing"
		switch {
		case strings.Contains(message, "did not answer"), strings.Contains(message, "timed out"):
			safe = "hook handler timed out; continuing"
		case strings.Contains(message, "failed on kit/"):
			safe = "hook handler failed; continuing"
		}
		_ = log.Warn(ev, safe)
	}
	if warning != "" {
		warn(warning)
	}
	delivery := ev
	if activityBody != nil {
		delivery.Data = maps.Clone(ev.Data)
		delivery.Data["body"] = *activityBody
	}
	r := Run(ctx, delivery, handlers, cfg.Timeout, warn)
	if ev.Event != "admission-check" {
		r = Response{Decision: Continue}
	}
	if !recorded {
		if err := log.Event(ev, r, time.Since(ev.Time)); err != nil {
			warn(err.Error())
		}
	}
	r.Reason = scrubMeta(r.Reason)
	return r
}

// scrubMeta reuses kit's credential scrubber and drops path-like metadata.
func scrubMeta(value string) string {
	if value == "" {
		return ""
	}
	for _, word := range strings.Fields(value) {
		_, value, _ := strings.Cut(word, "=")
		if pathLike(word) || pathLike(value) {
			return "[redacted]"
		}
	}
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "[redacted]"
	}
	scrubber, err := redact.New(salt[:])
	if err != nil {
		return "[redacted]"
	}
	safe, _ := scrubber.Text(value)
	return safe
}

func pathLike(word string) bool {
	word = strings.TrimLeft(word, `"'`+"([{")
	return strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") ||
		strings.Contains(word, `\`) || strings.Contains(word, "://")
}
