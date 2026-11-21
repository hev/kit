package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hev/kit/pkg/kitapi"
)

// HostedHandler forwards events to the configured Kit API and uses the
// caller's context for the entire request, including reading its answer.
func HostedHandler(credentials func() (endpoint, key string)) Handler {
	return Handler{Name: "hosted", Run: func(ctx context.Context, ev Event) (Response, error) {
		endpoint, key := credentials()
		if key == "" {
			return Response{}, nil
		}
		if !kitapi.ValidBaseURL(endpoint) {
			return Response{}, errors.New("invalid hosted hook endpoint")
		}
		body, err := json.Marshal(ev)
		if err != nil {
			return Response{}, errors.New("cannot encode hosted hook event")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/hooks/events", bytes.NewReader(body))
		if err != nil {
			return Response{}, errors.New("cannot build hosted hook request")
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := hostedHTTP.Do(req)
		if err != nil {
			return Response{}, errors.New("hosted hook request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return Response{}, errors.New("hosted hook request rejected")
		}
		return decodeHostedDecision(resp.Body, ev)
	}}
}

var hostedHTTP = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func decodeHostedDecision(body io.Reader, ev Event) (Response, error) {
	const limit = kitapi.HookResultBytes
	var result struct {
		EventID       string `json:"event_id"`
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		Context       string `json:"add_context"`
		SystemMessage string `json:"system_message"`
		BlockingHooks *int   `json:"blocking_hooks"`
		Elapsed       *int   `json:"elapsed_ms"`
	}
	invalid := errors.New("invalid hosted hook answer")
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil || len(raw) > limit || json.Unmarshal(raw, &result) != nil ||
		result.EventID != ev.ID || result.BlockingHooks == nil || result.Elapsed == nil ||
		*result.BlockingHooks < 0 || *result.Elapsed < 0 {
		return Response{}, invalid
	}
	switch result.Decision {
	case "allow":
		result.Decision = Continue
	case Block:
		if ev.Event != "pre-tool" {
			result.Decision = Continue
		}
	case Halt:
		if ev.Event != "pre-tool" && ev.Event != "post-tool" && ev.Event != "prompt-submit" {
			result.Decision = Continue
		}
	default:
		return Response{}, invalid
	}
	return Response{Decision: result.Decision, Reason: result.Reason, Context: result.Context, SystemMessage: result.SystemMessage}, nil
}
