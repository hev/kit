// Package layer writes chunks to a Layer namespace and searches them.
//
// The wire is Turbopuffer's, which Layer speaks natively (layer-pro's embed-wire
// RFC): text goes in, the store embeds at write and at query time, and hybrid
// retrieval is a multi-query fused server-side. kit computes no vectors, runs no
// model, and carries no reranker — every one of those would be a search engine
// living inside a capture daemon.
package layer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/trace"
)

// DefaultModel embeds the archive.
//
// Two things about this line are permanent. Turbopuffer cannot re-embed a
// namespace, so changing the model is a full re-ingest; and a vector column is
// immutable after creation, so a namespace that starts without one can never
// gain it. Qwen3-8B is chosen because all three qwen3 sizes normalize to the
// same 1024-dim f16 column — the larger model costs more per token ($0.05/M
// against $0.01/M) and nothing per byte stored, which makes quality the only
// axis that differs and the backfill of a 100 MB archive about a dollar.
const DefaultModel = "qwen/qwen3-embedding-8b"

// DefaultEndpoint is the region the layer-factory credential belongs to. A
// self-hosted Layer CE gateway is named here instead, and speaks the same wire.
const DefaultEndpoint = "https://gcp-us-central1.turbopuffer.com"

type Client struct {
	Endpoint  string
	APIKey    string
	Namespace string
	Model     string
	// Caps is what the store behind Endpoint can do. New fills it for the
	// hosted lane; WithStore selects another.
	Caps   Capabilities
	HTTP   *http.Client
	timing *Timing
}

func New(endpoint, apiKey, namespace, model string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if model == "" {
		model = DefaultModel
	}
	caps, _ := StaticCapabilities("")
	return &Client{
		Endpoint:  strings.TrimRight(endpoint, "/"),
		APIKey:    apiKey,
		Namespace: namespace,
		Model:     model,
		Caps:      caps,
		HTTP:      &http.Client{Timeout: 3 * time.Minute},
	}
}

// WithStore fills Caps for a configured store kind. There is no runtime
// capability read yet, so ResolveCapabilities is handed nil and answers from
// the static table.
func (c *Client) WithStore(kind string) (*Client, error) {
	caps, err := ResolveCapabilities(nil, kind)
	if err != nil {
		return nil, err
	}
	c.Caps = caps
	return c, nil
}

// schema declares the namespace on every write. `text` is the one embedded and
// full-text-indexed column; everything else is a filter, so a search can be
// scoped the way an operator actually thinks about it — this repo, this plan,
// since Tuesday — instead of hoping the words appear in the prose.
//
// The factory columns (instance, plan, rfc, issue, pr) are declared whether or
// not anything fills them yet. They cost nothing empty, and adding a filterable
// attribute later is cheap while adding the vector column later is impossible;
// declaring the whole shape once is the habit that keeps those two cases from
// being confused.
func (c *Client) schema() map[string]any {
	fts := c.Caps.FullTextFields("text", "workdir")
	str := func(fts bool) map[string]any {
		m := map[string]any{"type": "string"}
		if fts {
			m["full_text_search"] = true
		}
		return m
	}
	return map[string]any{
		"text":       c.textField(),
		"session_id": str(false), "turn_uuid": str(false), "parent_uuid": str(false),
		"seq": map[string]any{"type": "int"}, "block": map[string]any{"type": "int"},
		"part": map[string]any{"type": "int"},
		"ts":   str(false), "role": str(false), "block_type": str(false),
		"tier": str(false), "tool_name": str(false),
		"workdir": str(fts["workdir"]), "branch": str(false), "harness": str(false),
		"source_path": str(false), "host": str(false),
		"instance": str(false), "plan": str(false), "rfc": str(false),
		"issue": str(false), "pr": str(false),
	}
}

func scalarSchema(fields ...string) map[string]any {
	schema := make(map[string]any, len(fields))
	for _, field := range fields {
		typ := "string"
		switch field {
		case "total_tokens", "seq", "start", "end", "ms", "wall_ms", "api_ms", "idle_ms", "prompt_count", "tool_count", "request_count", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens":
			typ = "int"
		case "cost":
			typ = "float"
		case "ok", "has_subagents":
			typ = "bool"
		}
		schema[field] = map[string]any{"type": typ}
	}
	return schema
}

func blockSchema() map[string]any {
	schema := scalarSchema("session_id", "turn_uuid", "seq", "role", "block_type",
		"tool_use_id", "agent_id", "parent_agent_id", "start", "end", "ms", "tool_name", "ok",
		"model", "effort", "request_id", "input_tokens", "output_tokens", "cache_read_tokens",
		"cache_creation_tokens", "cost")
	// A block is read back whole and never filtered on. Filterable strings
	// are capped at 4 KiB, and a tool result is routinely larger than that.
	schema["text"] = map[string]any{"type": "string", "filterable": false}
	return schema
}

func sessionSchema() map[string]any {
	schema := scalarSchema("session_id", "summary", "first_prompt", "harness", "model", "repo_url",
		"branch", "host", "start", "end", "wall_ms", "api_ms", "idle_ms", "prompt_count",
		"tool_count", "request_count", "input_tokens", "output_tokens", "cache_read_tokens",
		"cache_creation_tokens", "total_tokens", "cost", "has_subagents")
	schema["prompt_ts"] = map[string]any{"type": "[]uint"}
	schema["tool_counts"] = map[string]any{"type": "string", "filterable": false}
	schema["tool_names"] = map[string]any{"type": "[]string"}
	// Display text can exceed the store's 4096-byte filterable value limit.
	for _, field := range []string{"summary", "first_prompt", "first_prompt_short"} {
		schema[field] = map[string]any{"type": "string", "filterable": false}
	}
	return schema
}

// Row is a chunk on the wire. Factory attribution rides alongside and is
// omitted when absent, which is what a laptop's traces look like.
type Row struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	SessionID  string `json:"session_id,omitempty"`
	TurnUUID   string `json:"turn_uuid,omitempty"`
	ParentUUID string `json:"parent_uuid,omitempty"`
	Seq        int64  `json:"seq"`
	Block      int    `json:"block"`
	Part       int    `json:"part"`
	TS         string `json:"ts,omitempty"`
	Role       string `json:"role,omitempty"`
	BlockType  string `json:"block_type,omitempty"`
	Tier       string `json:"tier,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	Workdir     string `json:"workdir,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Harness     string `json:"harness,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	IsSidechain bool   `json:"is_sidechain"`
	// Host separates one machine's sessions from another's in a shared
	// namespace. A laptop and a headless host writing the same archive is the
	// ordinary case, and without this their traces are indistinguishable.
	Host string `json:"host,omitempty"`

	Instance string `json:"instance,omitempty"`
	Plan     string `json:"plan,omitempty"`
	RFC      string `json:"rfc,omitempty"`
	Issue    string `json:"issue,omitempty"`
	PR       string `json:"pr,omitempty"`
}

// RowOf converts a chunk to a row. Factory attribution is applied separately by
// whoever knows it; the parser never invents it.
func RowOf(c trace.Chunk) Row {
	return Row{
		ID: c.ID, Text: c.Text,
		SessionID: c.SessionID, TurnUUID: c.TurnUUID, ParentUUID: c.ParentUUID,
		Seq: c.Seq, Block: c.Block, Part: c.Part, TS: c.TS, Role: c.Role,
		BlockType: c.BlockType, Tier: c.Tier, ToolName: c.ToolName,
		Workdir: c.Workdir, Branch: c.Branch, Harness: c.Harness,
		SourcePath: c.SourcePath, IsSidechain: c.IsSidechain,
		Host: Hostname(),
	}
}

// WriteResult reports what a batch cost, so a backfill can say what it spent
// rather than leaving it to an invoice at the end of the month.
type WriteResult struct {
	RowsUpserted    int `json:"rows_upserted"`
	EmbeddingTokens int
}

type writeResponse struct {
	Status       string `json:"status"`
	Error        string `json:"error"`
	RowsUpserted int    `json:"rows_upserted"`
	RowsAffected int    `json:"rows_affected"`
	Performance  struct {
		EmbeddingTokens int `json:"embedding_tokens"`
	} `json:"performance"`
}

// Write upserts one batch. The caller batches; this does one round trip so a
// failure names the rows it was carrying.
func (c *Client) Write(rows []Row) (WriteResult, error) {
	if len(rows) == 0 {
		return WriteResult{}, nil
	}
	body := map[string]any{
		"upsert_rows":     rows,
		"distance_metric": "cosine_distance",
		"schema":          c.schema(),
	}
	var out writeResponse
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace, body, &out); err != nil {
		return WriteResult{}, err
	}
	if out.Status != "OK" && out.Error != "" {
		return WriteResult{}, fmt.Errorf("layer write: %s", out.Error)
	}
	return WriteResult{RowsUpserted: out.RowsUpserted, EmbeddingTokens: out.Performance.EmbeddingTokens}, nil
}

// WriteBlocks writes whole, non-embedded blocks beside the chunk namespace.
func (c *Client) WriteBlocks(rows []trace.BlockRow) (WriteResult, error) {
	if !c.Caps.ReadSide() {
		return WriteResult{}, nil
	}
	return c.writeRows(c.Namespace+"-blocks", rows, blockSchema())
}

// WriteSessions writes one aggregate row per parsed session.
func (c *Client) WriteSessions(rows []trace.SessionRow) (WriteResult, error) {
	if !c.Caps.ReadSide() {
		return WriteResult{}, nil
	}
	// A replay of an older transcript must not roll a stable row back. Equal
	// end times remain writable so --force can backfill newly added metadata.
	wire := make([]map[string]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		raw, err := json.Marshal(row)
		if err != nil {
			return WriteResult{}, err
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return WriteResult{}, err
		}
		counts, err := json.Marshal(row.ToolCounts)
		if err != nil {
			return WriteResult{}, err
		}
		obj["tool_counts"], _ = json.Marshal(string(counts))
		wire = append(wire, obj)
	}
	return c.writeRows(c.Namespace+"-sessions", wire, sessionSchema(), []any{"Or", []any{[]any{"end", "Eq", nil}, []any{"end", "Lte", map[string]any{"$ref_new": "end"}}}})
}

// PatchSessionSummaries changes only the summary attribute on existing rows.
// Re-upserting the full row would needlessly resend large filterable scalars
// such as first_prompt, which older rows may contain above Layer's current
// write limit.
func (c *Client) PatchSessionSummaries(rows []trace.SessionRow) (WriteResult, error) {
	if !c.Caps.ReadSide() {
		return WriteResult{}, nil
	}
	patches := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		patches = append(patches, map[string]string{"id": row.ID, "summary": row.Summary})
	}
	body := map[string]any{
		"patch_rows": patches,
		"schema":     scalarSchema("summary"),
	}
	var out writeResponse
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions", body, &out); err != nil {
		return WriteResult{}, err
	}
	if out.Status != "OK" && out.Error != "" {
		return WriteResult{}, fmt.Errorf("layer write: %s", out.Error)
	}
	rowsWritten := out.RowsUpserted
	if out.RowsAffected > rowsWritten {
		rowsWritten = out.RowsAffected
	}
	return WriteResult{RowsUpserted: rowsWritten}, nil
}

func (c *Client) writeRows(namespace string, rows any, schema map[string]any, condition ...any) (WriteResult, error) {
	body := map[string]any{"upsert_rows": rows, "schema": schema}
	if len(condition) > 0 {
		if cond := c.Caps.WriteCondition(condition[0]); cond != nil {
			body["upsert_condition"] = cond
		}
	}
	var out writeResponse
	if err := c.do("POST", "/v2/namespaces/"+namespace, body, &out); err != nil {
		return WriteResult{}, err
	}
	if out.Status != "OK" && out.Error != "" {
		return WriteResult{}, fmt.Errorf("layer write: %s", out.Error)
	}
	return WriteResult{RowsUpserted: out.RowsUpserted, EmbeddingTokens: out.Performance.EmbeddingTokens}, nil
}

// ListSessionRows reads the aggregate namespace. A negative limit scans every
// page; positive limits retain the existing newest-start-first CLI behavior.
// Include all non-vector attributes so pre-backfill namespaces can be read even
// before prompt_ts, tool_names, and total_tokens have been declared.
func (c *Client) ListSessionRows(topK int, filter any) ([]trace.SessionRow, error) {
	return c.listSessionRows(topK, filter, false, false)
}

// ListSlimSessionRows omits full prompts at the store, including before backfill.
func (c *Client) ListSlimSessionRows(topK int, filter any) ([]trace.SessionRow, error) {
	return c.listSessionRows(topK, filter, true, false)
}

// SessionsWatermark returns the sessions namespace's last write time from
// namespace metadata: a cheap check for whether a cached archive is still whole.
func (c *Client) SessionsWatermark() (string, error) {
	var out struct {
		LastWriteAt string `json:"last_write_at"`
	}
	if err := c.do("GET", "/v2/namespaces/"+c.Namespace+"-sessions/metadata", nil, &out); err != nil {
		return "", err
	}
	return out.LastWriteAt, nil
}

// ListSessionIndexRows fetches only identity and archive facets before store filtering.
func (c *Client) ListSessionIndexRows(topK int, filter any) ([]trace.SessionRow, error) {
	return c.listSessionRows(topK, filter, false, true)
}

func (c *Client) listSessionRows(topK int, filter any, slim, index bool) ([]trace.SessionRow, error) {
	// A negative limit scans every page, using the unique row id as cursor.
	all := topK < 0 || topK > 10000
	if topK == 0 {
		topK = 1000
	}
	pageSize := topK
	if all || pageSize > 10000 {
		pageSize = 10000
	}
	rows := []trace.SessionRow{}
	cursor := ""
	for {
		f := filter
		if cursor != "" {
			f = And(filter, []any{"id", "Gt", cursor})
		}
		rank := []any{"id", "asc"}
		if !all && topK <= 10000 {
			rank = []any{"start", "desc"}
		}
		body := map[string]any{"rank_by": rank, "top_k": pageSize, "include_attributes": true}
		if slim {
			delete(body, "include_attributes")
			body["exclude_attributes"] = []string{"first_prompt", "vector"}
		}
		if index {
			body["include_attributes"] = []string{"session_id", "end", "repo_url", "model", "harness", "host", "tool_names"}
		}
		if f != nil {
			body["filters"] = f
		}
		var out struct {
			Rows  []trace.SessionRow `json:"rows"`
			Error string             `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-sessions/query", body, &out); err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("layer query: %s", out.Error)
		}
		rows = append(rows, out.Rows...)
		if len(out.Rows) < pageSize || (!all && len(rows) >= topK) {
			break
		}
		next := out.Rows[len(out.Rows)-1].ID
		if next <= cursor {
			return nil, fmt.Errorf("session pagination did not advance")
		}
		cursor = next
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Start > rows[j].Start })
	if topK > 0 && len(rows) > topK {
		rows = rows[:topK]
	}
	return rows, nil
}

// ListBlockRows reads the lossless read-side rows for one exact session.
func (c *Client) ListBlockRows(sessionID string) ([]trace.BlockRow, error) {
	attrs := []string{"text", "session_id", "turn_uuid", "seq", "role", "block_type", "tool_use_id",
		"agent_id", "parent_agent_id", "start", "end", "ms", "tool_name", "ok", "model", "effort",
		"request_id", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "cost"}
	body := map[string]any{
		"filters": []any{"session_id", "Eq", sessionID}, "rank_by": []any{"seq", "asc"},
		"top_k": 10000, "include_attributes": attrs,
	}
	var out struct {
		Rows  []trace.BlockRow `json:"rows"`
		Error string           `json:"error"`
	}
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-blocks/query", body, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	return out.Rows, nil
}

// Hit is one search result.
type Hit struct {
	TurnUUID  string  `json:"turn_uuid"`
	ID        string  `json:"id"`
	Dist      float64 `json:"$dist"`
	Text      string  `json:"text"`
	SessionID string  `json:"session_id"`
	TS        string  `json:"ts"`
	Role      string  `json:"role"`
	BlockType string  `json:"block_type"`
	ToolName  string  `json:"tool_name"`
	Sidechain bool    `json:"is_sidechain"`
	Harness   string  `json:"harness"`
	Workdir   string  `json:"workdir"`
	Plan      string  `json:"plan"`
	PR        string  `json:"pr"`
}

// Session is the distinct-session view used by hev ls. TurnCount counts
// normalized turns, not chunks (a long block may occupy several rows).
type Session struct {
	ID        string
	Harness   string
	Workdir   string
	FirstTS   string
	TurnCount int
}

// ListSessions groups chunk rows by their session coordinates in the store,
// then folds those groups into one summary per session. Keeping this as an
// aggregation means namespaces indexed before hev ls learned about them work
// immediately; there is no summary-row backfill and no non-content row to
// embed. from and to are optional RFC3339 bounds.
func (c *Client) ListSessions(from, to string) ([]Session, error) {
	var clauses []any
	if from != "" {
		clauses = append(clauses, []any{"ts", "Gte", from})
	}
	if to != "" {
		clauses = append(clauses, []any{"ts", "Lt", to})
	}
	body := map[string]any{
		"aggregate_by": map[string]any{"chunks": []any{"Count"}},
		"group_by":     []string{"session_id", "harness", "workdir", "ts", "seq"},
		"top_k":        10000,
	}
	if len(clauses) == 1 {
		body["filters"] = clauses[0]
	} else if len(clauses) > 1 {
		body["filters"] = []any{"And", clauses}
	}
	var out struct {
		Groups []struct {
			SessionID string `json:"session_id"`
			Harness   string `json:"harness"`
			Workdir   string `json:"workdir"`
			TS        string `json:"ts"`
			Seq       int64  `json:"seq"`
		} `json:"aggregation_groups"`
		Error string `json:"error"`
	}
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	type accumulated struct {
		session Session
		seqs    map[int64]bool
	}
	byID := map[string]*accumulated{}
	for _, g := range out.Groups {
		if g.SessionID == "" {
			continue
		}
		a := byID[g.SessionID]
		if a == nil {
			a = &accumulated{session: Session{ID: g.SessionID, Harness: g.Harness, Workdir: g.Workdir, FirstTS: g.TS}, seqs: map[int64]bool{}}
			byID[g.SessionID] = a
		}
		if a.session.FirstTS == "" || (g.TS != "" && g.TS < a.session.FirstTS) {
			a.session.FirstTS = g.TS
		}
		a.seqs[g.Seq] = true
	}
	ss := make([]Session, 0, len(byID))
	for _, a := range byID {
		a.session.TurnCount = len(a.seqs)
		ss = append(ss, a.session)
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].FirstTS > ss[j].FirstTS })
	return ss, nil
}

// SessionRows fetches chunks by session id without search or query embedding.
// Prefix bounds preserve the long-standing hev trace behavior used by the
// shortened ids from hev ls.
func (c *Client) SessionRows(prefix string) ([]Row, error) {
	attrs := []string{
		"text", "session_id", "turn_uuid", "parent_uuid", "seq", "block", "part",
		"ts", "role", "block_type", "tier", "tool_name", "workdir", "branch",
		"harness", "source_path", "is_sidechain",
	}
	body := map[string]any{
		"filters": []any{"And", []any{
			[]any{"session_id", "Gte", prefix},
			[]any{"session_id", "Lt", prefix + "\uffff"},
		}},
		"rank_by":            []any{[]any{"seq", "asc"}, []any{"part", "asc"}},
		"top_k":              10000,
		"include_attributes": attrs,
	}
	query := func() ([]Row, error) {
		var out struct {
			Rows  []Row  `json:"rows"`
			Error string `json:"error"`
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("layer query: %s", out.Error)
		}
		return out.Rows, nil
	}
	rows, err := query()
	if err != nil && strings.Contains(err.Error(), `attribute \"block\" not found`) {
		// Namespaces indexed before block coordinates were added reject an
		// unknown include attribute. Retry with the original RFC 0003 schema;
		// block type remains a useful compatibility grouping key.
		withoutBlock := append([]string{}, attrs[:5]...)
		body["include_attributes"] = append(withoutBlock, attrs[6:]...)
		return query()
	}
	return rows, err
}

// Search runs hybrid retrieval: a semantic leg that embeds the query in the
// store and a BM25 leg over the same column, fused by reciprocal rank on the
// server. One round trip, no vector on the wire in either direction, and
// nothing to tune on this side. Which of the two hybrid routes carries it is
// the store's answer (Capabilities.SearchRoute), never a choice made here.
//
// A filter, when given, is turbopuffer's filter form and scopes both legs —
// e.g. []any{"plan", "Eq", "scoped-key-leak"}.
func (c *Client) Search(query string, topK int, filter any) ([]Hit, error) {
	return c.search(query, topK, filter, []string{"text", "session_id", "turn_uuid", "ts", "role", "block_type", "harness", "workdir", "plan", "pr"})
}

// SearchHits is Search plus what the dashboard needs to label a hit: the tool
// that produced it and whether a subagent did. Search keeps its pinned wire.
func (c *Client) SearchHits(query string, topK int, filter any) ([]Hit, error) {
	return c.search(query, topK, filter, []string{"text", "session_id", "turn_uuid", "ts", "role", "block_type", "tool_name", "is_sidechain", "harness", "workdir", "plan", "pr"})
}

// MaxPhrasings is how many phrasings SearchPhrasings packs into one request:
// two legs each, inside turbopuffer's 16-subquery multi-query limit.
const MaxPhrasings = 8

// SearchPhrasings fans several phrasings of one question out as legs of a
// single multi-query — an ANN and a BM25 leg per phrasing — and lets the store
// fuse every leg with RRF. One phrasing is SearchHits.
func (c *Client) SearchPhrasings(phrasings []string, topK int, filter any) ([]Hit, error) {
	if len(phrasings) == 1 {
		return c.SearchHits(phrasings[0], topK, filter)
	}
	if len(phrasings) == 0 || len(phrasings) > MaxPhrasings {
		return nil, fmt.Errorf("1–%d phrasings per query, got %d", MaxPhrasings, len(phrasings))
	}
	route, err := c.Caps.SearchRoute()
	if err != nil {
		return nil, err
	}
	if route != RouteMultiQuery {
		return nil, fmt.Errorf("several phrasings need a store that fuses multi-query legs; %s serves one HybridText phrasing per query", c.Caps.Store.Kind)
	}
	if topK <= 0 {
		topK = 10
	}
	attrs := []string{"text", "session_id", "turn_uuid", "ts", "role", "block_type", "tool_name", "is_sidechain", "harness", "workdir", "plan", "pr"}
	body := c.multiQueryBody(phrasings[0], topK, filter, attrs)
	for _, p := range phrasings[1:] {
		more := c.multiQueryBody(p, topK, filter, attrs)["queries"].([]map[string]any)
		body["queries"] = append(body["queries"].([]map[string]any), more...)
	}
	var out struct {
		Results []struct {
			Rows []Hit `json:"rows"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	if len(out.Results) == 0 {
		return nil, nil
	}
	return out.Results[0].Rows, nil
}

func (c *Client) search(query string, topK int, filter any, attrs []string) ([]Hit, error) {
	if topK <= 0 {
		topK = 10
	}
	route, err := c.Caps.SearchRoute()
	if err != nil {
		return nil, err
	}
	if os.Getenv("HEV_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "hev: search %s route=%s store=%s\n", c.Namespace, route, c.Caps.Store.Kind)
	}
	var out struct {
		Rows    []Hit `json:"rows"`
		Results []struct {
			Rows []Hit `json:"rows"`
		} `json:"results"`
		Error string `json:"error"`
	}
	body := c.multiQueryBody(query, topK, filter, attrs)
	if route == RouteHybridText {
		body = c.hybridTextBody(query, topK, filter, attrs)
	}
	if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"/query", body, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	if route == RouteHybridText {
		return out.Rows, nil
	}
	if len(out.Results) == 0 {
		return nil, nil
	}
	// With rerank_by the server returns one fused set; it still arrives under
	// the multi-query envelope.
	return out.Results[0].Rows, nil
}

// multiQueryBody is the native passthrough: two legs, fused by the store.
func (c *Client) multiQueryBody(query string, topK int, filter any, attrs []string) map[string]any {
	leg := func(rankBy any) map[string]any {
		q := map[string]any{
			"rank_by":            rankBy,
			"top_k":              topK,
			"include_attributes": attrs,
		}
		if filter != nil {
			q["filters"] = filter
		}
		return q
	}
	return map[string]any{
		"queries": []map[string]any{
			leg([]any{"text", "ANN", []any{"Embed", query}}),
			leg([]any{"text", "BM25", query}),
		},
		"rerank_by": []any{"RRF"},
	}
}

// hybridTextBody is one HybridText expression; the gateway issues the legs and
// fuses them, and adds a dense leg of its own when the store can serve one.
// No cursor and no temporal_filter, on any store: kit pages nothing here, and
// its date bounds are scalar filters.
func (c *Client) hybridTextBody(query string, topK int, filter any, attrs []string) map[string]any {
	rank := []any{"text", "HybridText", query}
	if opts := c.Caps.HybridTextOptions(); opts != nil {
		rank = append(rank, opts)
	}
	body := map[string]any{
		"rank_by":            rank,
		"top_k":              topK,
		"include_attributes": attrs,
	}
	if filter != nil {
		body["filters"] = filter
	}
	return body
}

// Health is the gateway's liveness answer. Version is what the running image
// reports, which `hev up` compares against a release pin.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health reads GET /health. It is a Layer gateway route; a bare Turbopuffer
// endpoint does not serve it.
func (c *Client) Health() (Health, error) {
	var h Health
	req, err := http.NewRequest("GET", c.Endpoint+"/health", nil)
	if err != nil {
		return h, err
	}
	probe := &http.Client{Timeout: 5 * time.Second}
	resp, err := probe.Do(req)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("GET /health: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return h, err
	}
	if h.Status != "ok" {
		return h, fmt.Errorf("gateway status %q", h.Status)
	}
	return h, nil
}

func (c *Client) do(method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.Endpoint+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	defer func() { c.timing.record(time.Since(started), out) }()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		// The store's own error text is the most useful thing here; a status
		// code alone has sent people looking in the wrong place before.
		return &HTTPError{Status: resp.StatusCode, Message: fmt.Sprintf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// hostname is resolved once: it is the same for the life of the process and is
// stamped on every row.
var (
	hostOnce sync.Once
	hostName string
)

func Hostname() string {
	hostOnce.Do(func() {
		if v := os.Getenv("HEV_HOST"); v != "" {
			hostName = v
			return
		}
		h, err := os.Hostname()
		if err != nil {
			return
		}
		// Short name, lowercased — the same normalization the factory uses for
		// home_host, so the two agree about what a machine is called.
		if i := strings.Index(h, "."); i > 0 {
			h = h[:i]
		}
		hostName = strings.ToLower(h)
	})
	return hostName
}

// And composes optional store predicates.
func And(filters ...any) any {
	clauses := []any{}
	for _, f := range filters {
		if f != nil {
			clauses = append(clauses, f)
		}
	}
	if len(clauses) == 0 {
		return nil
	}
	if len(clauses) == 1 {
		return clauses[0]
	}
	return []any{"And", clauses}
}

type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }
func isNamespaceMissing(err error) bool {
	var e *HTTPError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}
