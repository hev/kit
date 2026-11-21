package hooks

import (
	"context"
	"crypto/rand"
	"maps"
	"os"
	"strings"
	"time"

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
	start := time.Now().UTC()
	ev := Event{
		Schema: Schema, ID: strings.ToLower(ulid.Make().String()), Time: start,
		Event: event, Harness: "kit", Host: hostlabel.Self(os.Hostname),
		Data: make(map[string]any, len(data)), Reason: scrubMeta(reason),
	}
	for key, value := range data {
		switch value := value.(type) {
		case string:
			ev.Data[key] = scrubMeta(value)
		case bool, int, int64:
			ev.Data[key] = value
		}
	}
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
	handlers := UserHandlers(os.Getenv("PATH"))
	delivery := ev
	if activityBody != nil {
		delivery.Data = maps.Clone(ev.Data)
		delivery.Data["body"] = *activityBody
	}
	r := Run(ctx, delivery, handlers, cfg.Timeout, warn)
	if event != "admission-check" {
		r = Response{Decision: Continue}
	}
	if err := log.Event(ev, r, time.Since(start)); err != nil {
		warn(err.Error())
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
		if _, suffix, ok := strings.Cut(word, "="); ok {
			word = suffix
		}
		word = strings.TrimLeft(word, `"'`+"([{")
		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") ||
			strings.Contains(word, `\`) || strings.Contains(word, "://") {
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
