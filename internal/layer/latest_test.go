package layer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func latestFixtureRequest() LatestRequest {
	return LatestRequest{
		LatestBy:    LatestSpec{Key: []string{"entity"}, OrderBy: []LatestOrder{{Field: "version", Direction: "desc", Nulls: "last"}, {Field: "id", Direction: "desc", Nulls: "last", CanonicalKey: "entity"}}},
		Scan:        LatestScan{MaxRows: 100, MaxBytes: 100000, Consistency: "observed"},
		Filters:     []any{"amount", "Gte", json.Number("9007199254740993")},
		AggregateBy: map[string][]string{"n": {"Count"}, "amount": {"Sum", "amount"}},
		Grouped: map[string]LatestGroupPlan{
			"models": {GroupBy: []LatestDimension{{Kind: "field", Field: "model"}}, Metrics: map[string]LatestMetric{"n": {Kind: "count"}}, PageSize: 1},
			"days":   {GroupBy: []LatestDimension{{Kind: "utc_day", Field: "start"}}, Metrics: map[string]LatestMetric{"n": {Kind: "count"}}, PageSize: 1},
		},
	}
}
func latestFixturePage(model, day string, more bool) latestPage {
	p := latestPage{Aggregations: map[string]json.Number{"n": "3", "amount": "9007199254740993"}, Grouped: map[string]latestGroupPage{}, Relation: LatestRelation{Complete: true, Consistency: "observed", ReadBoundary: "observed-source", SourceRows: 4, SourcePages: 1, SourceBytes: 800, SelectedRows: 3, MatchingRows: 3, Plan: "bounded_source_scan"}}
	for name, value := range map[string]string{"models": model, "days": day} {
		key, _ := json.Marshal(value)
		g := latestGroupPage{Groups: []LatestGroup{{Key: []LatestKey{{Type: "string", Value: key}}, Aggregates: map[string]json.Number{"n": "1"}}}, Complete: true, TotalGroups: 1, Binding: name + "-binding"}
		g.Relation.Complete = true
		g.Relation.Consistency = "observed_scan"
		if name == "models" {
			g.TotalGroups = 2
			if more {
				g.Complete = false
				g.NextCursor = "model-page-2"
			}
		}
		p.Grouped[name] = g
	}
	return p
}

func TestLatestFrozenNamedContinuations(t *testing.T) {
	request := latestFixtureRequest()
	var initial LatestRequest
	calls := 0
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.EscapedPath() != "/v2/namespaces/tenant-a/aggregate" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("routing/auth changed: %s", r.URL)
		}
		var got LatestRequest
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&got); err != nil {
			t.Error(err)
		}
		if calls == 1 {
			initial = got
			// Mutation after the first request must never enter the continuation.
			request.AggregateBy["amount"] = []string{"Sum", "other"}
			request.Filters.([]any)[2] = json.Number("1")
			plan := request.Grouped["days"]
			plan.PageSize = 999
			request.Grouped["days"] = plan
		} else {
			plan := got.Grouped["models"]
			if plan.Cursor != "model-page-2" {
				t.Error("missing named cursor")
			}
			plan.Cursor = ""
			got.Grouped["models"] = plan
			if !reflect.DeepEqual(initial, got) {
				t.Errorf("query changed: %v vs %v", initial, got)
			}
		}
		_ = json.NewEncoder(w).Encode(latestFixturePage(fmt.Sprint(calls), "same-day", calls == 1))
	}))
	defer wire.Close()
	result, err := New(wire.URL, "fixture-key", "tenant-a", "").AggregateLatest(request)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(result.Grouped["models"]) != 2 || len(result.Grouped["days"]) != 1 || result.Aggregations["amount"] != "9007199254740993" || result.Aggregations["n"] != "3" || len(result.Executions) != 2 {
		t.Fatalf("double consumption or loss: %+v calls %d", result, calls)
	}
}

func TestLatestDiscardsPartial(t *testing.T) {
	for _, failure := range []string{"auth", "server", "unsupported", "cursor", "totals", "binding", "missing-plan", "incomplete", "repeated-key", "empty-page", "count", "loop", "data", "quoted-number", "null-number"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				p := latestFixturePage(fmt.Sprint(calls), "day", calls == 1)
				if calls == 2 {
					switch failure {
					case "auth":
						http.Error(w, "denied", 401)
						return
					case "server":
						http.Error(w, "failed", 500)
						return
					case "unsupported":
						w.WriteHeader(422)
						fmt.Fprint(w, `{"error":"UnsupportedByStore","feature":"latest_by"}`)
						return
					case "cursor":
						http.Error(w, "group continuation does not match", 400)
						return
					case "totals":
						p.Aggregations["n"] = "4"
					case "binding":
						g := p.Grouped["models"]
						g.Binding = "changed"
						p.Grouped["models"] = g
					case "missing-plan":
						delete(p.Grouped, "days")
					case "incomplete":
						p.Relation.Complete = false
					case "repeated-key":
						p = latestFixturePage("1", "day", false)
					case "empty-page":
						g := p.Grouped["models"]
						g.Groups = nil
						g.Complete = false
						g.NextCursor = "third"
						p.Grouped["models"] = g
					case "count":
						g := p.Grouped["models"]
						g.TotalGroups = 3
						p.Grouped["models"] = g
					case "loop":
						g := p.Grouped["models"]
						g.Complete = false
						g.NextCursor = "model-page-2"
						p.Grouped["models"] = g
					case "quoted-number":
						fmt.Fprint(w, `{"aggregations":{"n":"3","amount":0}}`)
						return
					case "null-number":
						fmt.Fprint(w, `{"aggregations":{"n":null,"amount":0}}`)
						return
					case "data":
						fmt.Fprint(w, `{"aggregations":{"n":{}}}`)
						return
					}
				}
				_ = json.NewEncoder(w).Encode(p)
			}))
			defer wire.Close()
			result, err := New(wire.URL, "fixture", "test", "").AggregateLatest(latestFixtureRequest())
			if err == nil || result != nil || calls != 2 {
				t.Fatalf("partial accepted: %+v %v calls=%d", result, err, calls)
			}
			if LatestUnsupported(err) != (failure == "unsupported") {
				t.Fatalf("wrong fallback classification: %v", err)
			}
		})
	}
}

func TestLatestCancellation(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		t.Run(fmt.Sprint(continuation), func(t *testing.T) {
			entered := make(chan struct{})
			observed := make(chan struct{})
			calls := 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_, _ = io.Copy(io.Discard, r.Body)
				if continuation && calls == 1 {
					_ = json.NewEncoder(w).Encode(latestFixturePage("a", "day", true))
					return
				}
				close(entered)
				select {
				case <-r.Context().Done():
					close(observed)
				case <-time.After(3 * time.Second):
					t.Error("no cancellation")
				}
			}))
			defer wire.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				result, err := New(wire.URL, "fixture", "test", "").WithContext(ctx).AggregateLatest(latestFixtureRequest())
				if result != nil {
					done <- fmt.Errorf("partial escaped")
					return
				}
				done <- err
			}()
			<-entered
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("error %v", err)
			}
			<-observed
		})
	}
}

func TestLatestUnsupportedIsNarrow(t *testing.T) {
	for _, body := range []string{`{"error":"UnsupportedByStore","feature":"other"}`, `{"error":"invalid plan"}`, `not json`} {
		if LatestUnsupported(&HTTPError{Status: 422, Body: []byte(body)}) {
			t.Fatal(body)
		}
	}
	request := latestFixtureRequest()
	p := request.Grouped["days"]
	p.Cursor = "foreign"
	request.Grouped["days"] = p
	_, err := New("http://unused.invalid", "fixture", "test", "").AggregateLatest(request)
	if err == nil || !strings.Contains(err.Error(), "initial cursor") {
		t.Fatal(err)
	}
}

func TestLatestTenantIsolation(t *testing.T) {
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q LatestRequest
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		tenant := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/namespaces/"), "/aggregate")
		if r.Header.Get("Authorization") != "Bearer "+tenant {
			http.Error(w, "denied", 403)
			return
		}
		more := q.Grouped["models"].Cursor == ""
		if !more && q.Grouped["models"].Cursor != tenant+"-cursor" {
			http.Error(w, "foreign continuation", 400)
			return
		}
		p := latestFixturePage(tenant+fmt.Sprint(more), "day", more)
		g := p.Grouped["models"]
		g.Binding = tenant + "-binding"
		if more {
			g.NextCursor = tenant + "-cursor"
		}
		p.Grouped["models"] = g
		_ = json.NewEncoder(w).Encode(p)
	}))
	defer wire.Close()
	done := make(chan error, 2)
	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		go func(tenant string) {
			result, err := New(wire.URL, tenant, tenant, "").AggregateLatest(latestFixtureRequest())
			if err == nil {
				for _, g := range result.Grouped["models"] {
					var key string
					_ = json.Unmarshal(g.Key[0].Value, &key)
					if !strings.HasPrefix(key, tenant) {
						err = fmt.Errorf("foreign group %s", key)
					}
				}
			}
			done <- err
		}(tenant)
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestLatestEmptyAndScalarOnly(t *testing.T) {
	for _, grouped := range []bool{true, false} {
		t.Run(fmt.Sprint(grouped), func(t *testing.T) {
			q := latestFixtureRequest()
			if !grouped {
				q.Grouped = nil
			}
			calls := 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				p := latestFixturePage("", "", false)
				p.Aggregations = map[string]json.Number{"n": "0", "amount": "0"}
				p.Relation.SourceRows = 0
				p.Relation.SelectedRows = 0
				p.Relation.MatchingRows = 0
				if !grouped {
					p.Grouped = nil
				} else {
					for name, g := range p.Grouped {
						g.Groups = []LatestGroup{}
						g.TotalGroups = 0
						p.Grouped[name] = g
					}
				}
				_ = json.NewEncoder(w).Encode(p)
			}))
			defer wire.Close()
			result, err := New(wire.URL, "fixture", "test", "").AggregateLatest(q)
			if err != nil || result == nil || calls != 1 || result.Aggregations["n"] != "0" {
				t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestLatestModelExpansionWire(t *testing.T) {
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q LatestRequest
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		plan := q.Grouped["usage"]
		if plan.Expand == nil || plan.Expand.Kind != "models" || plan.Expand.Field != "usage" || plan.Metrics["tokens"].Kind != "model_sum" || plan.Metrics["tokens"].Field != "input" {
			t.Errorf("model plan changed: %+v", plan)
		}
		fmt.Fprint(w, `{"aggregations":{"n":1},"grouped":{"usage":{"groups":[{"key":[{"type":"string","value":"model-a"}],"aggregates":{"tokens":9007199254740993}}],"next_cursor":null,"complete":true,"total_groups":1,"binding":"usage-synthetic","relation":{"complete":true,"consistency":"observed_scan"}}},"relation":{"complete":true,"consistency":"observed","read_boundary":"observed_pages","source_rows":1,"source_pages":1,"source_bytes":100,"selected_rows":1,"matching_rows":1,"plan":"bounded_source_scan"}}`)
	}))
	defer wire.Close()
	q := latestFixtureRequest()
	q.AggregateBy = map[string][]string{"n": {"Count"}}
	q.Grouped = map[string]LatestGroupPlan{"usage": {Expand: &LatestExpansion{Kind: "models", Field: "usage"}, GroupBy: []LatestDimension{{Kind: "model"}}, Metrics: map[string]LatestMetric{"tokens": {Kind: "model_sum", Field: "input"}}, PageSize: 100}}
	result, err := New(wire.URL, "fixture", "test", "").AggregateLatest(q)
	if err != nil {
		t.Fatal(err)
	}
	if result.Grouped["usage"][0].Aggregates["tokens"] != "9007199254740993" {
		t.Fatal("lost per-model integer precision")
	}
}
