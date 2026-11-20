package notify

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/internal/secrets"
)

var retryAfterStamp = regexp.MustCompile(`retry after (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`)

// ResolveSecret reads a notifier's token through hev secrets. Errors
// name the notifier and class, never the reference or value.
func ResolveSecret(ctx context.Context, n Notifier, fresh bool) (string, error) {
	ttl := 5 * time.Minute
	if fresh {
		ttl = 0 // A zero TTL requires a new read (RFC 0009).
	}
	values, err := secrets.Resolve(ctx, []secrets.Ref{{Name: "HEV_NOTIFY_TOKEN", URI: n.Token, TTL: ttl}})
	if err != nil {
		text := err.Error()
		if m := retryAfterStamp.FindStringSubmatch(text); m != nil {
			at, _ := time.Parse(time.RFC3339, m[1])
			return "", &Error{Class: ClassRateLimited, Code: "secrets cooldown for notifier " + n.Name, RetryAt: at, Local: true}
		}
		if strings.Contains(text, "service-account token is required") || strings.Contains(text, "invalid secret reference") {
			return "", LocalError(ClassConfig, "no secrets provider for notifier "+n.Name)
		}
		return "", LocalError(ClassTransient, "secret resolution failed for notifier "+n.Name)
	}
	token := strings.TrimSpace(values["HEV_NOTIFY_TOKEN"])
	if token == "" {
		return "", LocalError(ClassCredential, "empty credential for notifier "+n.Name)
	}
	return token, nil
}
