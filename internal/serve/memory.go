package serve

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// MemoryReader serves rows held in memory with Layer's filter grammar. It
// backs local previews of a fixture; production reads go to Layer.
type MemoryReader struct {
	Sessions []trace.SessionRow
	Blocks   []trace.BlockRow
	// Spans are published dispatch trace facts; nil reads as an archive
	// with no spans namespace.
	Spans []trace.SpanRow
}

func (m *MemoryReader) ListTraceSpanRows(filter any) ([]trace.SpanRow, error) {
	var out []trace.SpanRow
	for _, row := range m.Spans {
		b, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		var attrs map[string]any
		if err := json.Unmarshal(b, &attrs); err != nil {
			return nil, err
		}
		ok, err := memoryMatch(attrs, filter)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MemoryReader) ListSessionRows(topK int, filter any) ([]trace.SessionRow, error) {
	var out []trace.SessionRow
	for _, row := range m.Sessions {
		b, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		var attrs map[string]any
		if err := json.Unmarshal(b, &attrs); err != nil {
			return nil, err
		}
		ok, err := memoryMatch(attrs, filter)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start > out[j].Start })
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (m *MemoryReader) ListBlockRows(sessionID string) ([]trace.BlockRow, error) {
	var out []trace.BlockRow
	for _, b := range m.Blocks {
		if b.SessionID == sessionID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

func memoryMatch(attrs map[string]any, filter any) (bool, error) {
	if filter == nil {
		return true, nil
	}
	f, ok := filter.([]any)
	if !ok || len(f) < 2 {
		return false, fmt.Errorf("invalid filter %v", filter)
	}
	op, _ := f[0].(string)
	switch op {
	case "And", "Or":
		kids, _ := f[1].([]any)
		for _, k := range kids {
			ok, err := memoryMatch(attrs, k)
			if err != nil {
				return false, err
			}
			if ok == (op == "Or") {
				return ok, nil
			}
		}
		return op == "And", nil
	case "Not":
		ok, err := memoryMatch(attrs, f[1])
		return !ok, err
	}
	if len(f) != 3 {
		return false, fmt.Errorf("invalid filter %v", filter)
	}
	value := attrs[op]
	want := f[2]
	switch f[1] {
	case "Eq":
		return memoryEqual(value, want), nil
	case "NotEq":
		return !memoryEqual(value, want), nil
	case "In", "NotIn":
		in := false
		for _, w := range toList(want) {
			if memoryEqual(value, w) {
				in = true
			}
		}
		return in == (f[1] == "In"), nil
	case "Gt", "Gte", "Lt", "Lte":
		a, aok := number(value)
		b, bok := number(want)
		if !aok || !bok {
			return false, nil
		}
		switch f[1] {
		case "Gt":
			return a > b, nil
		case "Gte":
			return a >= b, nil
		case "Lt":
			return a < b, nil
		}
		return a <= b, nil
	case "Contains", "ContainsAny":
		have := toList(value)
		for _, w := range toList(want) {
			for _, h := range have {
				if memoryEqual(h, w) {
					return true, nil
				}
			}
		}
		return false, nil
	case "Glob", "IGlob":
		s, _ := value.(string)
		p, _ := want.(string)
		if f[1] == "IGlob" {
			s, p = strings.ToLower(s), strings.ToLower(p)
		}
		return globMatch(p, s), nil
	}
	// Operators a preview has no need for match nothing rather than all.
	return false, nil
}

func toList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case nil:
		return nil
	}
	return []any{v}
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func memoryEqual(a, b any) bool {
	if x, ok := number(a); ok {
		y, ok := number(b)
		return ok && x == y
	}
	return fmt.Sprint(a) == fmt.Sprint(b) && (a != nil) == (b != nil)
}

// globMatch supports * wildcards, which is all the dashboard sends.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i, p := range parts[1:] {
		last := i == len(parts)-2
		if last {
			return strings.HasSuffix(s, p)
		}
		j := strings.Index(s, p)
		if j < 0 {
			return false
		}
		s = s[j+len(p):]
	}
	return true
}
