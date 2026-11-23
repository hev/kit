package preval

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixtureDocuments struct {
	order          *Evaluation
	evidence       *Evaluation
	refuseEvidence bool
}

func (d *fixtureDocuments) ReadOrder(context.Context, Provenance) (*Evaluation, error) {
	return d.order, nil
}

func (d *fixtureDocuments) WriteOrder(_ context.Context, e Evaluation) error {
	d.order = &e
	return nil
}

func (d *fixtureDocuments) ClaimLearning(_ context.Context, e Evaluation) (bool, error) {
	if d.order == nil || d.order.Learning.Status != "pending" {
		return false, nil
	}
	d.order = &e
	return true, nil
}

func (d *fixtureDocuments) WriteTrace(context.Context, string, Evaluation) error {
	return nil
}

func (d *fixtureDocuments) WriteEvidence(_ context.Context, _ string, e Evaluation) error {
	if d.refuseEvidence {
		return errors.New("fixture evidence unavailable")
	}
	d.evidence = &e
	return nil
}

func TestHookReconcilesOnlyAuthorizedAttributedWork(t *testing.T) {
	for _, provider := range []string{GitHub, AzureDevOps} {
		t.Run(provider, func(t *testing.T) {
			repo := Repository{Provider: provider, Organization: "synthetic", Repository: "42"}
			if provider == AzureDevOps {
				repo.Project = "test-project"
			}
			pr := PullRequest{Repository: repo, ID: 1}
			state := Active
			apiAvailable := true
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !apiAvailable {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if strings.Contains(r.URL.Path, "/iterations") || strings.Contains(r.URL.Path, "/threads") {
					fmt.Fprint(w, `{"count":0,"value":[]}`)
					return
				}
				switch provider {
				case GitHub:
					actual := "closed"
					if state == Active {
						actual = "open"
					}
					fmt.Fprintf(w, `{"number":1,"state":%q,"merged":%t,"updated_at":%q,"head":{"sha":"abc"},"merge_commit_sha":"def","base":{"repo":{"id":42,"owner":{"login":"synthetic"}}}}`,
						actual, state == Merged, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
				case AzureDevOps:
					actual := map[string]string{Active: "active", Merged: "completed", Closed: "abandoned"}[state]
					fmt.Fprintf(w, `{"pullRequestId":1,"status":%q,"repository":{"id":"42","project":{"id":"test-project"}},"lastMergeSourceCommit":{"commitId":"abc"},"lastMergeCommit":{"commitId":"def"}}`, actual)
				}
			}))
			defer api.Close()
			p := Provider{Repository: repo, Name: "kit", HookSecret: "synthetic-hook", HookUser: "synthetic-user", Token: "synthetic-api", apiBase: api.URL, client: api.Client()}
			docs := &fixtureDocuments{}
			links := []Registration{{Attribution: Attribution{PullRequest: pr, Provenance: Provenance{OrderID: "order", AttemptID: "attempt", EvidenceIDs: []string{"evidence"}}}}}
			newReconciler := func() *Reconciler {
				service, err := NewReconciler([]Provider{p}, links, docs, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				return service
			}
			service := newReconciler()
			deliver := func(number int, repoID string, auth bool, expected int) {
				t.Helper()
				var body string
				if provider == GitHub {
					body = fmt.Sprintf(`{"action":"closed","number":%d,"repository":{"id":%s,"owner":{"login":"synthetic"}},"pull_request":{"number":%d,"base":{"repo":{"id":%s,"owner":{"login":"synthetic"}}}}}`, number, repoID, number, repoID)
				} else {
					body = fmt.Sprintf(`{"id":"fixture","publisherId":"tfs","eventType":"git.pullrequest.updated","resource":{"pullRequestId":%d,"repository":{"id":%q,"project":{"id":"test-project"}}}}`, number, repoID)
				}
				request := httptest.NewRequest("POST", "/hooks/0", strings.NewReader(body))
				request.Header.Set("X-GitHub-Event", "pull_request")
				if auth {
					mac := hmac.New(sha256.New, []byte(p.HookSecret))
					mac.Write([]byte(body))
					request.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
					request.SetBasicAuth(p.HookUser, p.HookSecret)
				}
				w := httptest.NewRecorder()
				service.Handler().ServeHTTP(w, request)
				if w.Code != expected {
					t.Fatalf("status %d, want %d", w.Code, expected)
				}
			}
			deliver(1, "42", false, 401)
			deliver(1, "43", true, 403)
			deliver(0, "42", true, 400)
			deliver(2, "42", true, 204)
			if docs.order != nil {
				t.Fatal("unrelated or refused event evaluated an order")
			}
			deliver(1, "42", true, 204)
			if docs.order.Score != nil || docs.order.Outcome != Active {
				t.Fatalf("active pull request was scored: %+v", docs.order)
			}
			state = Closed
			deliver(1, "42", true, 204)
			if docs.order.Outcome != Closed || *docs.order.Score != -1 {
				t.Fatal("closed PR was not rejected")
			}
			state = Active
			if _, err := service.Sweep(context.Background(), 0, 1); err != nil || docs.order.Outcome != Active || docs.order.Score != nil {
				t.Fatalf("reactivation failed: %v", err)
			}
			state, apiAvailable = Merged, false
			deliver(1, "42", true, 503)
			if docs.order.Outcome != Active {
				t.Fatal("provider failure was treated as evidence")
			}
			apiAvailable, docs.refuseEvidence = true, true
			deliver(1, "42", true, 503)
			if docs.order.Outcome != Merged || *docs.order.Score != 1 || docs.evidence.Outcome != Active {
				t.Fatal("partial write did not retain deterministic outcome")
			}
			before, _ := json.Marshal(docs.order)
			docs.refuseEvidence, service = false, newReconciler()
			deliver(1, "42", true, 204)
			after, _ := json.Marshal(docs.evidence)
			if string(after) != string(before) {
				t.Fatal("replay failed to repair missing projection exactly")
			}
			state = Closed
			deliver(1, "42", true, 204)
			if docs.order.Outcome != Merged {
				t.Fatal("merged outcome downgraded without explicit correction")
			}
		})
	}
}

func TestStaleAndCorrectionRequireEvidence(t *testing.T) {
	pr := PullRequest{Repository: Repository{Provider: GitHub, Organization: "org", Repository: "42"}, ID: 1}
	t0 := time.Now().UTC()
	observation := Observation{Source: Source{PullRequest: pr, Revision: "abc", EventID: "event"}, ObservedAt: t0, LastActivity: t0.Add(-time.Hour), Outcome: Active}
	link := Attribution{PullRequest: pr, Provenance: Provenance{OrderID: "order", AttemptID: "attempt"}}
	eval, err := Project(link, observation, nil, time.Hour, false)
	if err != nil || eval.Outcome != Stale || *eval.Score != -0.25 {
		t.Fatalf("stale transition failed: %v", err)
	}
	observation.ObservedAt = t0.Add(time.Minute)
	observation.LastActivity = observation.ObservedAt
	active, err := Project(link, observation, &eval, time.Hour, false)
	if err != nil || active.Outcome != Active || active.Score != nil {
		t.Fatalf("activity failed to clear stale: %v", err)
	}
	observation.Outcome = Merged
	merged, err := Project(link, observation, nil, 0, false)
	if err != nil || merged.Outcome != Merged {
		t.Fatalf("merge failed: %v", err)
	}
	later := observation
	later.ObservedAt = later.ObservedAt.Add(time.Minute)
	later.Source.Revision = "later_edit"
	ordinary, err := Project(link, later, &merged, 0, false)
	if err != nil || ordinary.Outcome != Merged {
		t.Fatalf("ordinary edit penalized merge: %v", err)
	}
	later.Outcome, later.Source.ID = Reverted, 2
	link.Correction = &later
	observation.ObservedAt = later.ObservedAt.Add(time.Second)
	reverted, err := Project(link, observation, &merged, 0, false)
	if err != nil || reverted.Outcome != Reverted || reverted.ID != merged.ID || reverted.Source.ID != 2 {
		t.Fatalf("explicit revert failed: %v", err)
	}
	other := observation
	other.Source.ID = 3
	if _, err := Project(link, other, &merged, 0, false); err == nil {
		t.Fatal("correction applied to an unrelated PR")
	}
}
