package foh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Message is one OpenAI-compatible chat message.
type Message struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	Images     []ImageAttachment `json:"-"`
	ToolCalls  []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

// MarshalJSON encodes plain text messages as a string content field and
// multimodal messages (with Images) as an OpenAI/OpenRouter content-part array.
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.Images) == 0 {
		type plain Message
		return json.Marshal(plain(m))
	}
	parts := make([]map[string]any, 0, len(m.Images)+1)
	if m.Content != "" {
		parts = append(parts, map[string]any{"type": "text", "text": m.Content})
	}
	for _, img := range m.Images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": img.DataURL()},
		})
	}
	return json.Marshal(struct {
		Role       string           `json:"role"`
		Content    []map[string]any `json:"content"`
		ToolCalls  []ToolCall       `json:"tool_calls,omitempty"`
		ToolCallID string           `json:"tool_call_id,omitempty"`
		Name       string           `json:"name,omitempty"`
	}{
		Role:       m.Role,
		Content:    parts,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
		Name:       m.Name,
	})
}

// ToolCall is an assistant request to run a tool.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall names the tool and carries its JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSpec advertises a tool to the model.
type ToolSpec struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec is a tool's name, description and JSON Schema parameters.
type FunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Usage is the token accounting the provider reports for one request.
type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
}

// ChatRequest is one streamed completion request.
type ChatRequest struct {
	Model    string     `json:"model"`
	Messages []Message  `json:"messages"`
	Tools    []ToolSpec `json:"tools,omitempty"`
}

// ChatResult is an assembled streamed response.
type ChatResult struct {
	Model        string
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

// Delta is one streamed piece of a response: text or a tool call fragment.
type Delta struct {
	Text     string
	ToolName string // set when a tool call starts
}

const (
	// DefaultRequestTimeout bounds one streamed completion end to end.
	DefaultRequestTimeout = 5 * time.Minute
	maxSSELine            = 4 << 20
	maxErrorBody          = 4 << 10
	maxResponseBytes      = 32 << 20
)

// Client is an OpenRouter (OpenAI-compatible) streaming client. It is the
// openrouter Provider.
type Client struct {
	BaseURL string
	// Catalog lists the models; nil uses a default cache-backed Catalog.
	Catalog *Catalog
	APIKey  string
	HTTP    *http.Client
	// Timeout bounds each request; zero uses DefaultRequestTimeout.
	Timeout time.Duration
	// Referer and Title identify kit to OpenRouter's app attribution.
	Referer, Title string
}

// NewClient returns a client for baseURL with key.
func NewClient(baseURL, key string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{},
		Referer: "https://hevkit.com", Title: "hev foh"}
}

// ID is ProviderOpenRouter.
func (c *Client) ID() string { return ProviderOpenRouter }

// Secrets is the API key, for scrubbing.
func (c *Client) Secrets() []string { return []string{c.APIKey} }

// Models lists the cached OpenRouter catalog, each model stamped openrouter.
func (c *Client) Models(ctx context.Context, refresh bool) ([]Model, CatalogSource, error) {
	cat := c.Catalog
	if cat == nil {
		cat = &Catalog{Client: c}
	}
	models, src, err := cat.Models(ctx, refresh)
	out := make([]Model, len(models))
	for i, m := range models {
		m.Provider = ProviderOpenRouter
		out[i] = m
	}
	return out, src, err
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultRequestTimeout
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return nil, c.scrub(err)
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if c.Referer != "" {
		req.Header.Set("HTTP-Referer", c.Referer)
	}
	if c.Title != "" {
		req.Header.Set("X-Title", c.Title)
	}
	return req, nil
}

func (c *Client) scrub(err error) error { return scrubErr(err, c.APIKey) }

// statusError reads a bounded, scrubbed error body.
func (c *Client) statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	msg := strings.TrimSpace(string(b))
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error.Message != "" {
		msg = e.Error.Message
	}
	return c.scrub(fmt.Errorf("openrouter: %s: %s", resp.Status, msg))
}

type streamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream sends req with stream=true and assembles the SSE response, calling
// onDelta for each text fragment and each tool call start.
func (c *Client) Stream(ctx context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	body, err := json.Marshal(struct {
		ChatRequest
		Stream        bool `json:"stream"`
		StreamOptions any  `json:"stream_options"`
		Usage         any  `json:"usage"`
	}{req, true, map[string]bool{"include_usage": true}, map[string]bool{"include": true}})
	if err != nil {
		return ChatResult{}, err
	}
	hreq, err := c.newRequest(ctx, http.MethodPost, "/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient().Do(hreq)
	if err != nil {
		return ChatResult{}, c.scrub(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ChatResult{}, c.statusError(resp)
	}
	res := ChatResult{Model: req.Model}
	var text strings.Builder
	calls := map[int]*ToolCall{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxResponseBytes))
	sc.Buffer(make([]byte, 64<<10), maxSSELine)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue // comments (": OPENROUTER PROCESSING"), event names, blanks
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var ch streamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			continue
		}
		if ch.Error != nil {
			return res, c.scrub(fmt.Errorf("openrouter: %s", ch.Error.Message))
		}
		if ch.Model != "" {
			res.Model = ch.Model
		}
		if ch.Usage != nil {
			res.Usage = *ch.Usage
		}
		for _, choice := range ch.Choices {
			if s := choice.Delta.Content; s != "" {
				text.WriteString(s)
				if onDelta != nil {
					onDelta(Delta{Text: s})
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				call := calls[tc.Index]
				if call == nil {
					call = &ToolCall{Type: "function"}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					started := call.Function.Name == ""
					call.Function.Name += tc.Function.Name
					if started && onDelta != nil {
						onDelta(Delta{ToolName: call.Function.Name})
					}
				}
				call.Function.Arguments += tc.Function.Arguments
			}
			if choice.FinishReason != nil {
				res.FinishReason = *choice.FinishReason
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return res, c.scrub(fmt.Errorf("openrouter: stream: %w", ctxErr))
		}
		return res, c.scrub(fmt.Errorf("openrouter: stream: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return res, c.scrub(fmt.Errorf("openrouter: stream: %w", err))
	}
	res.Content = text.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for n, i := range idx {
		call := *calls[i]
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", n)
		}
		if call.Function.Arguments == "" {
			call.Function.Arguments = "{}"
		}
		res.ToolCalls = append(res.ToolCalls, call)
	}
	return res, nil
}

// ErrTimeout reports whether err is a deadline expiry.
func ErrTimeout(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
