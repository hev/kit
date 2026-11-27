package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/pkg/search"
)

// TurnHit is one search answer.
type TurnHit struct {
	Turn  Turn
	Score float64
}

// TurnIndex stores one app namespace's turn documents and answers the app's
// two trace reads. The server holds one per allowed namespace. An
// implementation scopes every read to owner and tenant; the server checks
// namespace, owner and tenant again on what comes back.
type TurnIndex interface {
	Upsert(ctx context.Context, turns []Turn) error
	Search(ctx context.Context, owner, tenant, query string, since time.Time, limit int) ([]TurnHit, error)
	Conversation(ctx context.Context, owner, tenant, chatID string) ([]Turn, error)
}

// tsLayout is fixed width, so string order is time order and `since` is a
// plain Gte filter on every store.
const tsLayout = "2006-01-02T15:04:05.000000Z"

// LayerTurns keeps one app namespace's turn documents in its own Layer
// namespace (LayerNamespace) through kit's Layer client: the text column
// (prompt and answer) is embedded and full-text indexed the way the trace
// archive's is on the same store, and namespace, owner, tenant, chat_id and ts
// are filterable.
type LayerTurns struct {
	Client *layer.Client
	// Namespace is the app namespace this index holds. Every row carries it
	// and every read filters on it.
	Namespace string
}

var turnAttrs = []string{"namespace", "owner", "tenant", "chat_id", "turn_id", "ts", "prompt", "answer", "tools", "service"}

func (l *LayerTurns) schema() map[string]any {
	str := func(filterable bool) map[string]any {
		return map[string]any{"type": "string", "filterable": filterable}
	}
	return map[string]any{
		"text":      l.Client.TextField(),
		"namespace": str(true), "owner": str(true), "tenant": str(true), "chat_id": str(true), "turn_id": str(true), "ts": str(true),
		// Display text can exceed a store's filterable value limit.
		"prompt": str(false), "answer": str(false), "tools": str(false), "service": str(false), "harness": str(false),
	}
}

// turnText is what search ranks: the user's words and the agent's answer.
func turnText(t Turn, maxRunes int) string {
	text := t.Prompt
	if t.Answer != "" {
		text += "\n\n" + t.Answer
	}
	if r := []rune(text); maxRunes > 0 && len(r) > maxRunes {
		text = string(r[:maxRunes])
	}
	return text
}

func (l *LayerTurns) Upsert(ctx context.Context, turns []Turn) error {
	if len(turns) == 0 {
		return nil
	}
	// The embedded column takes what the store's model embeds whole
	// (layer.Client.ChunkRunes); a longer turn is ranked by its head and
	// still stores prompt and answer in full.
	maxRunes := l.Client.ChunkRunes()
	rows := make([]map[string]any, 0, len(turns))
	for _, t := range turns {
		if t.Namespace != l.Namespace {
			return fmt.Errorf("turn of namespace %q offered to the %q index", t.Namespace, l.Namespace)
		}
		tools, _ := json.Marshal(t.Tools)
		rows = append(rows, map[string]any{
			"id": t.DocID(), "text": turnText(t, maxRunes),
			"namespace": t.Namespace, "owner": t.Owner, "tenant": t.Tenant, "chat_id": t.ChatID, "turn_id": t.TurnID,
			"ts": t.TS.UTC().Format(tsLayout), "prompt": t.Prompt, "answer": t.Answer, "tools": string(tools),
			"service": t.Service, "harness": "app:" + t.Service,
		})
	}
	client := l.Client.WithContext(ctx)
	schema := l.schema()
	for start := 0; start < len(rows); start += layer.MaxEmbeddedWriteRows {
		end := min(start+layer.MaxEmbeddedWriteRows, len(rows))
		var out struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		body := map[string]any{"upsert_rows": rows[start:end], "distance_metric": "cosine_distance", "schema": schema}
		if err := client.Request("POST", "/v2/namespaces/"+client.Namespace, body, &out); err != nil {
			return err
		}
		if out.Status != "OK" && out.Error != "" {
			return fmt.Errorf("layer write: %s", out.Error)
		}
	}
	return nil
}

func (l *LayerTurns) scope(owner, tenant string, more ...any) any {
	return search.And(append([]any{[]any{"namespace", "Eq", l.Namespace}, []any{"owner", "Eq", owner}, []any{"tenant", "Eq", tenant}}, more...)...)
}

func (l *LayerTurns) Search(ctx context.Context, owner, tenant, query string, since time.Time, limit int) ([]TurnHit, error) {
	var more []any
	if !since.IsZero() {
		more = append(more, []any{"ts", "Gte", since.UTC().Format(tsLayout)})
	}
	rows, err := l.Client.WithContext(ctx).SearchRows(query, limit, l.scope(owner, tenant, more...), turnAttrs)
	if err != nil {
		if namespaceNotCreated(err) {
			return nil, nil
		}
		return nil, err
	}
	hits := make([]TurnHit, 0, len(rows))
	for i, r := range rows {
		hits = append(hits, TurnHit{Turn: turnOf(r), Score: score(r, i)})
	}
	return hits, nil
}

// noEmbedModel is the gateway's 422 text when a query needs text embedded
// and the namespace names no model. A namespace that doesn't exist yet names
// none: the first Upsert declares the embedded text column with the store's
// model.
const noEmbedModel = "a model name must be provided when Layer resolves Embed"

// namespaceNotCreated reports whether a search failed only because the
// collector's Layer namespace hasn't been written yet: a 404, or the 422 a
// gateway answers before it can embed the query. Any other 422 is a real
// error.
func namespaceNotCreated(err error) bool {
	var he *layer.HTTPError
	if !errors.As(err, &he) {
		return false
	}
	switch he.Status {
	case http.StatusNotFound:
		return true
	case http.StatusUnprocessableEntity:
		return strings.Contains(string(he.Body), noEmbedModel)
	}
	return false
}

func (l *LayerTurns) Conversation(ctx context.Context, owner, tenant, chatID string) ([]Turn, error) {
	body := map[string]any{
		"filters":            l.scope(owner, tenant, []any{"chat_id", "Eq", chatID}),
		"rank_by":            []any{"ts", "asc"},
		"top_k":              1000,
		"include_attributes": turnAttrs,
	}
	var out struct {
		Rows  []map[string]any `json:"rows"`
		Error string           `json:"error"`
	}
	if err := l.Client.WithContext(ctx).Request("POST", "/v2/namespaces/"+l.Client.Namespace+"/query", body, &out); err != nil {
		var he *layer.HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return nil, nil
		}
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("layer query: %s", out.Error)
	}
	turns := make([]Turn, 0, len(out.Rows))
	for _, r := range out.Rows {
		turns = append(turns, turnOf(r))
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].TS.Before(turns[j].TS) })
	return turns, nil
}

func turnOf(r map[string]any) Turn {
	s := func(k string) string { v, _ := r[k].(string); return v }
	t := Turn{Namespace: s("namespace"), Owner: s("owner"), Tenant: s("tenant"), ChatID: s("chat_id"), TurnID: s("turn_id"), Prompt: s("prompt"), Answer: s("answer"), Service: s("service"), Tools: []Tool{}}
	t.TS, _ = time.Parse(time.RFC3339Nano, s("ts"))
	_ = json.Unmarshal([]byte(s("tools")), &t.Tools)
	if t.Tools == nil {
		t.Tools = []Tool{}
	}
	return t
}

// score reads a fused hit's relevance, higher is better: "$score" where the
// store reports one, else 1 - "$dist", else reciprocal rank.
func score(r map[string]any, rank int) float64 {
	if v, ok := r["$score"].(float64); ok {
		return v
	}
	if v, ok := r["$dist"].(float64); ok {
		return 1 - v
	}
	return 1 / float64(rank+1)
}
