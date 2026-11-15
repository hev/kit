package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Class says what a failed post means for its receipt (RFC 0017 section 3).
type Class string

const (
	ClassTransient   Class = "transient"    // network, timeout, 5xx: back off
	ClassRateLimited Class = "rate-limited" // server named a retry time
	ClassCredential  Class = "credential"   // token refused: retry, expire after 24h
	ClassChannel     Class = "channel"      // channel refused: failed
	ClassPayload     Class = "payload"      // message refused: failed (a kit bug)
	ClassThread      Class = "thread"       // thread anchor refused: post at top level
	ClassConfig      Class = "config"       // the node does not permit it: failed
)

// Error is a classified failure. Its text is the platform's error code and
// HTTP status only: never a response body, a request URL or a credential.
type Error struct {
	Class   Class
	Code    string
	Status  int
	RetryAt time.Time
	// Local marks an error raised on this node before any request, such as
	// a missing notifier; its Code is the whole message.
	Local bool
}

// LocalError is a failure on this node, before any request was made.
func LocalError(class Class, text string) error { return &Error{Class: class, Code: text, Local: true} }

func (e *Error) Error() string {
	if e.Local {
		return e.Code
	}
	text := "slack: " + e.Code
	if e.Status != 0 && e.Status != http.StatusOK {
		text += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	return text
}

// Classify returns the class of err, treating unknown errors as transient.
func Classify(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ClassTransient
}

// RetryAt returns the server's retry time for a rate-limited error.
func RetryAt(err error) time.Time {
	var e *Error
	if errors.As(err, &e) {
		return e.RetryAt
	}
	return time.Time{}
}

// slackCodes maps documented chat.postMessage error codes
// (https://docs.slack.dev/reference/methods/chat.postMessage) to classes.
// Codes not listed are transient: the normal backoff applies.
var slackCodes = map[string]Class{
	"invalid_auth":            ClassCredential,
	"not_authed":              ClassCredential,
	"token_revoked":           ClassCredential,
	"token_expired":           ClassCredential,
	"account_inactive":        ClassCredential,
	"missing_scope":           ClassCredential,
	"not_allowed_token_type":  ClassCredential,
	"team_access_not_granted": ClassCredential,

	"channel_not_found":                   ClassChannel,
	"not_in_channel":                      ClassChannel,
	"is_archived":                         ClassChannel,
	"restricted_action_read_only_channel": ClassChannel,
	"ekm_access_denied":                   ClassChannel,

	"no_text":             ClassPayload,
	"msg_blocks_too_long": ClassPayload,
	"invalid_arguments":   ClassPayload,
	"invalid_blocks":      ClassPayload,
	"invalid_post_type":   ClassPayload,
	"invalid_charset":     ClassPayload,

	// Slack documents no thread_ts error code; these are the refusals of a
	// reply to a given parent message.
	"cannot_reply_to_message":         ClassThread,
	"restricted_action_thread_locked": ClassThread,

	"ratelimited":  ClassRateLimited,
	"rate_limited": ClassRateLimited,
}

// SlackAPI is the production Web API base URL. Tests point a client at a fake.
const SlackAPI = "https://slack.com/api/"

// Slack is a bot-token client for the three methods kit uses.
type Slack struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Now     func() time.Time
}

func (s Slack) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Escape neutralises Slack's control characters so composed text cannot
// form a mention or link (https://docs.slack.dev/messaging/formatting-message-text):
// "&", "<" and ">" become HTML entities. kit never sets link_names.
func Escape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

type slackReply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// call posts JSON for write methods and a form for read methods: Slack's
// read methods document only application/x-www-form-urlencoded bodies
// (https://docs.slack.dev/apis/web-api/#post-bodies).
func (s Slack) call(ctx context.Context, method string, body any, out any) error {
	var b []byte
	contentType := "application/json; charset=utf-8"
	if form, ok := body.(url.Values); ok {
		b, contentType = []byte(form.Encode()), "application/x-www-form-urlencoded"
	} else {
		var err error
		if b, err = json.Marshal(body); err != nil {
			return &Error{Class: ClassPayload, Code: "encode"}
		}
	}
	base := s.BaseURL
	if base == "" {
		base = SlackAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/"+method, bytes.NewReader(b))
	if err != nil {
		return &Error{Class: ClassConfig, Code: "invalid request"}
	}
	// The token travels only in the Authorization header.
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", contentType)
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		// A *url.Error carries the request URL; report only the kind.
		code := "network error"
		var ue *url.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ue) && ue.Timeout()) {
			code = "timeout"
		}
		return &Error{Class: ClassTransient, Code: code}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	// HTTP 429 with Retry-After in seconds
	// (https://docs.slack.dev/apis/web-api/rate-limits).
	if resp.StatusCode == http.StatusTooManyRequests {
		return &Error{Class: ClassRateLimited, Code: "ratelimited", Status: resp.StatusCode, RetryAt: s.retryAt(resp)}
	}
	if resp.StatusCode >= 500 {
		return &Error{Class: ClassTransient, Code: "server error", Status: resp.StatusCode}
	}
	var r slackReply
	if err := json.Unmarshal(data, &r); err != nil {
		return &Error{Class: ClassTransient, Code: "invalid response", Status: resp.StatusCode}
	}
	if !r.OK {
		code := r.Error
		if !validCode(code) {
			code = "unknown error"
		}
		class, ok := slackCodes[code]
		if !ok {
			class = ClassTransient
		}
		e := &Error{Class: class, Code: code, Status: resp.StatusCode}
		if class == ClassRateLimited {
			e.RetryAt = s.retryAt(resp)
		}
		return e
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return &Error{Class: ClassTransient, Code: "invalid response", Status: resp.StatusCode}
		}
	}
	return nil
}

// validCode keeps error text to Slack's snake_case codes, so a hostile or
// broken server cannot put arbitrary text on a receipt.
func validCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func (s Slack) retryAt(resp *http.Response) time.Time {
	secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	if err != nil || secs < 0 {
		return time.Time{}
	}
	return s.now().Add(time.Duration(min(secs, 24*3600)) * time.Second)
}

// Post sends text with chat.postMessage, as a reply when thread is set. It
// returns the message reference (CHANNEL/TS) and the anchor later posts
// should reply under: the root's ts, which for a reply is thread itself.
func (s Slack) Post(ctx context.Context, channel, text, thread string) (ref, threadRef string, err error) {
	body := map[string]any{"channel": channel, "text": Escape(text), "unfurl_links": false, "unfurl_media": false}
	if thread != "" {
		body["thread_ts"] = thread
	}
	var r struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err = s.call(ctx, "chat.postMessage", body, &r); err != nil {
		return "", "", err
	}
	if r.TS == "" {
		return "", "", &Error{Class: ClassTransient, Code: "invalid response"}
	}
	if r.Channel == "" {
		r.Channel = channel
	}
	threadRef = thread
	if threadRef == "" {
		threadRef = r.TS
	}
	return r.Channel + "/" + r.TS, threadRef, nil
}

// AuthTest checks the token and returns the workspace name. auth.test needs
// no scope (https://docs.slack.dev/reference/methods/auth.test).
func (s Slack) AuthTest(ctx context.Context) (team string, err error) {
	var r struct {
		Team string `json:"team"`
	}
	if err = s.call(ctx, "auth.test", url.Values{}, &r); err != nil {
		return "", err
	}
	return r.Team, nil
}

// Membership reads is_member from conversations.info
// (https://docs.slack.dev/reference/methods/conversations.info;
// https://docs.slack.dev/reference/objects/conversation-object). Without
// channels:read/groups:read Slack answers missing_scope, which callers report
// as membership unknown.
func (s Slack) Membership(ctx context.Context, channel string) (member bool, err error) {
	var r struct {
		Channel struct {
			IsMember bool `json:"is_member"`
		} `json:"channel"`
	}
	if err = s.call(ctx, "conversations.info", url.Values{"channel": {channel}}, &r); err != nil {
		return false, err
	}
	return r.Channel.IsMember, nil
}

// IsCode reports whether err is the given Slack error code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}
