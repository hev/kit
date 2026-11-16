package filearchive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Filters arrive in the store's filter form, the same values kit sends Layer:
// ["attr", "Op", value], ["And", [...]], ["Or", [...]], ["Not", f]. Rows and
// filters are both compared in their JSON form, so an int64 bound written by
// the dashboard and a number read back from a file compare as numbers.

type compiled func(row map[string]any) bool

// compileFilter checks the filter once and returns a predicate. A nil filter
// matches every row.
func compileFilter(raw any) (compiled, error) {
	if raw == nil {
		return func(map[string]any) bool { return true }, nil
	}
	norm, err := normalize(raw)
	if err != nil {
		return nil, err
	}
	return compileNode(norm)
}

func normalize(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out any
	if err := d.Decode(&out); err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	return out, nil
}

func compileNode(node any) (compiled, error) {
	if node == nil {
		return func(map[string]any) bool { return true }, nil
	}
	f, ok := node.([]any)
	if !ok || len(f) < 2 {
		return nil, fmt.Errorf("filter: unsupported form %v", node)
	}
	head, _ := f[0].(string)
	switch head {
	case "And", "Or":
		clauses, ok := f[1].([]any)
		if !ok {
			return nil, fmt.Errorf("filter: %s needs a list", head)
		}
		parts := make([]compiled, 0, len(clauses))
		for _, c := range clauses {
			p, err := compileNode(c)
			if err != nil {
				return nil, err
			}
			parts = append(parts, p)
		}
		if head == "And" {
			return func(row map[string]any) bool {
				for _, p := range parts {
					if !p(row) {
						return false
					}
				}
				return true
			}, nil
		}
		return func(row map[string]any) bool {
			for _, p := range parts {
				if p(row) {
					return true
				}
			}
			return false
		}, nil
	case "Not":
		p, err := compileNode(f[1])
		if err != nil {
			return nil, err
		}
		return func(row map[string]any) bool { return !p(row) }, nil
	}
	if len(f) != 3 {
		return nil, fmt.Errorf("filter: unsupported form %v", node)
	}
	attr := head
	op, _ := f[1].(string)
	target := f[2]
	switch op {
	case "Eq":
		return func(row map[string]any) bool { return equal(row[attr], target) }, nil
	case "NotEq":
		return func(row map[string]any) bool { return !equal(row[attr], target) }, nil
	case "In", "NotIn":
		list, ok := target.([]any)
		if !ok {
			return nil, fmt.Errorf("filter: %s %s needs a list", attr, op)
		}
		set := map[string]bool{}
		for _, v := range list {
			set[key(v)] = true
		}
		in := func(row map[string]any) bool { return set[key(row[attr])] }
		if op == "In" {
			return in, nil
		}
		return func(row map[string]any) bool { return !in(row) }, nil
	case "Contains", "NotContains":
		want := key(target)
		has := func(row map[string]any) bool {
			for _, v := range asList(row[attr]) {
				if key(v) == want {
					return true
				}
			}
			return false
		}
		if op == "Contains" {
			return has, nil
		}
		return func(row map[string]any) bool { return !has(row) }, nil
	case "ContainsAny", "NotContainsAny":
		list, ok := target.([]any)
		if !ok {
			return nil, fmt.Errorf("filter: %s %s needs a list", attr, op)
		}
		set := map[string]bool{}
		for _, v := range list {
			set[key(v)] = true
		}
		has := func(row map[string]any) bool {
			for _, v := range asList(row[attr]) {
				if set[key(v)] {
					return true
				}
			}
			return false
		}
		if op == "ContainsAny" {
			return has, nil
		}
		return func(row map[string]any) bool { return !has(row) }, nil
	case "Gt", "Gte", "Lt", "Lte":
		return func(row map[string]any) bool {
			c, ok := compare(row[attr], target)
			if !ok {
				return false
			}
			switch op {
			case "Gt":
				return c > 0
			case "Gte":
				return c >= 0
			case "Lt":
				return c < 0
			default:
				return c <= 0
			}
		}, nil
	case "Glob", "NotGlob", "IGlob", "NotIGlob":
		pattern, _ := target.(string)
		fold := strings.HasPrefix(op, "I") || strings.HasPrefix(op, "NotI")
		if fold {
			pattern = strings.ToLower(pattern)
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("filter: bad glob %q", pattern)
		}
		match := func(row map[string]any) bool {
			s, ok := row[attr].(string)
			if !ok {
				return false
			}
			if fold {
				s = strings.ToLower(s)
			}
			m, _ := path.Match(pattern, s)
			return m
		}
		if strings.HasPrefix(op, "Not") {
			return func(row map[string]any) bool { return !match(row) }, nil
		}
		return match, nil
	}
	return nil, fmt.Errorf("filter: unsupported operator %q", op)
}

// asList reads an array attribute, including the JSON-string form older
// archives use for lists.
func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case string:
		if strings.HasPrefix(t, "[") {
			var out []any
			d := json.NewDecoder(strings.NewReader(t))
			d.UseNumber()
			if d.Decode(&out) == nil {
				return out
			}
		}
	}
	return nil
}

func key(v any) string {
	switch t := v.(type) {
	case nil:
		return "\x00nil"
	case string:
		return "s" + t
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return fmt.Sprintf("n%v", f)
		}
		return "n" + t.String()
	case bool:
		return fmt.Sprintf("b%v", t)
	}
	b, _ := json.Marshal(v)
	return "j" + string(b)
}

func equal(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return key(a) == key(b)
}

func compare(a, b any) (int, bool) {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return 0, false
		}
		fx, e1 := x.Float64()
		fy, e2 := y.Float64()
		if e1 != nil || e2 != nil {
			return 0, false
		}
		switch {
		case fx < fy:
			return -1, true
		case fx > fy:
			return 1, true
		}
		return 0, true
	case string:
		y, ok := b.(string)
		if !ok {
			return 0, false
		}
		return strings.Compare(x, y), true
	}
	return 0, false
}

// decodeRow turns a stored JSON object into the map the predicates read.
func decodeRow(raw []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var out map[string]any
	if err := d.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// filterSessions finds session ids a filter pins down: a top-level (or
// And-ed) session_id Eq or In clause. ok is false when the filter does not
// restrict sessions that way.
func filterSessions(raw any) (map[string]bool, bool) {
	if raw == nil {
		return nil, false
	}
	norm, err := normalize(raw)
	if err != nil {
		return nil, false
	}
	return sessionsOf(norm)
}

func sessionsOf(node any) (map[string]bool, bool) {
	f, ok := node.([]any)
	if !ok || len(f) < 2 {
		return nil, false
	}
	if f[0] == "And" {
		clauses, _ := f[1].([]any)
		var out map[string]bool
		found := false
		for _, c := range clauses {
			ids, ok := sessionsOf(c)
			if !ok {
				continue
			}
			if !found {
				out, found = ids, true
				continue
			}
			for id := range out {
				if !ids[id] {
					delete(out, id)
				}
			}
		}
		return out, found
	}
	if len(f) != 3 || f[0] != "session_id" {
		return nil, false
	}
	switch f[1] {
	case "Eq":
		if s, ok := f[2].(string); ok {
			return map[string]bool{s: true}, true
		}
	case "In":
		list, _ := f[2].([]any)
		out := map[string]bool{}
		for _, v := range list {
			if s, ok := v.(string); ok {
				out[s] = true
			}
		}
		return out, true
	}
	return nil, false
}
