package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Invented fixtures only. Queue keeps two distinct evals in the same session.
type fixtureLabels struct{ store ReviewStore }

func (f fixtureLabels) Queue(ctx context.Context, tenant string, q QueueRequest) (QueuePage, error) {
	labels := []Label{{ID: "one", EvalID: "eval-one", Kind: "repair", Session: "invented-session", Turn: "2", Source: "invented-source-one", Policy: "fixture-v1", Prediction: "true", Uncertain: true, Synthetic: true, SampleClass: "synthetic", Judgments: []Judgment{{ActorType: "agent", Actor: "fixture-agent", Verdict: "uncertain"}}}, {ID: "two", EvalID: "eval-two", Kind: "retry", Session: "invented-session", Turn: "3", Source: "invented-source-two", Policy: "fixture-v1", Prediction: "false", Synthetic: true, SampleClass: "synthetic"}}
	page := QueuePage{Items: []Label{}, Totals: []KindTotals{}, Coverage: "Synthetic index only; complete. No genuine precision estimate.", Truncated: true}
	selected := []Label{}
	for _, l := range labels {
		h, err := f.store.History(ctx, tenant, l.ID)
		if err != nil {
			return page, err
		}
		reviewed := len(h) > 0
		rt := KindTotals{SampleClass: "synthetic", State: "lower_bound", Kind: l.Kind, Required: 3, Shortage: 2}
		if reviewed {
			rt.Reviewed = 1
		} else {
			rt.Remaining = 1
		}
		if l.Uncertain {
			rt.Uncertain = 1
		}
		page.Totals = append(page.Totals, rt)
		if q.Kind != "" && q.Kind != l.Kind || q.Status == "reviewed" && !reviewed || q.Status == "unreviewed" && reviewed || q.Uncertainty == "uncertain" && !l.Uncertain || q.Uncertainty == "certain" && l.Uncertain {
			continue
		}
		selected = append(selected, l)
	}
	start := 0
	if q.Cursor != "" {
		if q.Cursor != tenant+":one" {
			return page, ErrReviewNotFound
		}
		start = 1
	}
	if start > len(selected) {
		start = len(selected)
	}
	end := min(start+q.Limit, len(selected))
	page.Items = selected[start:end]
	if end < len(selected) {
		page.Next = tenant + ":one"
	}
	return page, nil
}
func (f fixtureLabels) Context(_ context.Context, tenant, id string) (LabelContext, error) {
	if tenant != "tenant-a" && tenant != "tenant-b" || (id != "one" && id != "two") {
		return LabelContext{}, ErrReviewNotFound
	}
	return LabelContext{State: "ambiguous", Explanation: "Invented ordering ambiguity; preceding actions are available, source order unverified.", Blocks: []ContextBlock{{Role: "assistant", Text: "Run invented check", Source: tenant}, {Role: "tool", Text: "Invented error", Source: tenant}, {Role: "assistant", Text: "Retry invented check", Source: tenant}, {Role: "tool", Text: "Invented repeated error <script>alert(1)</script>", Source: tenant}}}, nil
}
func reviewFixture(t *testing.T) (*SQLiteReviewStore, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h, err := NewHosted(Config{Endpoint: "http://gateway.invalid", Review: &ReviewConfig{Source: fixtureLabels{store}, Store: store, Kinds: []string{"repair", "retry"}, Identity: func(r *http.Request, c Credentials) (Principal, error) {
		return Principal{Tenant: c.Namespace, Reviewer: "verified-" + c.Namespace}, nil
	}}}, func(r *http.Request) (Credentials, error) {
		c, err := r.Cookie("fixture_auth")
		if err != nil || (c.Value != "tenant-a" && c.Value != "tenant-b") {
			return Credentials{}, errors.New("denied")
		}
		return Credentials{Key: "invented-key", Namespace: c.Value}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, h, dir
}
func callReview(h http.Handler, method, path, tenant, body, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://review.test"+path, strings.NewReader(body))
	if tenant != "" {
		r.AddCookie(&http.Cookie{Name: "fixture_auth", Value: tenant})
	}
	if method == "POST" {
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Kit-Review", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestReviewLifecycleAndIsolation(t *testing.T) {
	_, h, dir := reviewFixture(t)
	for _, route := range []string{"/review", "/ui/review.js", "/ui/review.css", "/api/review/queue", "/api/review/items/one/context", "/api/review/items/one/reviews"} {
		for _, method := range []string{"GET", "POST"} {
			if w := callReview(h, method, route, "", "{}", "http://review.test"); w.Code != 401 {
				t.Fatalf("unauth %s %s: %d", method, route, w.Code)
			}
		}
	}
	w := callReview(h, "GET", "/api/review/queue", "tenant-a", "", "")
	var queue struct{ Page QueuePage }
	if err := json.Unmarshal(w.Body.Bytes(), &queue); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(queue.Page.Items) != 2 || queue.Page.Items[0].Session != queue.Page.Items[1].Session || queue.Page.Items[0].EvalID == queue.Page.Items[1].EvalID {
		t.Fatal("two labels/session lost")
	}
	for _, q := range []string{"limit=0", "limit=101", "limit=bad", "kind=unknown", "status=invalid", "uncertainty=invalid"} {
		if w := callReview(h, "GET", "/api/review/queue?"+q, "tenant-a", "", ""); w.Code != 400 {
			t.Fatalf("accepted %s", q)
		}
	}
	w = callReview(h, "GET", "/api/review/queue?limit=1", "tenant-a", "", "")
	json.Unmarshal(w.Body.Bytes(), &queue)
	if len(queue.Page.Items) != 1 || queue.Page.Next == "" {
		t.Fatal("pagination")
	}
	if w := callReview(h, "GET", "/api/review/queue?limit=1&cursor=tenant-a:one", "tenant-b", "", ""); w.Code != 404 {
		t.Fatal("cross tenant cursor accepted")
	}
	body := `{"verdict":"uncertain","note":"invented note","expected_revision":0}`
	for _, origin := range []string{"", "http://evil.test", "https://review.test", "http://review.test/path"} {
		if w := callReview(h, "POST", "/api/review/items/one/reviews", "tenant-a", body, origin); w.Code != 403 {
			t.Fatalf("csrf accepted %q %d", origin, w.Code)
		}
	}
	for _, extra := range []string{`,"reviewer":"impostor"`, `,"tenant":"tenant-b"`, `,"actor_type":"human"`} {
		if w := callReview(h, "POST", "/api/review/items/one/reviews", "tenant-a", strings.TrimSuffix(body, "}")+extra+"}", "http://review.test"); w.Code != 400 {
			t.Fatal("impersonation accepted")
		}
	}
	if w := callReview(h, "POST", "/api/review/items/private-other/reviews", "tenant-a", body, "http://review.test"); w.Code != 404 {
		t.Fatal("unowned item accepted")
	}
	w = callReview(h, "POST", "/api/review/items/one/reviews", "tenant-a", body, "http://review.test")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var review Review
	json.Unmarshal(w.Body.Bytes(), &review)
	if review.Reviewer != "verified-tenant-a" || review.ActorType != "human" {
		t.Fatal("identity not bound")
	}
	if w := callReview(h, "POST", "/api/review/items/one/reviews", "tenant-a", body, "http://review.test"); w.Code != 409 {
		t.Fatal("stale revision accepted")
	}
	revised := `{"verdict":"correct","note":"invented revision","expected_revision":1}`
	if w := callReview(h, "POST", "/api/review/items/one/reviews", "tenant-a", revised, "http://review.test"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	fresh, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	history, err := fresh.History(context.Background(), "tenant-a", "one")
	if err != nil || len(history) != 2 || history[0].Verdict != "uncertain" || history[1].Verdict != "correct" {
		t.Fatal("durable history", history, err)
	}
	other, err := fresh.History(context.Background(), "tenant-b", "one")
	if err != nil || len(other) != 0 {
		t.Fatal("tenant leak")
	}
	w = callReview(h, "GET", "/api/review/queue?status=unreviewed", "tenant-a", "", "")
	json.Unmarshal(w.Body.Bytes(), &queue)
	if len(queue.Page.Items) != 1 || queue.Page.Items[0].ID != "two" || queue.Page.Totals[0].Reviewed != 1 {
		t.Fatal("resume/totals")
	}
}
func TestReviewBrowser(t *testing.T) {
	if os.Getenv("KIT_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set KIT_PLAYWRIGHT_MODULE to existing playwright module; no downloads")
	}
	_, h, _ := reviewFixture(t)
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	script, err := filepath.Abs("../../scripts/test-review-browser.cjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "node", script, server.URL)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	}
	t.Log(string(out))
}
func TestReviewConcurrentStore(t *testing.T) {
	s, _, dir := reviewFixture(t)
	other, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	errs := make(chan error, 2)
	for _, store := range []*SQLiteReviewStore{s, other} {
		go func(store *SQLiteReviewStore) {
			_, err := store.Save(context.Background(), Principal{"tenant-a", "verified-human"}, "one", ReviewChange{Verdict: "correct"})
			errs <- err
		}(store)
	}
	first, second := <-errs, <-errs
	if !((first == nil && errors.Is(second, ErrReviewConflict)) || (second == nil && errors.Is(first, ErrReviewConflict))) {
		t.Fatal(first, second)
	}
}

// Assert source failures cannot expose notes, upstream errors, or credentials.
type failingLabels struct{ fixtureLabels }

func (f failingLabels) Context(context.Context, string, string) (LabelContext, error) {
	return LabelContext{}, fmt.Errorf("invented-secret-do-not-reflect")
}
func TestReviewErrorsAreGeneric(t *testing.T) {
	_, _, _ = reviewFixture(t)
	req := httptest.NewRequest("GET", "http://review.test/api/review/items/one/context", nil)
	w := httptest.NewRecorder()
	c := ReviewConfig{Source: failingLabels{}, Identity: func(*http.Request, Credentials) (Principal, error) { return Principal{"tenant-a", "human"}, nil }}
	c.handle(w, req, Credentials{Namespace: "tenant-a"})
	body, _ := io.ReadAll(w.Result().Body)
	if w.Code != 502 || strings.Contains(string(body), "invented-secret") {
		t.Fatal("private error leak")
	}
}
