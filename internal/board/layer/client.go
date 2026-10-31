package layer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/daemon"
	kitlayer "github.com/hev/kit/internal/layer"
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
}

func New(endpoint, key, namespace, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	transport := kitlayer.New(endpoint, key, namespace, model)
	return &Client{transport: transport, Endpoint: strings.TrimRight(endpoint, "/"), APIKey: key, Namespace: namespace, Model: model, HTTP: transport.HTTP}
}
func Load() (*Client, error) {
	c := settings.Current
	if c.Endpoint == "" || c.Namespace == "" {
		return nil, fmt.Errorf("configure board.layer_endpoint and board.namespace before serving")
	}
	key := os.Getenv("FACTORY_BOARD_LAYER_API_KEY")
	if key == "" {
		var err error
		key, err = daemon.LayerKey()
		if err != nil {
			return nil, err
		}
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
	return map[string]any{
		"text": map[string]any{
			"type":             "string",
			"full_text_search": true,
			"embed":            map[string]any{"model": c.Model},
		},
		"kind": str, "thread_id": num, "n": num,
		"board": str, "title": str, "tags": strs, "body": str,
		"author": str, "role": str, "instance": str, "host": str, "session": str,
		"created": str, "updated": str,
		"replies": num, "views": num, "useful_by": strs,
		"pinned": boolean, "locked": boolean,
		"last_author": str, "last_created": str, "last_n": num,
	}
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
