package orders

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/preval"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/steer"
)

const maxLog = 4 << 20

func execute(ctx context.Context, route Route, req Request) (Reply, error) {
	if route.SSH == "" {
		return ExecuteLocal(ctx, req)
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return Reply{}, ErrInvalid
	}
	out, err := dispatch.RPC(ctx, dispatch.Host{SSH: route.SSH}, dispatch.Request{APILocalOnly: true, Action: "orders-api", Note: string(payload)})
	if err != nil {
		return Reply{}, ErrUnavailable
	}
	var wire RPCReply
	if err := json.Unmarshal([]byte(out.Log), &wire); err != nil {
		return Reply{}, ErrUnavailable
	}
	return wire.Reply, wire.Err()
}

// RPCReply is the bounded adapter's wire response. Local error details stay
// on the execution host.
type RPCReply struct {
	Reply Reply `json:"reply"`
	Code  int   `json:"code,omitempty"`
}

var rpcErrors = [...]error{nil, ErrInvalid, ErrForbidden, ErrNotFound, ErrConflict, ErrRejected, ErrUnavailable, ErrPrecondition}

func (r RPCReply) Err() error {
	if r.Code < 0 || r.Code >= len(rpcErrors) {
		return ErrUnavailable
	}
	return rpcErrors[r.Code]
}

func HandleRPC(ctx context.Context, raw string) RPCReply {
	var req Request
	if len(raw) > 4<<20 || json.Unmarshal([]byte(raw), &req) != nil {
		return RPCReply{Code: 1}
	}
	reply, err := ExecuteLocal(ctx, req)
	for code, candidate := range rpcErrors {
		if errors.Is(err, candidate) {
			return RPCReply{Reply: reply, Code: code}
		}
	}
	return RPCReply{Code: 6}
}

// ExecuteLocal operates inside a dedicated, operator-provisioned execution
// account. Its caller already authenticated and authorized the team route.
func ExecuteLocal(ctx context.Context, req Request) (Reply, error) {
	if ctx.Err() != nil {
		return Reply{}, ErrUnavailable
	}
	if !dispatch.ValidLabel(req.Host) || len(req.Order) > 2<<20 || len(req.Note) > 1<<20 {
		return Reply{}, ErrInvalid
	}
	if req.Action != "list" && !dispatch.ValidAddress(req.ID) {
		return Reply{}, ErrInvalid
	}
	selected, err := dispatch.APIWorkspace(ctx)
	if err != nil || !selected.AllowsHost(req.Host) {
		return Reply{}, ErrForbidden
	}
	q := dispatch.Request{WorkspaceName: selected.Name, APILocalOnly: true, Action: req.Action, Name: req.ID, Order: req.Order, Note: req.Note,
		Resolve: req.Resolve, Level: req.Level, HaltTimeout: req.HaltTimeout, Offset: req.Offset,
		Caller: &dispatch.Caller{Sender: req.Actor}}
	switch req.Action {
	case "list":
		rows, err := list(q.WorkspaceName)
		return Reply{Rows: rows}, err
	case "show":
		return get(q.Name, q.WorkspaceName)
	case "set-evaluation":
		current, err := get(q.Name, q.WorkspaceName)
		if err != nil {
			return Reply{}, err
		}
		if current.Active || req.IfMatch == "" || len(req.Evaluation) > 32<<10 {
			return Reply{}, ErrConflict
		}
		var evaluation preval.Evaluation
		if json.Unmarshal(req.Evaluation, &evaluation) != nil || !evaluation.Valid() || evaluation.Provenance.OrderID != q.Name {
			return Reply{}, ErrRejected
		}
		if err := dispatch.SetEvaluationConditional(evaluation, req.IfMatch); err != nil {
			if errors.Is(err, preval.ErrConflict) {
				return Reply{}, ErrPrecondition
			}
			return Reply{}, ErrUnavailable
		}
		return get(q.Name, q.WorkspaceName)
	case "logs":
		return logs(q, req)
	case "start", "amend":
		o, err := dispatch.Parse(req.Order)
		if err != nil || o.Name != req.ID {
			return Reply{}, ErrRejected
		}
		if o.Host != "" && o.Host != req.Host {
			return Reply{}, ErrForbidden
		}
		if err := dispatch.CheckAPIOrder(o, req.Host); err != nil {
			return Reply{}, ErrForbidden
		}
		o.Host = req.Host
		q.Meta = dispatch.Meta{Order: o, WorkspaceName: selected.Name, Sender: req.Actor, RequestedBy: req.Actor}
		if req.Trace != "" {
			trace, parent, err := dispatch.ParseTraceRef(req.Trace)
			if err != nil {
				return Reply{}, ErrInvalid
			}
			q.Meta.TraceID, q.Meta.ParentSpanID = trace, parent
		}
	case "say":
		if req.Resolve == "" || strings.TrimSpace(req.Note) == "" {
			return Reply{}, ErrInvalid
		}
	case "rework", "retry", "stop", "clean":
	case "steer":
		if !steer.ValidLevel(req.Level) || strings.TrimSpace(req.Note) == "" {
			return Reply{}, ErrInvalid
		}
		if req.HaltTimeout != "" {
			d, err := time.ParseDuration(req.HaltTimeout)
			if err != nil || d <= 0 {
				return Reply{}, ErrInvalid
			}
		}
	default:
		return Reply{}, ErrInvalid
	}
	if q.Action != "start" {
		current, err := get(q.Name, q.WorkspaceName)
		if err != nil {
			return Reply{}, err
		}
		if q.Action == "amend" && current.Order["repo"] != q.Meta.Repo {
			return Reply{}, ErrConflict
		}
		if q.Action == "clean" && current.Active {
			return Reply{}, ErrConflict
		}
	}
	out, err := dispatch.Handle(q)
	if err != nil {
		if q.Action == "start" && strings.Contains(err.Error(), "already exists") {
			return Reply{}, ErrConflict
		}
		if q.Action == "start" || q.Action == "amend" {
			return Reply{}, ErrRejected
		}
		return Reply{}, ErrConflict
	}
	if req.Action == "steer" {
		store := steer.RunStore(dispatch.Root(), req.ID)
		if name, ok := strings.CutPrefix(req.ID, "o-"); ok {
			store = steer.OrchestratorStore(dispatch.Root(), name)
		}
		receipt, err := store.Get(out.SteerID)
		if err != nil {
			return Reply{}, ErrUnavailable
		}
		return Reply{Receipt: object(receipt)}, nil
	}
	if req.Action == "clean" || req.Action == "stop" && strings.HasPrefix(out.Message, "cancelled queued ") {
		return Reply{}, nil
	}
	reply, err := get(q.Name, q.WorkspaceName)
	if err == nil && req.Action == "stop" {
		if reply.Active {
			reply.Order["state"] = "halting"
		} else {
			reply.Order["state"] = "halted"
		}
	}
	return reply, err
}

func list(workspaceName string) ([]map[string]any, error) {
	out, err := dispatch.Handle(dispatch.Request{APILocalOnly: true, WorkspaceName: workspaceName, Action: "list"})
	if err != nil {
		return nil, ErrUnavailable
	}
	rows := make([]map[string]any, 0, len(out.Rows))
	for _, row := range out.Rows {
		if row.Forward == nil && row.Parent == "" && row.Repo != "" && row.State != "closed" {
			rows = append(rows, summary(row))
		}
	}
	return rows, nil
}

func get(id, workspaceName string) (Reply, error) {
	rows, err := list(workspaceName)
	if err != nil {
		return Reply{}, err
	}
	for _, row := range rows {
		if row["id"] != id {
			continue
		}
		out, err := dispatch.Handle(dispatch.Request{APILocalOnly: true, WorkspaceName: workspaceName, Action: "show", Name: id})
		if err != nil {
			return Reply{}, ErrNotFound
		}
		for file, field := range map[string]string{"order.md": "order", "status.md": "status", "report.md": "report", "inbox.md": "inbox"} {
			if text := out.Files[file]; text != "" {
				row[field] = scrub(text)
			}
		}
		for file, field := range map[string]string{"decisions.json": "decisions", "result.json": "result", "review-notes.json": "review_notes"} {
			if text := out.Files[file]; text != "" {
				var value any
				if json.Unmarshal([]byte(scrub(text)), &value) == nil && value != nil {
					row[field] = value
				}
			}
		}
		reply := Reply{Order: row, Active: out.Active}
		if result, ok := row["result"].(map[string]any); ok {
			if attempt, ok := result["span_id"].(string); ok && attempt != "" {
				var evaluation *preval.Evaluation
				if raw, err := json.Marshal(result["evaluation"]); err == nil && json.Unmarshal(raw, &evaluation) == nil {
					reply.EvaluationETag = dispatch.OrderEvaluationRevision(id, attempt, evaluation)
					if evaluation != nil {
						row["evaluation"] = evaluation
					}
				}
			}
		}
		return reply, nil
	}
	return Reply{}, ErrNotFound
}

func object(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func summary(row dispatch.Row) map[string]any {
	value := object(row)
	out := map[string]any{"id": row.Name}
	for _, key := range strings.Fields("name host repo base harness model validation preview reply issue class timeout every until max_lifetime context skills kind state position orchestrator requested_by branch head_sha outcome start trace_id steer") {
		if field, ok := value[key]; ok {
			out[key] = field
		}
	}
	if row.Start.IsZero() {
		delete(out, "start")
	}
	if row.PR != "" {
		out["pr_url"] = row.PR
	}
	if row.Trace != "" {
		out["session_id"] = row.Trace
	}
	if row.Outcome == "done" || row.Outcome == "blocked" || row.Outcome == "needs-decision" {
		outcome := map[string]any{"kind": row.Outcome}
		for k, v := range map[string]string{"key": row.Key, "question": row.Question, "reason": row.Reason} {
			if v != "" {
				outcome[k] = scrub(v)
			}
		}
		out["outcome"] = outcome
	} else {
		delete(out, "outcome")
	}
	if counts, ok := out["steer"].(map[string]any); ok {
		for _, key := range []string{"halting", "halted"} {
			if counts[key] == true {
				counts[key] = 1
			} else {
				delete(counts, key)
			}
		}
	}
	for _, key := range []string{"repo", "preview", "context", "reply"} {
		if text, ok := out[key].(string); ok {
			out[key] = scrub(text)
		}
	}
	if out["host"] == "" {
		delete(out, "host")
	}
	if row.Attention != "" {
		out["attention_status"] = row.Attention
	}
	return out
}

var scrubber = sync.OnceValue(func() *redact.Scrubber {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil
	}
	s, _ := redact.New(salt)
	return s
})

func scrub(s string) string {
	r := scrubber()
	if r == nil {
		return "[REDACTED]"
	}
	out, _ := r.Text(s)
	for _, path := range []string{dispatch.Root(), os.Getenv("HOME")} {
		if path != "" && path != "." && path != string(filepath.Separator) {
			out = strings.ReplaceAll(out, path, "[REDACTED]")
		}
	}
	return redact.PublicTokens(out)
}

func logs(q dispatch.Request, req Request) (Reply, error) {
	if req.Limit < 1 || req.Limit > maxLog || req.Offset < 0 || req.Round < 0 {
		return Reply{}, ErrInvalid
	}
	current, err := get(req.ID, q.WorkspaceName)
	if err != nil {
		return Reply{}, err
	}
	out, err := dispatch.Handle(dispatch.Request{APILocalOnly: true, WorkspaceName: q.WorkspaceName, Action: q.Action, Name: q.Name, Offset: req.Offset, APILogLimit: req.Limit, Round: req.Round})
	if err != nil {
		return Reply{}, ErrNotFound
	}
	return Reply{Log: out.Log, Offset: out.Offset, Active: out.Active || current.Active}, nil
}
