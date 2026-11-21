package layer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hev/kit/internal/board/settings"
	kitlayer "github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/storagemode"
	"github.com/hev/kit/pkg/search"
)

const DefaultModel = kitlayer.DefaultModel

type Client struct {
	transport *kitlayer.Client
	Endpoint  string
	APIKey    string
	Namespace string
	Model     string
	HTTP      *http.Client
	// storeAware is set by ForStore: the text column is declared and searched
	// the way the transport's store takes them, instead of the hosted
	// multi-query wire the team board has always used.
	storeAware bool
}

func New(endpoint, key, namespace, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	transport := kitlayer.New(endpoint, key, namespace, model)
	return &Client{transport: transport, Endpoint: strings.TrimRight(endpoint, "/"), APIKey: key, Namespace: namespace, Model: model, HTTP: transport.HTTP}
}

// ForStore wraps a kit Layer client whose store capabilities are already
// resolved (kitlayer.Client.WithStore). The board then declares its text
// column with the store's embedding and searches by the store's hybrid route,
// so it also works on a store that answers only the hybrid route, with no
// multi-query.
func ForStore(transport *kitlayer.Client) *Client {
	return &Client{transport: transport, Endpoint: transport.Endpoint, APIKey: transport.APIKey, Namespace: transport.Namespace, Model: transport.Model, HTTP: transport.HTTP, storeAware: true}
}

// KeyOptions is how the board asks storagemode for its key: its own
// FACTORY_BOARD_LAYER_API_KEY first, then kit's usual lookup.
var KeyOptions = storagemode.Options{EnvKeys: []string{"FACTORY_BOARD_LAYER_API_KEY"}}

// Load is the Layer client for a board that has a key. With no key the board
// runs in filesystem mode instead; see store.Open.
func Load() (*Client, error) {
	r, err := storagemode.Resolve(KeyOptions)
	if err != nil {
		return nil, err
	}
	if r.Mode != storagemode.Layer {
		return nil, fmt.Errorf("no board Layer API key configured")
	}
	return WithKey(r.Key)
}

// WithKey is the Layer client for a key storagemode already resolved.
func WithKey(key string) (*Client, error) {
	c := settings.Current
	if c.Endpoint == "" || c.Namespace == "" {
		return nil, fmt.Errorf("a Layer API key is configured, so the board uses Layer: configure board.layer_endpoint and board.namespace before serving (unset the key to keep the board in files)")
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("board Layer API key is empty")
	}
	return New(c.Endpoint, key, c.Namespace, c.Model), nil
}

func (c *Client) Schema() map[string]any {
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "int"}
	boolean := map[string]any{"type": "bool"}
	strs := map[string]any{"type": "[]string"}
	text := map[string]any{
		"type":             "string",
		"full_text_search": true,
		"embed":            map[string]any{"model": c.Model},
	}
	schema := map[string]any{
		"text": text,
		"kind": str, "thread_id": num, "n": num,
		"board": str, "title": str, "tags": strs, "body": str,
		"author": str, "author_name": str, "role": str, "instance": str, "host": str, "session": str,
		"created": str, "updated": str,
		"replies": num, "views": num, "useful_by": strs,
		"pinned": boolean, "locked": boolean,
		"last_author": str, "last_created": str, "last_n": num,
		// Image attachments: metadata only, as JSON; the bytes stay on the
		// serving host's disk. Absent on rows without images.
		"images": str, "image_count": num,
	}
	if c.storeAware {
		// An app board also keeps the display name its host app asserts.
		schema["text"] = c.transport.TextField()
		schema["author_name"] = str
		// Lists are JSON strings on an app board (board/store.NewApp).
		schema["tags"], schema["useful_by"] = str, str
	}
	return schema
}

type Row = map[string]any

type writeResponse struct {
	Status       string `json:"status"`
	Error        string `json:"error"`
	RowsUpserted int    `json:"rows_upserted"`
	RowsPatched  int    `json:"rows_patched"`
}

func (c *Client) Upsert(rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	return c.write(map[string]any{
		"upsert_rows":     rows,
		"distance_metric": "cosine_distance",
		"schema":          c.Schema(),
	})
}
func (c *Client) Patch(rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	for _, r := range rows {
		if _, has := r["text"]; has {
			return fmt.Errorf("layer patch: text is write-once; upsert the row")
		}
	}
	return c.write(map[string]any{"patch_rows": rows, "schema": c.Schema()})
}
func (c *Client) write(body map[string]any) error {
	var out writeResponse
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace, body, &out); err != nil {
		return err
	}
	if out.Status != "OK" && out.Error != "" {
		return fmt.Errorf("layer write: %s", out.Error)
	}
	return nil
}
func (c *Client) Query(body map[string]any) ([]Row, error) {
	var out struct {
		Rows  []Row  `json:"rows"`
		Error string `json:"error"`
	}
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
		if isMissingNamespace(err) {
			return nil, nil
		}
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	return out.Rows, nil
}
func (c *Client) Search(q search.Query) ([]Row, error) {
	if c.SearchRoute() == "layer.hybrid_text" {
		rows, err := c.transport.SearchRows(strings.Join(q.Phrasings, "\n"), q.TopK, q.Filter, q.Attrs)
		if err != nil {
			return nil, err
		}
		out := make([]Row, len(rows))
		for i, r := range rows {
			out[i] = r
		}
		return out, nil
	}
	body, err := q.Body()
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &raw); err != nil {
		if isMissingNamespace(err) {
			return nil, nil
		}
		return nil, err
	}
	return search.Rows[Row](raw)
}
func isMissingNamespace(err error) bool {
	s := err.Error()
	return strings.Contains(s, "404") && strings.Contains(strings.ToLower(s), "not found")
}

func (c *Client) do(method, path string, body any, out any) error {
	return c.transport.Request(method, path, body, out)
}

// DeleteWhere removes rows selected by a namespace-local filter.
func (c *Client) DeleteWhere(filter any) error {
	return c.write(map[string]any{"delete_by_filter": filter})
}

// SearchRoute reports the Layer route selected by this client.
func (c *Client) SearchRoute() string {
	if route, err := c.transport.Caps.SearchRoute(); err == nil && route == kitlayer.RouteHybridText {
		return "layer.hybrid_text"
	}
	return "layer.multi_query"
}

// RoutedSearch opens a request-scoped search client using the namespace's
// advertised routes. Older gateways use the original multi-query contract.
func (c *Client) RoutedSearch(ctx context.Context) (*Client, error) {
	var inventory kitlayer.Capabilities
	err := c.transport.WithContext(ctx).Request("GET", "/v2/namespaces/"+c.Namespace+"/capabilities", nil, &inventory)
	var upstream *kitlayer.HTTPError
	if errors.As(err, &upstream) && (upstream.Status == 404 || upstream.Status == 405 || upstream.Status == 501) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	caps, err := kitlayer.ResolveCapabilities(&inventory, "")
	if err != nil {
		return nil, err
	}
	if _, err := caps.SearchRoute(); err != nil {
		return nil, err
	}
	transport := *c.transport
	transport.Caps = caps
	routed := *c
	routed.transport = &transport
	return &routed, nil
}

// WithContext scopes the transport to this operation without mutating the client.
func (c *Client) WithContext(ctx context.Context) *Client {
	copy := *c
	copy.transport = c.transport.WithContext(ctx)
	return &copy
}

// SearchSemantic reports whether the selected route includes a semantic leg.
func (c *Client) SearchSemantic() bool {
	return c.SearchRoute() == "layer.multi_query" || c.transport.Caps.CanEmbed()
}

// WithHTTP sends the board's requests through h.
func (c *Client) WithHTTP(h *http.Client) *Client {
	c.HTTP, c.transport.HTTP = h, h
	return c
}
