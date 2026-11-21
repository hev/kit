package hookapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/pkg/kitapi"
)

var deniedTarget = errors.New("webhook target is not public HTTPS")

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	// Global unicast includes some special-purpose address ranges.
	return ip.IsGlobalUnicast() && !ip.IsPrivate() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(ip) &&
		!netip.MustParsePrefix("0.0.0.0/8").Contains(ip) &&
		!netip.MustParsePrefix("192.0.0.0/24").Contains(ip) &&
		!netip.MustParsePrefix("198.18.0.0/15").Contains(ip) &&
		!netip.MustParsePrefix("240.0.0.0/4").Contains(ip) &&
		(!ip.Is6() || netip.MustParsePrefix("2000::/3").Contains(ip)) &&
		!netip.MustParsePrefix("2002::/16").Contains(ip) &&
		!netip.MustParsePrefix("2001::/32").Contains(ip)
}

func publicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, deniedTarget
	}
	for _, ip := range ips {
		if !publicAddress(ip) {
			return nil, deniedTarget
		}
	}
	return ips, nil
}

func validateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, "https://") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return deniedTarget
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = publicIPs(ctx, u.Hostname())
	return err
}

func webhookClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns: 32, IdleConnTimeout: 30 * time.Second,
		MaxResponseHeaderBytes: 16 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, deniedTarget
			}
			ips, err := publicIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			dialer := net.Dialer{Timeout: 5 * time.Second}
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, errors.New("webhook connection failed")
		},
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type attempt struct {
	N       int       `json:"n"`
	At      time.Time `json:"at"`
	Code    *int      `json:"status_code"`
	Latency int64     `json:"latency_ms"`
	Error   string    `json:"error,omitempty"`
}

type delivery struct {
	ID         string     `json:"id"`
	HookID     string     `json:"hook_id"`
	EventID    string     `json:"event_id"`
	Event      string     `json:"event"`
	Blocking   bool       `json:"blocking"`
	Test       bool       `json:"test"`
	Status     string     `json:"status"`
	Code       *int       `json:"status_code"`
	Latency    int64      `json:"latency_ms"`
	Attempts   []attempt  `json:"attempts"`
	Next       *time.Time `json:"next_retry_at,omitempty"`
	Decision   string     `json:"decision,omitempty"`
	FailedOpen bool       `json:"failed_open,omitempty"`
	Created    time.Time  `json:"created_at"`
	Completed  *time.Time `json:"completed_at,omitempty"`
}

var retryDelays = [...]time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 15 * time.Hour}

func settle(d *delivery, a attempt, answer hooks.Response, now time.Time) {
	d.Attempts = append(d.Attempts, a)
	d.Code, d.Latency = a.Code, a.Latency
	d.Next, d.Completed = nil, &now
	switch {
	case a.Error == "":
		d.Status = "succeeded"
	case !d.Blocking && !d.Test && a.nongone() && a.N <= len(retryDelays):
		next := now.Add(retryDelays[a.N-1])
		d.Status, d.Next, d.Completed = "retrying", &next, nil
	default:
		d.Status = "failed"
	}
	if d.Blocking {
		d.Decision, d.FailedOpen = apiDecision(answer.Decision), a.Error != ""
	}
}

func (a attempt) nongone() bool { return a.Code == nil || *a.Code != 410 }

func apiDecision(decision string) string {
	if decision == hooks.Block || decision == hooks.Halt {
		return decision
	}
	return "allow"
}

func (s *Service) send(ctx context.Context, h hook, d delivery, body []byte) (a attempt, result hooks.Response) {
	timeout := 10 * time.Second
	if d.Blocking {
		timeout = time.Duration(h.Timeout) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	a = attempt{N: len(d.Attempts) + 1, At: start.UTC()}
	defer func() { a.Latency = time.Since(start).Milliseconds() }()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		a.Error = "invalid target"
		return
	}
	req.GetBody = nil // A transport replay must never repeat a blocking delivery.
	timestamp := strconv.FormatInt(start.Unix(), 10)
	signature := sign(h.Secret, timestamp, body)
	if !d.Test && h.OldSecret != "" && start.Before(h.OldUntil) {
		signature += "," + sign(h.OldSecret, timestamp, body)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hev-Signature-256", signature)
	req.Header.Set("X-Hev-Timestamp", timestamp)
	req.Header.Set("X-Hev-Delivery-Id", d.ID)
	req.Header.Set("X-Hev-Hook-Id", h.ID)
	req.Header.Set("X-Hev-Event", d.Event)
	if d.Test {
		req.Header.Set("X-Hev-Test", "1")
	}
	response, err := s.client.Do(req)
	if err != nil {
		a.Error = "connection failed"
		if ctx.Err() != nil {
			a.Error = "timeout"
		}
		return
	}
	defer response.Body.Close()
	a.Code = &response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		a.Error = "non-2xx response"
		return
	}
	if !d.Blocking || response.StatusCode == 204 {
		return
	}
	// Both spellings of context may be present in one response.
	const wireLimit = kitapi.HookResultBytes + kitapi.HookTextWireBytes
	raw, err := io.ReadAll(io.LimitReader(response.Body, wireLimit+1))
	if err != nil || len(raw) > wireLimit {
		a.Error = "invalid response body"
		return
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var decoded struct {
		hooks.Response
		AddContext *string `json:"add_context"`
	}
	if bytes.TrimSpace(raw)[0] != '{' || json.Unmarshal(raw, &decoded) != nil ||
		utf8.RuneCountInString(decoded.Context) > kitapi.HookTextRunes ||
		(decoded.AddContext != nil && utf8.RuneCountInString(*decoded.AddContext) > kitapi.HookTextRunes) {
		a.Error = "invalid response body"
		return
	}
	switch decoded.Decision {
	case "", "allow", hooks.Continue:
		decoded.Decision = hooks.Continue
	case hooks.Block:
		if d.Event != "pre-tool" {
			decoded.Decision = hooks.Continue
		}
	case hooks.Halt:
		if d.Event == "session-start" {
			decoded.Decision = hooks.Continue
		}
	default:
		a.Error = "invalid response body"
		return
	}
	if decoded.AddContext != nil {
		decoded.Context = *decoded.AddContext
	}
	if d.Event == "pre-tool" {
		decoded.Context = ""
	}
	decoded.Reason = limitText(decoded.Reason, kitapi.HookTextRunes)
	decoded.SystemMessage = limitText(decoded.SystemMessage, kitapi.HookTextRunes)
	return a, decoded.Response
}

// limitText keeps a UTF-8 prefix and preserves the decision independently of text.
func limitText(text string, maxRunes int) string {
	for i := range text {
		if maxRunes == 0 {
			return text[:i]
		}
		maxRunes--
	}
	return text
}
