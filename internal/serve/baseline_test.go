package serve

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestEvalBaselineLatestAndCache(t *testing.T) {
	s, f := dashboardFixture(t)
	data := requestJSON(t, s, "/api/session/trace-0")
	baseline := data["eval_baseline"].(map[string]any)
	all := baseline["overall"].(map[string]any)
	accuracy := all["accuracy"].(map[string]any)
	if accuracy["avg"] != 3.5 || accuracy["n"] != float64(2) {
		t.Fatalf("latest averages: %v", all)
	}
	if all["clarity"].(map[string]any)["n"] != float64(1) {
		t.Fatal("missing marks counted")
	}
	project := baseline["project"].(map[string]any)
	if project["accuracy"].(map[string]any)["avg"] != float64(2) {
		t.Fatalf("project: %v", project)
	}
	f.mu.Lock()
	for _, r := range f.rows["test-evals"] {
		if r["session_id"] == "trace-0" {
			r["marks"] = `{"accuracy":4}`
		}
	}
	f.mu.Unlock()
	// A new timed request must reuse the server cache, while its ordinary
	// evaluation snapshot must see the updated row.
	next := requestJSON(t, s, "/api/session/trace-0")
	if next["eval_baseline"].(map[string]any)["overall"].(map[string]any)["accuracy"].(map[string]any)["avg"] != 3.5 {
		t.Fatal("request-local timing reset the shared baseline")
	}
	if next["eval"].(map[string]any)["marks"].(map[string]any)["accuracy"] != float64(4) {
		t.Fatal("request reused stale evaluation data")
	}
	if next["timing"].(map[string]any)["queries"] != float64(3) {
		t.Fatalf("cached request repeated baseline queries: %v", next["timing"])
	}
	cached, err := s.evalBaseline("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if cached.Overall["accuracy"].Avg != 3.5 {
		t.Fatal("cache did not retain snapshot")
	}
	s.baselines.expires = time.Now().Add(-time.Second)
	fresh, err := s.evalBaseline("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Overall["accuracy"].Avg != 4.5 || fresh.Project["accuracy"].Avg != 4 {
		t.Fatalf("refresh: %+v", fresh)
	}
}

// slowCorpus blocks the corpus-wide scans the baseline needs, as a large
// archive would, while single-session reads stay fast.
type slowCorpus struct {
	*fakeReader
	release chan struct{}
}

func (s *slowCorpus) ListSessionRows(n int, filter any) ([]trace.SessionRow, error) {
	if filter == nil {
		<-s.release
	}
	return s.fakeReader.ListSessionRows(n, filter)
}
func (s *slowCorpus) ListEvalRows(any) ([]layer.EvalRow, error) { return nil, nil }
func (s *slowCorpus) SearchEvals(string, int, any) ([]layer.Hit, error) {
	return nil, nil
}

func TestSessionOpensWhileBaselineScanIsSlow(t *testing.T) {
	old := baselineWait
	baselineWait = 50 * time.Millisecond
	defer func() { baselineWait = old }()
	f := &slowCorpus{fakeReader: &fakeReader{sessions: []trace.SessionRow{{ID: "s1", SessionID: "s1", RepoURL: "https://github.com/example/alpha"}}}, release: make(chan struct{})}
	s := New(f)
	var release sync.Once
	defer release.Do(func() { close(f.release) })
	opened := make(chan error, 1)
	go func() {
		defer close(opened)
		for i := 0; i < 3; i++ { // concurrent opens must not queue behind the scan
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/session/s1", nil))
			if w.Code != 200 {
				opened <- fmt.Errorf("status %d: %s", w.Code, w.Body)
				return
			}
			var data map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				opened <- err
				return
			}
			if v, ok := data["eval_baseline"]; !ok || v != nil {
				opened <- fmt.Errorf("pending baseline should be null, got %v", v)
				return
			}
		}
	}()
	select {
	case err := <-opened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("session waited on the corpus scan")
	}
	// Release only after all opens returned with a pending baseline.
	release.Do(func() { close(f.release) })
	got, err := s.evalBaselineWithin("alpha", 5*time.Second)
	if err != nil || got.Overall == nil {
		t.Fatalf("refresh did not complete in background: %+v %v", got, err)
	}
}
