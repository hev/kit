package collect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/grep"
	"github.com/hev/kit/pkg/search"
)

// FileTurns keeps one app namespace's turn documents in a file archive
// namespace (LayerNamespace's name, under the archive root), for a host with
// no Layer API key. It is the file archive `hev traces` uses with no key:
// the same directory layout, the same .lock around every read and write, and
// the same grep search (every word of the query, case-insensitive, ranked by
// words matched then newest first). Nothing is embedded, so search matches
// words, not meaning.
//
// Each owner's turns (within a tenant) share one chunk file whose name is a
// hash of namespace, tenant and owner, so a search or a conversation fetch
// reads only the asking owner's file; the owner, tenant and namespace
// filters then run on every row as well.
type FileTurns struct {
	Store *filearchive.Store
	// Namespace is the app namespace this index holds. Every row carries it
	// and every read filters on it.
	Namespace string
}

// ownerFile names the chunk file that holds one owner's turns in a tenant:
// a plain archive key, so it is the file name as is.
func (f *FileTurns) ownerFile(tenant, owner string) string {
	sum := sha256.Sum256([]byte(f.Namespace + "\x00" + tenant + "\x00" + owner))
	return "app-" + hex.EncodeToString(sum[:16])
}

func (f *FileTurns) Upsert(ctx context.Context, turns []Turn) error {
	byFile := map[string][]map[string]any{}
	var order []string
	for _, t := range turns {
		if t.Namespace != f.Namespace {
			return fmt.Errorf("turn of namespace %q offered to the %q index", t.Namespace, f.Namespace)
		}
		tools, _ := json.Marshal(t.Tools)
		file := f.ownerFile(t.Tenant, t.Owner)
		if byFile[file] == nil {
			order = append(order, file)
		}
		byFile[file] = append(byFile[file], map[string]any{
			"id": t.DocID(), "text": turnText(t, 0),
			"namespace": t.Namespace, "owner": t.Owner, "tenant": t.Tenant, "chat_id": t.ChatID, "turn_id": t.TurnID,
			"ts": t.TS.UTC().Format(tsLayout), "prompt": t.Prompt, "answer": t.Answer, "tools": string(tools),
			"service": t.Service, "harness": "app:" + t.Service,
		})
	}
	for _, file := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := f.Store.WriteDocs(file, byFile[file]); err != nil {
			return err
		}
	}
	return nil
}

func (f *FileTurns) scope(owner, tenant string, more ...any) any {
	return search.And(append([]any{
		[]any{"session_id", "Eq", f.ownerFile(tenant, owner)},
		[]any{"namespace", "Eq", f.Namespace}, []any{"owner", "Eq", owner}, []any{"tenant", "Eq", tenant},
	}, more...)...)
}

// rows reads one owner's turn rows, keeping only those the scope admits.
func (f *FileTurns) rows(owner, tenant string, keep func(Turn) bool) (map[string]Turn, error) {
	docs, err := f.Store.Docs(f.ownerFile(tenant, owner))
	if err != nil {
		return nil, err
	}
	out := make(map[string]Turn, len(docs))
	for _, d := range docs {
		t := turnOf(d)
		if t.Namespace != f.Namespace || t.Owner != owner || t.Tenant != tenant || (keep != nil && !keep(t)) {
			continue
		}
		id, _ := d["id"].(string)
		out[id] = t
	}
	return out, nil
}

func (f *FileTurns) Search(ctx context.Context, owner, tenant, query string, since time.Time, limit int) ([]TurnHit, error) {
	q, err := grep.Compile([]string{query})
	if err != nil {
		return []TurnHit{}, nil // nothing searchable: punctuation only
	}
	var more []any
	if !since.IsZero() {
		more = append(more, []any{"ts", "Gte", since.UTC().Format(tsLayout)})
	}
	hits, err := f.Store.SearchQuery(q, limit, f.scope(owner, tenant, more...))
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return []TurnHit{}, nil
	}
	// Search returns the chunk attributes; the turn's own (prompt, answer,
	// tools, chat) are read from the same owner's file.
	turns, err := f.rows(owner, tenant, nil)
	if err != nil {
		return nil, err
	}
	out := make([]TurnHit, 0, len(hits))
	for _, h := range hits {
		t, ok := turns[h.ID]
		if !ok || (!since.IsZero() && t.TS.Before(since)) {
			continue
		}
		out = append(out, TurnHit{Turn: t, Score: h.Dist})
	}
	return out, nil
}

func (f *FileTurns) Conversation(ctx context.Context, owner, tenant, chatID string) ([]Turn, error) {
	turns, err := f.rows(owner, tenant, func(t Turn) bool { return t.ChatID == chatID })
	if err != nil {
		return nil, err
	}
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].TS.Equal(out[j].TS) {
			return out[i].TS.Before(out[j].TS)
		}
		return out[i].TurnID < out[j].TurnID
	})
	return out, nil
}
