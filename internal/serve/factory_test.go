package serve

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/cost"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func factoryFixture(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	now := time.Now().Add(-time.Minute).UnixMilli()
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("prices.toml", []byte("[[rates]]\nmodel='fixture-model'\neffective='2020-01-01'\ninput=2\ncached=1\noutput=4\n"))
	write("subscriptions.toml", []byte("[[plans]]\nname='Fixture subscription'\nmonthly_usd=100\nreset_day=1\nharness='codex'\nweekly_token_basis=1000000\n"))
	rows := []cost.Row{
		{ID: "worker", Harness: "codex", Model: "fixture-model", Instance: "example", Role: "worker", Issue: "EX-1", Repo: "example/repo", Start: now, End: now, Usage: map[string]cost.Usage{"fixture-model": {Input: 100000, Output: 20000}}},
		{ID: "gaffer", Harness: "codex", Model: "fixture-model", Instance: "example", Role: "gaffer", Start: now, End: now, Usage: map[string]cost.Usage{"fixture-model": {Input: 20000, Output: 1000}}},
		{ID: "promo", Harness: "codex", Model: "fixture-model", Role: "other", Start: now, End: now, Usage: map[string]cost.Usage{"fixture-model": {Input: 30000}}},
	}
	var lines []byte
	for _, r := range rows {
		b, _ := json.Marshal(r)
		lines = append(lines, append(b, '\n')...)
	}
	write("sessions.jsonl", lines)
	write("beats.jsonl", []byte(fmt.Sprintf("{\"instance\":\"example\",\"session_id\":\"gaffer\",\"timestamp\":%d}\n", now)))
	outcomes := cost.Outcomes{Team: "example", RefreshedAt: now, Issues: []cost.Outcome{{Issue: "EX-1", Team: "example", State: "Done", DoneAt: now, PRs: []cost.PR{{URL: "https://example.com/pr/1", Repo: "example/repo", MergedAt: now}}, Deploys: []cost.Deploy{{ID: "deploy-1", Repo: "example/repo", LandedAt: now}}}}}
	outcomes.PRs = []cost.PR{{URL: "https://example.com/pr/1", Repo: "example/repo", MergedAt: now}, {URL: "https://example.com/pr/2", Repo: "example/repo", MergedAt: now}}
	outcomes.Deploys = []cost.Deploy{{ID: "deploy-1", Repo: "example/repo", LandedAt: now}, {ID: "unassociated", Repo: "example/repo", LandedAt: now}}
	b, _ := json.Marshal(outcomes)
	write("outcomes.json", b)
	c := ResolveFactoryPaths(dir)
	c.Team = "example"
	c.Repos = []string{"example/repo"}
	return New(nil).WithFactory(c)
}
func TestFactoryAccountingAPI(t *testing.T) {
	s := factoryFixture(t)
	get := func(query string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/factory?"+query, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	all := get("window=7d")
	filtered := get("window=7d&role=worker")
	total := all["total"].(map[string]any)
	if total["sessions"] != float64(3) || total["beats"] != float64(1) {
		t.Fatal(total)
	}
	if all["shipping"].(map[string]any)["prs_merged"] != float64(2) || filtered["shipping"].(map[string]any)["prs_merged"] != float64(1) {
		t.Fatal(all["shipping"])
	}
	if all["shipping"].(map[string]any)["deploys_landed"] != float64(2) || filtered["shipping"].(map[string]any)["deploys_landed"] != float64(1) {
		t.Fatal("unassociated deploy missing, double counted, or attributed to worker filter")
	}
	rows := all["sessions"].([]any)
	var workerSub float64
	for _, r := range rows {
		row := r.(map[string]any)
		if row["session_id"] == "worker" {
			workerSub = row["sub_usd"].(float64)
		}
	}
	if filtered["total"].(map[string]any)["sub_usd"] != workerSub {
		t.Fatal("filter changed allocation")
	}
}
func TestFactoryFixtureServer(t *testing.T) {
	addr := os.Getenv("HEV_FACTORY_FIXTURE_LISTEN")
	if addr == "" {
		t.Skip("optional browser fixture")
	}
	t.Log("synthetic accounting fixture only:", addr)
	if err := http.ListenAndServe(addr, factoryFixture(t).Handler()); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryPlaceholderCoverage(t *testing.T) {
	s := factoryFixture(t)
	c := s.factoryConfig
	if err := os.WriteFile(c.Subscriptions, []byte("[[plans]]\nname='Pending'\nharness='codex'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(c.Rows, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	row := cost.Row{ID: "missing", Harness: "codex", Role: "worker", Instance: "example", Start: time.Now().Add(-time.Minute).UnixMilli(), End: time.Now().UnixMilli()}
	if err := json.NewEncoder(f).Encode(row); err != nil {
		t.Fatal(err)
	}
	f.Close()
	get := func() map[string]any { return requestJSON(t, s, "/api/factory?window=7d") }
	v := get()
	if v["incomplete_sessions"] != float64(1) {
		t.Fatal(v["incomplete_sessions"])
	}
	plans := v["plans"].([]any)
	if plans[0].(map[string]any)["monthly_usd"] != nil || plans[0].(map[string]any)["tier"] != "" {
		t.Fatal(plans)
	}
	if v["total"].(map[string]any)["unallocated"] != float64(4) {
		t.Fatal(v["total"])
	}
	// Editing the plain file is picked up without a restart; no invented default price/tier.
	if err := os.WriteFile(c.Subscriptions, []byte("[[plans]]\nname='Pending'\nharness='codex'\nmonthly_usd=100\ntier='operator fixture'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v = get()
	if v["total"].(map[string]any)["unallocated"] != float64(0) || v["plans"].([]any)[0].(map[string]any)["tier"] != "operator fixture" {
		t.Fatal(v["total"])
	}
}
