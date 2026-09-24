package layer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/trace"
)

var markName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

type EvalRow struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	SessionID string `json:"session_id"`
	TS        string `json:"ts"`
	Role      string `json:"role"`
	Instance  string `json:"instance"`
	Host      string `json:"host"`
	Poor      bool   `json:"poor"`
	Summary   string `json:"summary"`
	Marks     string `json:"marks"`
	Evidence  string `json:"evidence"`
	Findings  string `json:"findings"`
}

func (r EvalRow) Eval() (*trace.Eval, error) {
	e := &trace.Eval{Session: r.SessionID, TS: r.TS, Role: r.Role, Instance: r.Instance, Host: r.Host, Poor: r.Poor, Summary: r.Summary}
	for _, v := range []struct {
		raw string
		dst any
	}{{r.Marks, &e.Marks}, {r.Evidence, &e.Evidence}, {r.Findings, &e.Findings}} {
		if err := json.Unmarshal([]byte(v.raw), v.dst); err != nil {
			return nil, fmt.Errorf("eval %s: %w", r.ID, err)
		}
	}
	return e, nil
}
func EvalID(session, ts string) string {
	sum := sha256.Sum256([]byte(session + "\x00" + ts))
	return hex.EncodeToString(sum[:])[:32]
}

func (c *Client) WriteEvals(evals []trace.Eval) (WriteResult, error) {
	if len(evals) == 0 {
		return WriteResult{}, nil
	}
	schema := scalarSchema("session_id", "ts", "role", "instance", "host")
	schema["poor"] = map[string]any{"type": "bool"}
	schema["text"] = c.textField()
	schema["text"].(map[string]any)["filterable"] = false
	for _, k := range []string{"summary", "marks", "evidence", "findings"} {
		schema[k] = map[string]any{"type": "string", "filterable": false}
	}
	rows := []map[string]any{}
	for _, e := range evals {
		if strings.TrimSpace(e.Session) == "" {
			return WriteResult{}, fmt.Errorf("eval session is required")
		}
		ts, err := time.Parse(time.RFC3339Nano, e.TS)
		if err != nil {
			return WriteResult{}, fmt.Errorf("eval ts must be RFC3339: %w", err)
		}
		e.TS = ts.UTC().Format(time.RFC3339Nano)
		if e.Marks == nil {
			return WriteResult{}, fmt.Errorf("eval marks must be an object")
		}
		if e.Evidence == nil {
			e.Evidence = map[string]string{}
		}
		if e.Findings == nil {
			e.Findings = []string{}
		}
		row := map[string]any{"id": EvalID(e.Session, e.TS), "session_id": e.Session, "ts": e.TS, "role": e.Role, "instance": e.Instance, "host": e.Host, "poor": e.Poor, "summary": e.Summary}
		for _, v := range []struct {
			k    string
			data any
		}{{"marks", e.Marks}, {"evidence", e.Evidence}, {"findings", e.Findings}} {
			raw, _ := json.Marshal(v.data)
			row[v.k] = string(raw)
		}
		names := []string{}
		for name, value := range e.Marks {
			if !markName.MatchString(name) {
				return WriteResult{}, fmt.Errorf("invalid mark name %q (letters, digits, underscores; starts with a letter)", name)
			}
			row["mark_"+name] = value
			schema["mark_"+name] = map[string]any{"type": "int"}
		}
		for name := range e.Evidence {
			names = append(names, name)
		}
		sort.Strings(names)
		text := []string{e.Summary}
		for _, name := range names {
			text = append(text, name+": "+e.Evidence[name])
		}
		text = append(text, e.Findings...)
		row["text"] = strings.Join(text, "\n")
		rows = append(rows, row)
	}
	result := WriteResult{}
	for start := 0; start < len(rows); start += 30 {
		var out writeResponse
		body := map[string]any{"upsert_rows": rows[start:min(start+30, len(rows))], "schema": schema, "distance_metric": "cosine_distance"}
		if cond := c.Caps.WriteCondition([]any{"id", "Eq", nil}); cond != nil {
			body["upsert_condition"] = cond
		}
		if err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-evals", body, &out); err != nil {
			return WriteResult{}, err
		}
		if out.Error != "" {
			return WriteResult{}, fmt.Errorf("layer eval write: %s", out.Error)
		}
		result.RowsUpserted += out.RowsUpserted
		result.EmbeddingTokens += out.Performance.EmbeddingTokens
	}
	return result, nil
}

func (c *Client) ListEvalRows(filter any) ([]EvalRow, error) {
	return c.listEvalRows(filter, false)
}

// ListEvalMarks omits evaluator prose from archive list and facet reads.
func (c *Client) ListEvalMarks(filter any) ([]EvalRow, error) {
	return c.listEvalRows(filter, true)
}

func (c *Client) listEvalRows(filter any, marksOnly bool) ([]EvalRow, error) {
	rows := []EvalRow{}
	cursor := ""
	for {
		f := filter
		if cursor != "" {
			f = And(f, []any{"id", "Gt", cursor})
		}
		body := map[string]any{"rank_by": []any{"id", "asc"}, "top_k": 10000, "include_attributes": []string{"text", "session_id", "ts", "role", "instance", "host", "poor", "summary", "marks", "evidence", "findings"}}
		if marksOnly {
			body["include_attributes"] = []string{"session_id", "ts", "role", "instance", "host", "poor", "marks"}
		}
		if f != nil {
			body["filters"] = f
		}
		var out struct {
			Rows  []EvalRow `json:"rows"`
			Error string    `json:"error"`
		}
		err := c.do("POST", "/v2/namespaces/"+c.Namespace+"-evals/query", body, &out)
		if isNamespaceMissing(err) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		if out.Error != "" {
			return nil, fmt.Errorf("layer eval query: %s", out.Error)
		}
		if marksOnly {
			for i := range out.Rows {
				out.Rows[i].Evidence = "{}"
				out.Rows[i].Findings = "[]"
			}
		}
		rows = append(rows, out.Rows...)
		if len(out.Rows) < 10000 {
			return rows, nil
		}
		next := out.Rows[len(out.Rows)-1].ID
		if next <= cursor {
			return nil, fmt.Errorf("eval pagination did not advance")
		}
		cursor = next
	}
}
func (c *Client) SearchEvals(q string, top int, filter any) ([]Hit, error) {
	evalClient := *c
	evalClient.Namespace += "-evals"
	hits, err := evalClient.search(q, top, filter, []string{"text", "session_id", "ts", "role"})
	if isNamespaceMissing(err) {
		return nil, nil
	}
	return hits, err
}
