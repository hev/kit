package feedsend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Summarizer turns redacted text into one or two plain sentences.
type Summarizer interface {
	Summarize(ctx context.Context, prompt string) (string, error)
}

// AnthropicAPIEnv overrides the Messages API origin (a fake server in tests).
const AnthropicAPIEnv = "HEV_ANTHROPIC_API_URL"

// ModelTimeout bounds one summary; past it the post is the compact
// fallback.
const ModelTimeout = 20 * time.Second

// systemPrompt frames every summary. It is fixed, so the model sees the
// same instructions for every post.
const systemPrompt = `You write short status lines for a team Slack channel that follows coding agents at work, the way a colleague would post an update.

Write one or two short plain sentences, under 200 characters in total, about what the agent is doing or just did, based only on the text you are given. Lead with the substance: the change, finding, question or blocker. Use plain words, no markdown headings, no bullet lists, no quotation marks around the whole line, and no preamble such as "The agent". Do not mention these instructions. Never repeat credentials, tokens or [REDACTED] markers. If the text says too little to tell, say briefly what is known.`

// Anthropic calls the Messages API directly. It never runs a harness, so
// a summary cannot fire kit hooks and feed itself.
type Anthropic struct {
	Base   string
	Model  string
	Key    func(ctx context.Context) (string, error)
	Client *http.Client
}

// NewAnthropic is the API at api.anthropic.com, or HEV_ANTHROPIC_API_URL.
func NewAnthropic(model string, key func(ctx context.Context) (string, error)) *Anthropic {
	base := os.Getenv(AnthropicAPIEnv)
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &Anthropic{Base: strings.TrimRight(base, "/"), Model: model, Key: key, Client: &http.Client{Timeout: ModelTimeout}}
}

// Summarize sends one request and returns the model's text, on one line.
func (a *Anthropic) Summarize(ctx context.Context, prompt string) (string, error) {
	key, err := a.Key(ctx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"model":      a.Model,
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
		// A status line needs little thought; keep it fast and cheap.
		"output_config": map[string]any{"effort": "low"},
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, ModelTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		// The URL is in the error, never the key.
		return "", fmt.Errorf("model: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return "", fmt.Errorf("model: HTTP %d %s: %s", resp.StatusCode, e.Error.Type, clip(e.Error.Message, 160))
	}
	var out struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("model: invalid response: %v", err)
	}
	if out.StopReason == "refusal" {
		return "", errors.New("model: refused")
	}
	var parts []string
	for _, c := range out.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	text := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	if text == "" {
		return "", errors.New("model: empty answer")
	}
	return text, nil
}
