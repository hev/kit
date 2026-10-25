package layer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
)

// LatestRequest is the af8bfd9 latest-relation wire, not physical-row Aggregate.
// Filters are Layer filter expressions. No client authorization/scope fields or
// joins are accepted. Callers must establish complete consumer semantics first.
type LatestRequest struct {
	LatestBy    LatestSpec                 `json:"latest_by"`
	Scan        LatestScan                 `json:"scan"`
	Filters     any                        `json:"filters,omitempty"`
	AggregateBy map[string][]string        `json:"aggregate_by"`
	Grouped     map[string]LatestGroupPlan `json:"grouped,omitempty"`
}
type LatestSpec struct {
	Key     []string      `json:"key"`
	OrderBy []LatestOrder `json:"order_by"`
}
type LatestOrder struct {
	Field        string `json:"field"`
	Direction    string `json:"direction"`
	Nulls        string `json:"nulls"`
	CanonicalKey string `json:"canonical_key,omitempty"`
}
type LatestScan struct {
	MaxRows     int    `json:"max_rows"`
	MaxBytes    int    `json:"max_bytes"`
	Consistency string `json:"consistency"`
}
type LatestDimension struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
}
type LatestExpansion struct {
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	Timezone string `json:"timezone,omitempty"`
}
type LatestMetric struct {
	Kind  string `json:"kind"`
	Field string `json:"field,omitempty"`
}
type LatestGroupPlan struct {
	GroupBy  []LatestDimension       `json:"group_by"`
	Expand   *LatestExpansion        `json:"expand,omitempty"`
	Metrics  map[string]LatestMetric `json:"metrics"`
	PageSize int                     `json:"page_size"`
	Cursor   string                  `json:"cursor,omitempty"`
}

// LatestNumbers preserves exact JSON numbers and rejects strings/null metrics.
type LatestNumbers map[string]json.Number

func (numbers *LatestNumbers) UnmarshalJSON(raw []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	if values == nil {
		return fmt.Errorf("latest: null metrics")
	}
	result := LatestNumbers{}
	for name, value := range values {
		number := json.Number(string(bytes.TrimSpace(value)))
		if !validLatestNumber(number) {
			return fmt.Errorf("latest: metric %s must be a JSON number", name)
		}
		result[name] = number
	}
	*numbers = result
	return nil
}

type LatestKey struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value,omitempty"`
}
type LatestGroup struct {
	Key        []LatestKey   `json:"key"`
	Aggregates LatestNumbers `json:"aggregates"`
}
type LatestRelation struct {
	Complete     bool   `json:"complete"`
	Consistency  string `json:"consistency"`
	ReadBoundary string `json:"read_boundary"`
	SourceRows   int    `json:"source_rows"`
	SourcePages  int    `json:"source_pages"`
	SourceBytes  int    `json:"source_bytes"`
	SelectedRows int    `json:"selected_rows"`
	MatchingRows int    `json:"matching_rows"`
	Plan         string `json:"plan"`
}
type latestGroupPage struct {
	Groups      []LatestGroup `json:"groups"`
	NextCursor  string        `json:"next_cursor"`
	Complete    bool          `json:"complete"`
	TotalGroups int           `json:"total_groups"`
	Binding     string        `json:"binding"`
	Relation    struct {
		Complete    bool   `json:"complete"`
		Consistency string `json:"consistency"`
	} `json:"relation"`
}
type latestPage struct {
	Aggregations LatestNumbers              `json:"aggregations"`
	Grouped      map[string]latestGroupPage `json:"grouped"`
	Relation     LatestRelation             `json:"relation"`
}

// LatestResult contains each named group exactly once and scalar totals once.
// Executions records every full source rescan, including continuation work.
// No partial result survives any error. Observed reads are not snapshots.
type LatestResult struct {
	Aggregations LatestNumbers
	Grouped      map[string][]LatestGroup
	Executions   []LatestRelation
}

// LatestUnsupported recognizes only the exact structured capability rejection.
// Auth, budget, cursor, malformed data and server errors must remain visible.
func LatestUnsupported(err error) bool {
	var e *HTTPError
	if !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity {
		return false
	}
	var body struct {
		Error   string `json:"error"`
		Feature string `json:"feature"`
	}
	return json.Unmarshal(e.Body, &body) == nil && body.Error == "UnsupportedByStore" && body.Feature == "latest_by"
}

// AggregateLatest executes a complete bounded relation on the client's namespace.
// It freezes all query content and client routing/auth before the first call;
// only named cursors change. It never retries on another namespace or scans rows
// as a fallback. Consumers own capability decisions and fallback semantics.
func (c *Client) AggregateLatest(request LatestRequest) (*LatestResult, error) {
	if request.Scan.Consistency != "observed" {
		return nil, fmt.Errorf("latest: only observed consistency is supported")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var frozen LatestRequest
	// Preserve integers in arbitrary nested filter expressions.
	if err := decodeLatestRequest(raw, &frozen); err != nil {
		return nil, err
	}
	for name, plan := range frozen.Grouped {
		if plan.Cursor != "" {
			return nil, fmt.Errorf("latest %s: initial cursor must be empty", name)
		}
		if plan.PageSize < 1 || plan.PageSize > 10000 {
			return nil, fmt.Errorf("latest %s: invalid page size", name)
		}
	}
	client := *c
	path := "/v2/namespaces/" + url.PathEscape(client.Namespace) + "/aggregate"
	result := &LatestResult{Grouped: map[string][]LatestGroup{}}
	done := map[string]bool{}
	bindings := map[string]string{}
	totals := map[string]int{}
	seen := map[string]map[string]bool{}
	keys := map[string]map[string]bool{}
	for execution := 0; execution <= 100000; execution++ {
		var page latestPage
		if err := client.do(http.MethodPost, path, frozen, &page); err != nil {
			return nil, err
		}
		rel := page.Relation
		if !rel.Complete || rel.Consistency != frozen.Scan.Consistency || rel.Plan != "bounded_source_scan" || rel.ReadBoundary == "" || rel.SourceRows < 0 || rel.SourceBytes < 0 || rel.SourcePages < 0 || rel.SelectedRows < 0 || rel.MatchingRows < 0 || rel.MatchingRows > rel.SelectedRows || rel.SelectedRows > rel.SourceRows || rel.SourceRows > frozen.Scan.MaxRows || rel.SourceBytes > frozen.Scan.MaxBytes {
			return nil, fmt.Errorf("latest: incomplete or invalid source relation")
		}
		if len(page.Aggregations) != len(frozen.AggregateBy) {
			return nil, fmt.Errorf("latest: missing scalar metrics")
		}
		for name := range frozen.AggregateBy {
			if _, ok := page.Aggregations[name]; !ok {
				return nil, fmt.Errorf("latest: missing scalar %s", name)
			}
		}
		if execution > 0 {
			first := result.Executions[0]
			if rel.ReadBoundary != first.ReadBoundary || rel.SelectedRows != first.SelectedRows || rel.MatchingRows != first.MatchingRows {
				return nil, fmt.Errorf("latest: relation changed during continuation")
			}
		}
		for _, number := range page.Aggregations {
			if !validLatestNumber(number) {
				return nil, fmt.Errorf("latest: invalid scalar number")
			}
		}
		if execution == 0 {
			result.Aggregations = page.Aggregations
		} else if !reflect.DeepEqual(result.Aggregations, page.Aggregations) {
			return nil, fmt.Errorf("latest: scalar totals changed during continuation")
		}
		result.Executions = append(result.Executions, rel)
		if len(page.Grouped) != len(frozen.Grouped) {
			return nil, fmt.Errorf("latest: missing named plans")
		}
		for name, plan := range frozen.Grouped {
			group, ok := page.Grouped[name]
			if !ok {
				return nil, fmt.Errorf("latest: missing plan %s", name)
			}
			if done[name] {
				continue
			} // Other plans replay; consume only pending pages.
			if !group.Relation.Complete || group.Relation.Consistency != "observed_scan" || group.Binding == "" || group.TotalGroups < 0 || group.TotalGroups > 100000/max(1, len(frozen.Grouped)) || len(group.Groups) > plan.PageSize || (group.Complete != (group.NextCursor == "")) {
				return nil, fmt.Errorf("latest %s: incomplete or invalid group page", name)
			}
			if execution == 0 {
				bindings[name], totals[name] = group.Binding, group.TotalGroups
				seen[name], keys[name] = map[string]bool{}, map[string]bool{}
				result.Grouped[name] = []LatestGroup{}
			} else if bindings[name] != group.Binding || totals[name] != group.TotalGroups {
				return nil, fmt.Errorf("latest %s: relation changed", name)
			}
			for _, row := range group.Groups {
				if len(row.Key) != len(plan.GroupBy) || len(row.Aggregates) != len(plan.Metrics) {
					return nil, fmt.Errorf("latest %s: invalid group shape", name)
				}
				for metric := range plan.Metrics {
					if _, ok := row.Aggregates[metric]; !ok {
						return nil, fmt.Errorf("latest %s: missing metric", name)
					}
				}
				for _, number := range row.Aggregates {
					if !validLatestNumber(number) {
						return nil, fmt.Errorf("latest %s: invalid metric number", name)
					}
				}
				for _, key := range row.Key {
					if !validLatestKey(key) {
						return nil, fmt.Errorf("latest %s: invalid typed key", name)
					}
				}
				key, _ := json.Marshal(row.Key)
				if keys[name][string(key)] {
					return nil, fmt.Errorf("latest %s: repeated group", name)
				}
				keys[name][string(key)] = true
			}
			result.Grouped[name] = append(result.Grouped[name], group.Groups...)
			n := len(result.Grouped[name])
			if n > group.TotalGroups || (group.Complete && n != group.TotalGroups) || (!group.Complete && (len(group.Groups) == 0 || n >= group.TotalGroups || seen[name][group.NextCursor])) {
				return nil, fmt.Errorf("latest %s: group pagination did not advance", name)
			}
			if group.Complete {
				done[name] = true
			} else {
				seen[name][group.NextCursor] = true
				plan.Cursor = group.NextCursor
				frozen.Grouped[name] = plan
			}
		}
		if len(done) == len(frozen.Grouped) {
			return result, nil
		}
	}
	return nil, fmt.Errorf("latest: continuation limit exceeded")
}

func decodeLatestRequest(raw []byte, out *LatestRequest) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(out)
}

func validLatestNumber(number json.Number) bool {
	raw := []byte(number.String())
	return len(raw) > 0 && json.Valid(raw) && (raw[0] == '-' || raw[0] >= '0' && raw[0] <= '9')
}
func validLatestKey(key LatestKey) bool {
	switch key.Type {
	case "null":
		return len(key.Value) == 0
	case "bool":
		return string(key.Value) == "true" || string(key.Value) == "false"
	case "string":
		var value string
		return len(key.Value) > 0 && key.Value[0] == '"' && json.Unmarshal(key.Value, &value) == nil
	case "number":
		var value string
		return json.Unmarshal(key.Value, &value) == nil && validLatestNumber(json.Number(value))
	}
	return false
}
