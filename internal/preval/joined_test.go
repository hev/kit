package preval_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/filearchive"
	p "github.com/hev/kit/internal/preval"
	"github.com/hev/kit/internal/trace"
)

type joinedHTTP struct {
	hosted  string
	tls     *http.Transport
	calls   *atomic.Int32
	outcome *string
}

func (j joinedHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasPrefix(r.URL.String(), j.hosted+"/") {
		return j.tls.RoundTrip(r)
	}
	w := httptest.NewRecorder()
	switch {
	case r.URL.Host == "api.github.com" && r.URL.Path == "/repos/synthetic/repo/pulls/9" && r.Header.Get("Authorization") == "Bearer api-fixture":
		state := "closed"
		if *j.outcome == p.Active {
			state = "open"
		}
		fmt.Fprintf(w, `{"number":9,"state":%q,"merged":%t,"updated_at":%q,"head":{"sha":"edit"},"merge_commit_sha":"merged","base":{"repo":{"id":42,"owner":{"login":"synthetic"}}}}`, state, *j.outcome == p.Merged, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	case r.URL.Host == "dev.azure.com" && r.URL.Path == "/synthetic/test-project/_apis/git/repositories/repo-id/pullRequests/9":
		user, pass, ok := r.BasicAuth()
		if !ok || user != "" || pass != "api-fixture" {
			return nil, errors.New("bad synthetic provider authentication")
		}
		status := map[string]string{p.Active: "active", p.Closed: "abandoned", p.Merged: "completed"}[*j.outcome]
		fmt.Fprintf(w, `{"pullRequestId":9,"status":%q,"repository":{"id":"repo-id","project":{"id":"test-project"}},"lastMergeSourceCommit":{"commitId":"edit"},"lastMergeCommit":{"commitId":"merged"}}`, status)
	case r.URL.Host == "dev.azure.com" && (strings.HasSuffix(r.URL.Path, "/iterations") || strings.HasSuffix(r.URL.Path, "/threads")):
		fmt.Fprint(w, `{"count":0,"value":[]}`)
		return w.Result(), nil
	default:
		return nil, errors.New("unexpected network request")
	}
	j.calls.Add(1)
	return w.Result(), nil
}

type joinedLocal struct {
	archive  *filearchive.Store
	evidence *evidence.Store
	fail     bool
}

func (d *joinedLocal) ReadOrder(_ context.Context, v p.Provenance) (*p.Evaluation, error) {
	return dispatch.ReadEvaluation("hev", v.OrderID, v.AttemptID)
}
func (d *joinedLocal) WriteOrder(_ context.Context, e p.Evaluation) error {
	return dispatch.SetEvaluation("hev", e)
}
func (d *joinedLocal) ClaimLearning(_ context.Context, e p.Evaluation) (bool, error) {
	return dispatch.ClaimEvaluationLearning("hev", e)
}
func (d *joinedLocal) WriteTrace(_ context.Context, id string, e p.Evaluation) error {
	return d.archive.SetEvaluation(id, e)
}
func (d *joinedLocal) WriteEvidence(_ context.Context, id string, e p.Evaluation) error {
	if d.fail && e.Outcome == p.Merged {
		d.fail = false
		return errors.New("partial write fixture")
	}
	return d.evidence.SetEvaluation(id, e)
}

type receipt struct {
	URL, Cert, Writer, Foreign, Unenrolled string
	Docs                                   map[string]map[string]any
}

func TestJoinedPullRequestEvaluation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var r receipt
	tlsTransport := http.DefaultTransport.(*http.Transport).Clone()
	tlsTransport.Proxy = nil
	if ready := os.Getenv("JOINED_READY"); ready != "" {
		body, err := os.ReadFile(ready)
		if err != nil || json.Unmarshal(body, &r) != nil || r.URL == "" {
			t.Fatalf("hosted fixture receipt unavailable: %v", err)
		}
		tlsTransport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		tlsTransport.TLSClientConfig.RootCAs = x509.NewCertPool()
		if !tlsTransport.TLSClientConfig.RootCAs.AppendCertsFromPEM([]byte(r.Cert)) {
			t.Fatal("fixture PEM")
		}
	}
	var calls atomic.Int32
	transport := http.DefaultTransport
	outcome := p.Active
	http.DefaultTransport = joinedHTTP{r.URL, tlsTransport, &calls, &outcome}
	defer func() { http.DefaultTransport = transport }()
	defer tlsTransport.CloseIdleConnections()
	hosted := p.HostedDocuments{BaseURL: r.URL, OrderPath: "/v1/orders", TracePath: "/v1/traces/sessions", EvidencePath: "/v1/evidence", Token: r.Writer}
	get := func(path, token string, status int) map[string]any {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if err != nil || res.StatusCode != status {
			t.Fatalf("GET %s: %d want %d %s (%v)", path, res.StatusCode, status, data, err)
		}
		var v map[string]any
		_ = json.Unmarshal(data, &v)
		return v
	}
	for _, provider := range []string{p.GitHub, p.AzureDevOps} {
		t.Run(provider, func(t *testing.T) {
			label := strings.ReplaceAll(provider, "_", "-")
			oid, sid, aid := label+"-order", label+"-session", label+"-attempt"
			repo := p.Repository{Provider: provider, Organization: "synthetic", Repository: "42"}
			if provider == p.AzureDevOps {
				repo.Project, repo.Repository = "test-project", "repo-id"
			}
			pr := p.PullRequest{Repository: repo, ID: 9}
			providerConfig := p.Provider{Repository: repo, Name: "repo", HookSecret: "hook-fixture", HookUser: "service", Token: "api-fixture"}
			for _, mode := range []string{"local", "hosted"} {
				t.Run(mode, func(t *testing.T) {
					if mode == "hosted" && r.URL == "" {
						t.Skip("run with the kit-cloud joined fixture for hosted coverage")
					}
					outcome = p.Active
					var docs p.Documents
					var evidenceID string
					var local *joinedLocal
					var initial *evidence.Package
					var initialHosted map[string]any
					if mode == "local" {
						root := t.TempDir()
						t.Setenv("HEV_ORDERS_DIR", root)
						if err := os.MkdirAll(filepath.Join(root, oid), 0700); err != nil {
							t.Fatal(err)
						}
						if err := dispatch.WriteJSON(filepath.Join(root, oid, "meta.json"), map[string]any{"workspace_name": "hev"}); err != nil {
							t.Fatal(err)
						}
						if err := dispatch.WriteJSON(filepath.Join(root, oid, "result.json"), map[string]any{"span_id": aid, "end": time.Now(), "extension": "original order data"}); err != nil {
							t.Fatal(err)
						}
						ar, err := filearchive.Open(t.TempDir(), "joined")
						if err != nil {
							t.Fatal(err)
						}
						if _, err := ar.WriteSessions([]trace.SessionRow{{ID: sid, SessionID: sid, Order: oid, ParentSpanID: aid, Summary: "original trace data"}}); err != nil {
							t.Fatal(err)
						}
						ev, err := evidence.Open(t.TempDir())
						if err != nil {
							t.Fatal(err)
						}
						initial, err = ev.Put(evidence.Meta{Title: "Proof", Order: oid, Session: sid}, []evidence.ItemInput{{Name: "proof.txt", Data: []byte("original sealed bytes")}}, nil)
						if err != nil {
							t.Fatal(err)
						}
						evidenceID = initial.ID
						local = &joinedLocal{ar, ev, true}
						docs = local
					} else {
						initialHosted = r.Docs[provider]
						evidenceID = initialHosted["id"].(string)
						docs = hosted
					}
					provenance := p.Provenance{OrderID: oid, AttemptID: aid, TraceSessionIDs: []string{sid}, EvidenceIDs: []string{evidenceID}}
					registration := p.Registration{Attribution: p.Attribution{PullRequest: pr, Provenance: provenance}}
					create := func() *p.Reconciler {
						t.Helper()
						result, err := p.NewReconciler([]p.Provider{providerConfig}, []p.Registration{registration}, docs, time.Hour)
						if err != nil {
							t.Fatal(err)
						}
						return result
					}
					send := func(s *p.Reconciler, auth bool, status int) {
						t.Helper()
						b := `{"action":"closed","number":9,"repository":{"id":42,"owner":{"login":"synthetic"}},"pull_request":{"number":9,"base":{"repo":{"id":42,"owner":{"login":"synthetic"}}}}}`
						if provider == p.AzureDevOps {
							b = `{"id":"ado-fixture","publisherId":"tfs","eventType":"git.pullrequest.updated","resource":{"pullRequestId":9,"repository":{"id":"repo-id","project":{"id":"test-project"}}}}`
						}
						req := httptest.NewRequest(http.MethodPost, "/hooks/0", strings.NewReader(b))
						req.Header.Set("X-GitHub-Event", "pull_request")
						if auth && provider == p.GitHub {
							mac := hmac.New(sha256.New, []byte(providerConfig.HookSecret))
							mac.Write([]byte(b))
							req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
						} else if auth {
							req.SetBasicAuth(providerConfig.HookUser, providerConfig.HookSecret)
						}
						res := httptest.NewRecorder()
						s.Handler().ServeHTTP(res, req)
						if res.Code != status {
							t.Fatalf("hook %d want %d", res.Code, status)
						}
					}
					check := func(expected string, score *float64) *p.Evaluation {
						t.Helper()
						e, err := docs.ReadOrder(ctx, provenance)
						if err != nil || e == nil || e.Outcome != expected || !reflect.DeepEqual(e.Score, score) {
							t.Fatalf("unexpected order result: %+v %v", e, err)
						}
						return e
					}
					service := create()
					priorCalls := calls.Load()
					send(service, false, 401)
					if calls.Load() != priorCalls {
						t.Fatal("unauthenticated hook prompted lookup")
					}
					for _, state := range []string{p.Active, p.Closed, p.Active} {
						outcome = state
						send(service, true, 204)
						var score *float64
						if state == p.Closed {
							value := -1.0
							score = &value
						}
						expected := check(state, score)
						var tr, proof *p.Evaluation
						if mode == "local" {
							tr, _, _ = local.archive.ReadEvaluation(sid)
							doc, err := local.evidence.Load(evidenceID)
							if err != nil {
								t.Fatal(err)
							}
							proof = doc.Evaluation
						} else {
							for _, target := range []struct {
								path string
								eval **p.Evaluation
							}{
								{"/v1/traces/sessions/" + sid, &tr}, {"/v1/evidence/" + evidenceID, &proof},
							} {
								doc := get(target.path, r.Writer, 200)
								raw, err := json.Marshal(doc["evaluation"])
								if err != nil {
									t.Fatal(err)
								}
								if err := json.Unmarshal(raw, target.eval); err != nil {
									t.Fatal(err)
								}
							}
						}
						if !reflect.DeepEqual(tr, expected) || !reflect.DeepEqual(proof, expected) {
							t.Fatalf("outcome %s lost during propagation", state)
						}
					}
					previous, _ := docs.ReadOrder(ctx, provenance)
					outcome = p.Merged
					send(service, true, 503)
					if calls.Load() != priorCalls+4 {
						t.Fatal("missing authoritative lookup")
					}
					positive := 1.0
					first := check(p.Merged, &positive)
					evPath := "/v1/evidence/" + evidenceID
					if mode == "local" {
						before, err := local.evidence.Load(evidenceID)
						if err != nil || !reflect.DeepEqual(before.Evaluation, previous) {
							t.Fatal("local partial write fixture did not hold")
						}
					} else {
						got := get(evPath, r.Writer, 200)["evaluation"].(map[string]any)
						if got["outcome"] != p.Active || got["score"] != nil {
							t.Fatal("hosted partial write fixture did not hold")
						}
					}
					send(create(), true, 204)
					send(create(), true, 204)
					if got := check(p.Merged, &positive); !reflect.DeepEqual(first, got) {
						t.Fatal("replay changed evaluation")
					}
					if mode == "local" {
						current, err := local.evidence.Verify(evidenceID)
						if err != nil || current.Evaluation.ID != first.ID {
							t.Fatalf("sealed local proof: %v", err)
						}
						current.Evaluation = nil
						if !reflect.DeepEqual(initial, current) {
							t.Fatal("sealed local manifest or hashes changed")
						}
						tr, _, err := local.archive.ReadEvaluation(sid)
						if err != nil || tr.ID != first.ID {
							t.Fatalf("trace eval not repaired: %v", err)
						}
						body, err := os.ReadFile(filepath.Join(dispatch.Root(), oid, "result.json"))
						if err != nil || !strings.Contains(string(body), "original order data") {
							t.Fatal("original order data lost")
						}
					} else {
						for _, path := range []string{"/v1/orders/" + oid, "/v1/traces/sessions/" + sid, evPath} {
							record := get(path, r.Writer, 200)
							ev, ok := record["evaluation"].(map[string]any)
							if !ok || ev["evaluation_id"] != first.ID || ev["score"] != float64(1) {
								t.Fatalf("lost joined projection %s", path)
							}
							get(path, r.Foreign, 404)
						}
						sources, err := hosted.LearningMaterial(ctx, *first)
						if err != nil || len(sources) != 3 || !strings.Contains(sources[1].Text, "synthetic assistant finding") ||
							!strings.Contains(sources[1].Text, "synthetic tool result") || strings.Contains(sources[1].Text, "/Users/fixture") {
							t.Fatalf("hosted transcript was missing or unsafe: %v", err)
						}
						current := get(evPath, r.Writer, 200)
						for _, field := range []string{"content_sha256", "sealed_at", "items", "title", "state"} {
							if !reflect.DeepEqual(initialHosted[field], current[field]) {
								t.Fatalf("sealed hosted %s changed", field)
							}
						}
						if current["content_sha256"] == "" || current["sealed_at"] == "" {
							t.Fatal("missing seal or digest")
						}
						req, err := http.NewRequestWithContext(ctx, http.MethodPut, r.URL+evPath+"/evaluation", strings.NewReader("{}"))
						if err != nil {
							t.Fatal(err)
						}
						req.Header.Set("Authorization", "Bearer "+r.Unenrolled)
						res, err := http.DefaultClient.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						res.Body.Close()
						if res.StatusCode != 403 {
							t.Fatalf("unenrolled update %d", res.StatusCode)
						}
					}
					t.Logf("provider=%s storage=%s projection=%s lookup+repair+replay verified; sealed integrity preserved", provider, mode, first.ID)
				})
			}
		})
	}
}
