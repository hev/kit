package attention

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hev/kit/internal/secrets"
)

// SlackMessage is one chat.postMessage call.
type SlackMessage struct {
	Token    string
	Channel  string
	Text     string
	ThreadTS string
}

// Poster posts a message and returns the channel and ts Slack answered with.
type Poster interface {
	Post(ctx context.Context, m SlackMessage) (channel, ts string, err error)
}

// SlackAPIEnv overrides the Web API base URL (a fake server in tests).
const SlackAPIEnv = "HEV_SLACK_API_URL"

// SlackHTTP posts through Slack's Web API.
type SlackHTTP struct {
	Base   string
	Client *http.Client
}

// DefaultPoster is the Web API at slack.com, or HEV_SLACK_API_URL.
func DefaultPoster() Poster {
	base := os.Getenv(SlackAPIEnv)
	if base == "" {
		base = "https://slack.com/api"
	}
	return SlackHTTP{Base: strings.TrimRight(base, "/"), Client: &http.Client{Timeout: 15 * time.Second}}
}

// SlackError is an error Slack answered with (ok:false).
type SlackError struct{ Code string }

func (e SlackError) Error() string { return "slack: " + e.Code }

// UncertainError is a post whose request reached Slack but whose answer was
// lost (a dropped connection, a timeout, an unreadable 2xx). Slack may have
// posted it; chat.postMessage has no idempotency key, so it is never posted
// again automatically.
type UncertainError struct{ Err error }

func (e UncertainError) Error() string { return e.Err.Error() }
func (e UncertainError) Unwrap() error { return e.Err }

// Post calls chat.postMessage with unfurling off. Errors never carry the
// token.
func (s SlackHTTP) Post(ctx context.Context, m SlackMessage) (string, string, error) {
	body := map[string]any{"channel": m.Channel, "text": m.Text, "unfurl_links": false, "unfurl_media": false}
	if m.ThreadTS != "" {
		body["thread_ts"] = m.ThreadTS
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Base+"/chat.postMessage", bytes.NewReader(b))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+m.Token)
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	// Whether the request reached the wire decides if a transport error is
	// a definite miss (retry) or an unknown outcome (never repost).
	// WroteRequest runs on the transport's goroutine.
	var wrote atomic.Bool
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		WroteRequest: func(httptrace.WroteRequestInfo) { wrote.Store(true) },
	}))
	resp, err := client.Do(req)
	if err != nil {
		// url.Error carries the URL, never the header.
		err = fmt.Errorf("slack: %w", err)
		if wrote.Load() {
			return "", "", UncertainError{Err: err}
		}
		return "", "", err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil && resp.StatusCode/100 == 2 {
		return "", "", UncertainError{Err: fmt.Errorf("slack: reading response: %w", readErr)}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", "", SlackError{Code: "rate_limited"}
	}
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		// A 2xx Slack may have acted on: unknown, not a miss.
		return "", "", UncertainError{Err: fmt.Errorf("slack: invalid response: %v", err)}
	}
	if !out.OK {
		return "", "", SlackError{Code: out.Error}
	}
	return out.Channel, out.TS, nil
}

// threadGone reports a reply whose thread can no longer be used; the notice
// then starts a new thread.
func threadGone(err error) bool {
	var se SlackError
	return errors.As(err, &se) && (se.Code == "thread_not_found" || se.Code == "channel_not_found" || se.Code == "message_not_found")
}

// legacyName is an item title kit's vault-neutral NAME rule refuses
// (secret://SLACK_BOT_TOKEN). 1Password titles allow it.
var legacyName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,127}$`)

// ResolveToken reads the bot token through hev secrets (the local cache
// daemon, then a direct read). The value never leaves the process.
var ResolveToken = func(ctx context.Context, ref string) (string, error) {
	uri := ref
	if secrets.IsNeutral(ref) && !secrets.ValidNeutral(ref) {
		// secret://SLACK_BOT_TOKEN names a 1Password item whose title is
		// not a portable neutral NAME. On a 1Password host it is still
		// unambiguous: item NAME in the host vault and field.
		name := strings.TrimPrefix(ref, "secret://")
		m, err := secrets.HostManager()
		if err != nil {
			return "", err
		}
		if m.Kind != secrets.ManagerOnePassword || !legacyName.MatchString(name) {
			return "", fmt.Errorf("%s: NAME must be lowercase letters, digits and hyphens on this host", ref)
		}
		uri = "op://" + m.Vault + "/" + name + "/" + m.Field
	} else {
		var err error
		if uri, err = secrets.ExpandRef(ref); err != nil {
			return "", err
		}
	}
	refs := []secrets.Ref{{Name: "SLACK_BOT_TOKEN", URI: uri, TTL: time.Hour}}
	caller := secrets.DefaultCaller("kit notify")
	values, err := secrets.ResolveWith(ctx, refs, secrets.Options{Caller: caller, CacheOnly: true})
	if errors.Is(err, secrets.ErrUnavailable) {
		values, err = secrets.ResolveWith(ctx, refs, secrets.Options{Caller: caller, Notice: io.Discard})
	}
	if err != nil {
		return "", fmt.Errorf("read slack token %s: %w", ref, err)
	}
	tok := strings.TrimSpace(values["SLACK_BOT_TOKEN"])
	if tok == "" {
		return "", fmt.Errorf("read slack token %s: empty", ref)
	}
	return tok, nil
}
