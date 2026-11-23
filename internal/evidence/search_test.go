package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func searchFixture(t *testing.T) (*Store, map[string]string) {
	t.Helper()
	s := testStore(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { now = now.Add(time.Minute); return now }
	ids := map[string]string{}
	put := func(key string, m Meta, items []ItemInput, claims []ClaimInput) {
		p, err := s.Put(m, items, claims)
		if err != nil {
			t.Fatal(err)
		}
		ids[key] = p.ID
	}
	put("checkout", Meta{Title: "Checkout mobile viewport overflow", Order: "checkout-fix", Source: "browser-use", Repo: "hev/app", Tags: []string{"mobile"}},
		[]ItemInput{{Data: pngOf(t, 390, 844), Name: "checkout-390px.png", Caption: "Submit button overflows at 390px"}},
		[]ClaimInput{{Text: "Submit button stays inside the viewport", Verdict: "pass"}})
	put("console", Meta{Title: "Console dump", Order: "checkout-fix", Source: "validator", Gate: "browser-validation", Author: "val-1"},
		[]ItemInput{{Data: []byte("line one\nTypeError: cannot read property 'total' of undefined\nline three\n"), Name: "console.log"}},
		[]ClaimInput{{Text: "no errors", Verdict: "fail"}})
	put("billing", Meta{Title: "Billing export", Order: "billing-uat", Source: "computer-use", Session: "sess-9", Tags: []string{"billing"}},
		[]ItemInput{{Data: mp4Of("isom", 5000, 8, 8), Name: "flow.mp4", Caption: "CSV export flow"}, {Data: []byte(strings.Repeat("row,ok\n", 12000) + "export retried twice\n"), Name: "big.csv"}},
		nil)
	put("spread", Meta{Title: "Spread words", Summary: "alpha"}, []ItemInput{{Data: []byte("omega"), Name: "o.txt"}}, nil)
	return s, ids
}

func hitIDs(r *SearchResponse) string {
	var out []string
	for _, h := range r.Data {
		out = append(out, h.Package.ID)
	}
	return strings.Join(out, ",")
}

// Rule: ripgrep and the Go walk return the same package-grouped hits for
// every query and filter. Only the engine label may differ.
func TestSearchParityRgAndGo(t *testing.T) {
	s, ids := searchFixture(t)
	queries := []SearchQuery{
		{Q: "checkout button overflow"},
		{Q: "button overflows", Also: []string{"390px viewport"}},
		{Q: "TypeError undefined"},
		{Q: "nothing matches this", Also: []string{"typeerror"}},
		{Q: "export retries"},
		{Q: "csv export flow", Filter: Filter{Modalities: []string{"video"}}},
		{Q: "checkout", Filter: Filter{Order: "checkout-fix", Verdicts: []string{"fail"}}},
		{Q: "checkout", Filter: Filter{Sources: []string{"browser-use"}, Repo: "hev/app", Tags: []string{"mobile"}}},
		{Q: "console", Filter: Filter{Gate: "browser-validation", Author: "val-1"}},
		{Q: "billing", Filter: Filter{Session: "sess-9", Since: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)}},
		{Q: "alpha omega"},
		{Q: `"quoted" <html>`},
		{Q: "zzz-no-such-word"},
	}
	run := func(engine string) []string {
		t.Setenv("HEV_GREP", engine)
		var out []string
		for _, q := range queries {
			r, err := s.Search(context.Background(), q)
			if err != nil {
				t.Fatalf("%s %q: %v", engine, q.Q, err)
			}
			if r.Semantic || r.Route != "phrase.scan" {
				t.Fatalf("meta %+v", r)
			}
			r.Engine = ""
			b, _ := json.Marshal(r)
			out = append(out, string(b))
		}
		return out
	}
	goOut := run("go")
	if _, err := exec.LookPath("rg"); err != nil {
		t.Log("rg not on PATH: only the Go walk ran")
	} else {
		t.Setenv("HEV_GREP", "")
		r, _ := s.Search(context.Background(), SearchQuery{Q: "checkout"})
		if r.Engine != "rg" {
			t.Fatalf("engine %s with rg on PATH", r.Engine)
		}
		rgOut := run("")
		for i := range queries {
			if rgOut[i] != goOut[i] {
				t.Errorf("query %q differs:\n rg %s\n go %s", queries[i].Q, rgOut[i], goOut[i])
			}
		}
	}
	// And the hits are the right ones.
	t.Setenv("HEV_GREP", "go")
	for _, tt := range []struct {
		q    SearchQuery
		want string
	}{
		{queries[0], ids["checkout"]},
		{queries[1], ids["checkout"]},
		{queries[2], ids["console"]},
		{queries[3], ids["console"]},
		{queries[4], ids["billing"]},
		{queries[5], ids["billing"]},
		{queries[6], ids["console"]},
		{queries[7], ids["checkout"]},
		{queries[9], ids["billing"]},
		{queries[10], ids["spread"]},
		{queries[12], ""},
	} {
		r, err := s.Search(context.Background(), tt.q)
		if err != nil {
			t.Fatal(err)
		}
		if got := hitIDs(r); got != tt.want {
			t.Errorf("%q: %s, want %s", tt.q.Q, got, tt.want)
		}
	}
	r, _ := s.Search(context.Background(), SearchQuery{Q: "TypeError"})
	m := r.Data[0].Matches[0]
	if m.Target != "item" || m.Modality != "text" || !strings.Contains(m.Snippet, "TypeError") || m.Seq != 1 {
		t.Fatalf("match %+v", m)
	}
	// Two packages in one order rank by words hit, then newest.
	r, _ = s.Search(context.Background(), SearchQuery{Q: "checkout"})
	if hitIDs(r) != ids["console"]+","+ids["checkout"] {
		t.Fatalf("ranking %s", hitIDs(r))
	}
}

func TestSearchRefusals(t *testing.T) {
	s, _ := searchFixture(t)
	if _, err := s.Search(context.Background(), SearchQuery{Q: "x", Also: make([]string, MaxAlso+1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("8 also: %v", err)
	}
	if _, err := s.Search(context.Background(), SearchQuery{Q: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty query: %v", err)
	}
	if _, err := s.Search(context.Background(), SearchQuery{Q: "x", Filter: Filter{Verdicts: []string{"maybe"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad verdict: %v", err)
	}
	if _, err := s.Search(context.Background(), SearchQuery{Q: "x", Also: []string{"a", "b", "c", "d", "e", "f", "g"}}); err != nil {
		t.Fatalf("7 also: %v", err)
	}
}
