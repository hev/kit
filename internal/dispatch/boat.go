package dispatch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Boat uses the public v1 HTTP API; it does not require the Boat CLI.
type Boat struct {
	endpoint, key string
	client        *http.Client
}

func NewBoat(h Host) (*Boat, error) {
	env := h.APIKeyEnv
	if env == "" {
		env = "BOAT_API_KEY"
	}
	key := os.Getenv(env)
	if key == "" {
		return nil, fmt.Errorf("Boat API key environment variable %s is empty", env)
	}
	return &Boat{endpoint: "https://boat.dev/api/v1", key: key, client: &http.Client{Timeout: 11 * time.Minute}}, nil
}
func (b *Boat) call(ctx context.Context, method, path string, body, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, b.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("Boat request failed; mutation outcome may be unknown: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not echo response bodies: a provider error can contain secrets.
		var v struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return fmt.Errorf("Boat %s %s: HTTP %d (%s)", method, strings.Split(path, "?")[0], resp.StatusCode, v.Code)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
func (b *Boat) Create(ctx context.Context, s MachineSpec) (Machine, error) {
	var r struct {
		Sandbox Machine `json:"sandbox"`
	}
	q := map[string]any{"name": s.Name, "type": s.Size, "ttlSeconds": int(s.AutoStop.Seconds()), "labels": map[string]string{"hev.dispatch": s.Host, "hev.order": strings.TrimPrefix(s.Name, "d-")}}
	if s.From != "" {
		q["from"] = s.From
	}
	if s.Environment != "" {
		q["environment"] = s.Environment
	}
	if s.Region != "" {
		return Machine{}, fmt.Errorf("Boat v1 does not expose region selection")
	}
	err := b.call(ctx, "POST", "/sandboxes", q, &r)
	return r.Sandbox, err
}
func (b *Boat) List(ctx context.Context, host string) ([]Machine, error) {
	var all []Machine
	cursor := ""
	for {
		q := url.Values{"label": {"hev.dispatch=" + host}, "limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var r struct {
			Sandboxes []Machine `json:"sandboxes"`
			Page      struct {
				Next string `json:"nextCursor"`
				More bool   `json:"hasMore"`
			} `json:"pageInfo"`
		}
		if err := b.call(ctx, "GET", "/sandboxes?"+q.Encode(), nil, &r); err != nil {
			return nil, err
		}
		all = append(all, r.Sandboxes...)
		if !r.Page.More {
			return all, nil
		}
		if r.Page.Next == "" || r.Page.Next == cursor {
			return nil, fmt.Errorf("Boat invalid pagination")
		}
		cursor = r.Page.Next
	}
}
func (b *Boat) WaitReady(ctx context.Context, id string) error {
	for {
		var r struct {
			Sandbox Machine `json:"sandbox"`
		}
		if err := b.call(ctx, "GET", "/sandboxes/"+url.PathEscape(id), nil, &r); err != nil {
			return err
		}
		switch r.Sandbox.State {
		case "ready", "idle", "running":
			return nil
		case "error", "cancelled", "archived":
			return fmt.Errorf("Boat %s is %s", id, r.Sandbox.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func (b *Boat) Exec(ctx context.Context, id, command string) (string, error) {
	var r struct {
		Stdout    string `json:"stdout"`
		ExitCode  *int   `json:"exitCode"`
		TimedOut  bool   `json:"timedOut"`
		Truncated bool   `json:"stdoutTruncated"`
	}
	err := b.call(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/commands", map[string]any{"command": command, "timeoutSeconds": 600}, &r)
	if err != nil {
		return "", err
	}
	if r.ExitCode == nil || *r.ExitCode != 0 || r.TimedOut || r.Truncated {
		return "", fmt.Errorf("Boat command failed or output truncated (exit=%v, timeout=%v)", r.ExitCode, r.TimedOut)
	}
	return r.Stdout, nil
}
func (b *Boat) Copy(ctx context.Context, id, path string) ([]byte, error) {
	var r struct {
		Content string `json:"content"`
	}
	err := b.call(ctx, "GET", "/sandboxes/"+url.PathEscape(id)+"/files?"+url.Values{"path": {path}, "encoding": {"base64"}}.Encode(), nil, &r)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(r.Content)
}
func (b *Boat) Stop(ctx context.Context, id string) error {
	return b.call(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/stop", map[string]any{}, nil)
}
func (b *Boat) Delete(ctx context.Context, id string) error {
	return b.call(ctx, "DELETE", "/sandboxes/"+url.PathEscape(id), nil, nil)
}
func (b *Boat) Resume(ctx context.Context, id string, ttl time.Duration) error {
	if err := b.call(ctx, "POST", "/sandboxes/"+url.PathEscape(id)+"/resume", map[string]any{"ttlSeconds": int(ttl.Seconds())}, nil); err != nil {
		return err
	}
	return b.WaitReady(ctx, id)
}
