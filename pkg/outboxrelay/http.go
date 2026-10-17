package outboxrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTP implements Layer's row/query wire for a dedicated projection namespace.
// Configuration is explicit; it does not consult environment or stored secrets.
// The namespace must not be shared with trace indexing or another destination.
type HTTP struct {
	Endpoint, Namespace, APIKey string
	Client                      *http.Client
}

type wireDocument struct {
	ID      string `json:"id"`
	Key     string `json:"idempotency_key"`
	Order   string `json:"order_id"`
	Payload string `json:"payload_json"`
	Text    string `json:"text"`
}

func (h HTTP) request(ctx context.Context, path string, body any, out any) error {
	if h.Endpoint == "" || h.Namespace == "" {
		return errors.New("explicit Layer endpoint and namespace required")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(h.Endpoint, "/")+"/v2/namespaces/"+url.PathEscape(h.Namespace)+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Layer HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}
func (h HTTP) Put(ctx context.Context, d Document) error {
	var out struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Rows   int    `json:"rows_upserted"`
	}
	row := wireDocument{d.ID, d.Key, d.Order, string(d.Payload), string(d.Payload)}
	schema := map[string]any{}
	for _, name := range []string{"idempotency_key", "order_id", "payload_json", "text"} {
		schema[name] = map[string]any{"type": "string"}
	}
	// Text is searchable without asking Layer for transactions or batch guarantees.
	schema["text"] = map[string]any{"type": "string", "full_text_search": true}
	err := h.request(ctx, "", map[string]any{"upsert_rows": []wireDocument{row}, "schema": schema}, &out)
	if err != nil {
		return err
	}
	if out.Error != "" || out.Rows != 1 {
		return errors.New("Layer did not acknowledge one document")
	}
	return nil
}
func (h HTTP) Inspect(ctx context.Context, id string) (*Document, error) {
	var out struct {
		Rows  []wireDocument `json:"rows"`
		Error string         `json:"error"`
	}
	err := h.request(ctx, "/query", map[string]any{"filters": []any{"id", "Eq", id}, "rank_by": []any{"id", "asc"}, "top_k": 1, "include_attributes": true}, &out)
	if err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, errors.New("Layer query failed")
	}
	if len(out.Rows) == 0 {
		return nil, nil
	}
	row := out.Rows[0]
	if row.ID != id || !json.Valid([]byte(row.Payload)) {
		return nil, ErrConflict
	}
	var value any
	dec := json.NewDecoder(strings.NewReader(row.Payload))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &Document{ID: row.ID, Key: row.Key, Order: row.Order, Payload: canonical}, nil
}
