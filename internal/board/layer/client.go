package layer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
}

func New(endpoint, key, namespace, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	transport := kitlayer.New(endpoint, key, namespace, model)
	return &Client{transport: transport, Endpoint: strings.TrimRight(endpoint, "/"), APIKey: key, Namespace: namespace, Model: model, HTTP: transport.HTTP}
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
	out := map[string]any{
		"text": map[string]any{
			"type":             "string",
			"full_text_search": true,
			"embed":            map[string]any{"model": c.Model},
		},
		"kind": str, "thread_id": num, "n": num,
		// title and body are left out on purpose: `hev board facets schema
		// text` declares them filterable:false in its own write, and an
		// ordinary write that redeclared them would race that setting.
		"board": str, "tags": strs,
		"author": str, "role": str, "instance": str, "host": str, "session": str,
		"created": str, "updated": str,
		"replies": num, "views": num, "useful_by": strs,
		"pinned": boolean, "locked": boolean,
		"last_author": str, "last_created": str, "last_n": num,
		// Image attachments: metadata only, as JSON; the bytes stay on the
		// serving host's disk. Absent on rows without images.
		"images": str, "image_count": num,
	}
	for k, v := range FacetSchema() {
		out[k] = v
	}
	return out
}

// FacetAttrs are the six thread-row attributes the facets add: one
// filterable string array per facet, and who last set them.
var FacetAttrs = []string{"facet_repo", "facet_host", "facet_tool", "facet_area", "facet_client", "facets_by"}

// FacetSchema declares the facet attributes explicitly. It is the first of
// the two schema writes, and part of every ordinary write's schema.
func FacetSchema() map[string]any {
	out := map[string]any{}
	for _, k := range FacetAttrs {
		typ := "[]string"
		if k == "facets_by" {
			typ = "string"
		}
		out[k] = map[string]any{"type": typ, "filterable": true}
	}
	return out
}

// TextSchema is the second, separate schema write: title and body declared
// filterable:false. No board query filters on either.
func TextSchema() map[string]any {
	return map[string]any{
		"title": map[string]any{"type": "string", "filterable": false},
		"body":  map[string]any{"type": "string", "filterable": false},
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

// ReadSchema is the namespace's current schema. A namespace that does not
// exist yet has none.
func (c *Client) ReadSchema() (map[string]map[string]any, error) {
	var out map[string]map[string]any
	if err := c.do("GET", "/v1/namespaces/"+c.Namespace+"/schema", nil, &out); err != nil {
		if isMissingNamespace(err) {
			return map[string]map[string]any{}, nil
		}
		return nil, err
	}
	return out, nil
}

// SchemaChange is one attribute a schema write would change.
type SchemaChange struct {
	Attr string         `json:"attr"`
	From map[string]any `json:"from,omitempty"` // nil: not declared yet
	To   map[string]any `json:"to"`
}

// PlanSchema compares want (FacetSchema or TextSchema) with the namespace's
// schema and returns only what differs, so a rerun after a write plans
// nothing. An existing attribute keeps every setting it has except
// filterable; a type that does not match is an error, never a rewrite.
func (c *Client) PlanSchema(want map[string]any) ([]SchemaChange, error) {
	current, err := c.ReadSchema()
	if err != nil {
		return nil, err
	}
	return PlanSchemaFrom(current, want)
}

// PlanSchemaFrom is PlanSchema against a schema already read.
func PlanSchemaFrom(current map[string]map[string]any, want map[string]any) ([]SchemaChange, error) {
	var out []SchemaChange
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w := want[k].(map[string]any)
		have, ok := current[k]
		if !ok || have == nil {
			out = append(out, SchemaChange{Attr: k, To: w})
			continue
		}
		if t, _ := have["type"].(string); t != "" && t != w["type"] {
			return nil, fmt.Errorf("attribute %s is %s in the namespace, want %s; no schema write planned", k, t, w["type"])
		}
		if _, embedded := have["embed"]; embedded {
			return nil, fmt.Errorf("attribute %s carries an embedding; no schema write planned", k)
		}
		// filterable defaults to true when the store reports nothing.
		f, set := have["filterable"].(bool)
		if !set {
			f = true
		}
		if f == w["filterable"] {
			continue
		}
		to := map[string]any{}
		for kk, v := range have {
			if v != nil { // a null setting is the store's default; never send it
				to[kk] = v
			}
		}
		to["type"], to["filterable"] = w["type"], w["filterable"]
		out = append(out, SchemaChange{Attr: k, From: have, To: to})
	}
	return out, nil
}

// ApplySchema is a schema-only write of the planned changes: no rows. An
// empty plan writes nothing.
func (c *Client) ApplySchema(changes []SchemaChange) error {
	if len(changes) == 0 {
		return nil
	}
	schema := map[string]any{}
	for _, ch := range changes {
		schema[ch.Attr] = ch.To
	}
	return c.write(map[string]any{"schema": schema})
}
