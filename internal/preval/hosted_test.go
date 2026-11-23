package preval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHostedLearningClaimUsesOneConditionalWrite(t *testing.T) {
	pr := PullRequest{Repository: Repository{Provider: AzureDevOps, Organization: "sample", Project: "project", Repository: "repository"}, ID: 42}
	p := Provenance{OrderID: "order", AttemptID: "attempt"}
	evaluation, err := Project(Attribution{PullRequest: pr, Provenance: p},
		Observation{Source: Source{PullRequest: pr, Revision: "commit", EventID: "event"}, ObservedAt: time.Now(), Outcome: Merged}, nil, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	revision := 0
	var puts int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-tenant-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		etag := fmt.Sprintf(`"%d"`, revision)
		if r.Method == http.MethodGet && r.URL.Path == "/v1/orders/order" {
			w.Header().Set("ETag", etag)
			_ = json.NewEncoder(w).Encode(map[string]any{"evaluation": evaluation, "order": "Original acceptance rules."})
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/orders/order/evaluation" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("If-Match") != etag {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		var candidate Evaluation
		if json.NewDecoder(r.Body).Decode(&candidate) != nil || !candidate.Valid() {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		evaluation = candidate
		revision++
		puts++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	document := HostedDocuments{BaseURL: server.URL, OrderPath: "/v1/orders", Token: "synthetic-tenant-key", client: server.Client()}
	claim := evaluation
	claim.Learning = Learning{Status: "failed"}
	type result struct {
		claimed bool
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			ok, err := document.ClaimLearning(context.Background(), claim)
			results <- result{ok, err}
		}()
	}
	winners := 0
	for range 2 {
		value := <-results
		if value.err != nil {
			t.Fatal(value.err)
		}
		if value.claimed {
			winners++
		}
	}
	mu.Lock()
	applied := puts
	mu.Unlock()
	if winners != 1 || applied != 1 {
		t.Fatalf("claims: %d, writes: %d", winners, applied)
	}
	excerpts, err := document.LearningMaterial(context.Background(), claim)
	if err != nil || len(excerpts) != 1 || excerpts[0].Text != "Original acceptance rules.\n" {
		t.Fatalf("could not read sources through tenant scope: %+v %v", excerpts, err)
	}
	document.Token = "synthetic-unenrolled-key"
	if _, err := document.LearningMaterial(context.Background(), claim); err == nil {
		t.Fatal("an unauthorized tenant read source contents")
	}
}
