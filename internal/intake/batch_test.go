package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const batchTemplate = "---\nname: {{name}}\nrepo: example\n---\n{{status}}: {{count}}\n{{identifiers}}\n{{urls}}\n## Acceptance\n- Done\n"

func batchFixture(t *testing.T) *fixture {
	t.Helper()
	f := setup(t)
	f.live(t)
	r := &f.p.cfg.Status[0]
	r.Batch = true
	if err := os.WriteFile(r.Template, []byte(batchTemplate), 0600); err != nil {
		t.Fatal(err)
	}
	for j := 2; j <= 3; j++ {
		i := f.issue()
		i.ID = fmt.Sprint(j)
		i.Identifier = fmt.Sprintf("TEST-%d", j)
		i.URL = "https://example.invalid/" + i.Identifier
		f.extraIssues = append(f.extraIssues, i)
	}
	return f
}
func batchDecisions(t *testing.T, f *fixture) []Decision {
	t.Helper()
	var ds []Decision
	for _, line := range strings.Split(strings.TrimSpace(f.out.String()), "\n") {
		var d Decision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatal(err)
		}
		ds = append(ds, d)
	}
	return ds
}
func TestBatchThreeTicketsOneDispatch(t *testing.T) {
	f := batchFixture(t)
	f.poll(t)
	ds := batchDecisions(t, f)
	if len(ds) != 4 || ds[3].Decision != "dispatched" || strings.Join(ds[3].Keys, ",") != "TEST-1,TEST-2,TEST-3" || f.p.day().Dispatches != 1 || f.calls != 4 {
		t.Fatal(ds, f.calls)
	}
	b, err := os.ReadFile(os.Getenv("INTAKE_CAPTURE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range ds[3].Keys {
		if !strings.Contains(string(b), key) {
			t.Fatal(string(b))
		}
	}
}
func TestBatchDeniedTicketsDoNotBlock(t *testing.T) {
	for _, reason := range []string{"no human authorizer", "assigning actor outside allowed team"} {
		t.Run(reason, func(t *testing.T) {
			f := batchFixture(t)
			i := f.extraIssues[0]
			if reason == "no human authorizer" {
				f.assignmentBot = true
			} else {
				f.guest = true
			}
			cached, err := f.p.history(context.Background(), i, "bot-id")
			if err != nil {
				t.Fatal(err)
			}
			f.p.state.Cache[i.ID] = cached
			f.assignmentBot = false
			f.guest = false
			reason = cached.Reason
			f.poll(t)
			ds := batchDecisions(t, f)
			if len(ds) != 4 || ds[1].Reason != reason || len(ds[3].Keys) != 2 || ds[3].Decision != "dispatched" {
				t.Fatal(ds)
			}
		})
	}
}
func TestBatchActiveCooldownChangedAndNewTickets(t *testing.T) {
	f := batchFixture(t)
	f.p.cfg.DailyCap = 10
	f.poll(t)
	name := f.p.state.Batches[0].Name
	// Reload durable state before checking the active run.
	p, err := newPoller(f.p.cfg, "fake-only", f.out)
	if err != nil {
		t.Fatal(err)
	}
	p.now = f.p.now
	p.endpoint = f.p.endpoint
	f.p = p
	t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"running"}]}}`, name))
	i := f.issue()
	i.ID = "4"
	i.Identifier = "TEST-4"
	i.URL = "https://example.invalid/TEST-4"
	f.extraIssues = append(f.extraIssues, i)
	f.out.Reset()
	f.poll(t)
	if f.p.day().Dispatches != 1 || !strings.Contains(f.out.String(), "batch active") {
		t.Fatal(f.out.String())
	}
	t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"done"}]}}`, name))
	f.extraIssues = f.extraIssues[:2]
	f.out.Reset()
	f.poll(t)
	if f.p.day().Dispatches != 1 || !strings.Contains(f.out.String(), "batch cooldown") {
		t.Fatal(f.out.String())
	}
	f.extraIssues = append(f.extraIssues, i)
	f.out.Reset()
	f.poll(t)
	if f.p.day().Dispatches != 2 {
		t.Fatal(f.out.String())
	}
	now := f.p.now()
	f.p.now = func() time.Time { return now.Add(24 * time.Hour) }
	f.out.Reset()
	f.poll(t)
	if f.p.day().Dispatches != 1 || len(f.p.state.Batches) != 3 {
		t.Fatal(f.out.String())
	}
}
func TestBatchPauseCapAndCodeLimit(t *testing.T) {
	for _, gate := range []string{"paused", "daily cap", "code-job limit"} {
		t.Run(gate, func(t *testing.T) {
			f := batchFixture(t)
			switch gate {
			case "paused":
				f.p.cfg.PauseFile = f.p.cfg.StateDir + "/PAUSED"
				if err := os.WriteFile(f.p.cfg.PauseFile, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "daily cap":
				f.p.cfg.DailyCap = 0
			case "code-job limit":
				f.p.cfg.Status[0].Concurrency = "code"
				f.p.cfg.Limits.CodeJobs = 0
			}
			f.poll(t)
			if f.p.day().Dispatches != 0 || !strings.Contains(f.out.String(), gate) {
				t.Fatal(f.out.String())
			}
		})
	}
}
func TestBatchRenderAndCheck(t *testing.T) {
	config, _, tpl := previewFixture(t, batchTemplate)
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config, append(b, []byte("batch = true\nbatch_cooldown = '2h'\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"linear", "render", "--config", config, "--status", "Ready", "--identifier", "TEST-2", "--url", "url2", "--identifier", "TEST-1", "--url", "url1"}
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "TEST-1\nTEST-2\nurl1\nurl2") {
		t.Fatal(out.String())
	}
	if err := run([]string{"linear", "check", "--config", config}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"identifier", "url", "title"} {
		if err := os.WriteFile(tpl, []byte(batchTemplate+"{{"+key+"}}"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := run([]string{"linear", "check", "--config", config}, io.Discard); err == nil {
			t.Fatal(key)
		}
	}
	for _, key := range []string{"identifiers", "urls", "count"} {
		r := Rule{Template: tpl}
		if err := os.WriteFile(tpl, []byte(previewTemplate+"{{"+key+"}}"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := renderOrder(Issue{}, r, "test"); err == nil {
			t.Fatal(key)
		}
	}
}
func TestBatchOnlineRender(t *testing.T) {
	config, _, _ := previewFixture(t, batchTemplate)
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config, append(b, []byte("batch = true\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PREVIEW_TEST_KEY", "fake")
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	calls := 0
	http.DefaultTransport = previewTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var q struct{ Variables map[string]string }
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Fatal(err)
		}
		key := q.Variables["id"]
		body := fmt.Sprintf(`{"data":{"issue":{"identifier":%q,"url":%q,"state":{"name":"Ready"}}}}`, key, "https://example.invalid/"+key)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	var out bytes.Buffer
	if err := run([]string{"linear", "render", "--config", config, "--status", "Ready", "--ticket", "TEST-1", "--ticket", "TEST-2"}, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(out.String(), "TEST-1\nTEST-2") {
		t.Fatal(calls, out.String())
	}
}
func TestBatchUncertainBlocksChangedSet(t *testing.T) {
	f := batchFixture(t)
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{}}`), nil
		}
		return nil, fmt.Errorf("lost response")
	}
	f.poll(t)
	i := f.issue()
	i.ID = "new"
	i.Identifier = "TEST-4"
	i.URL = "https://example.invalid/TEST-4"
	f.extraIssues = append(f.extraIssues[:1], i)
	f.out.Reset()
	f.poll(t)
	if !strings.Contains(f.out.String(), "batch active") || f.p.day().Dispatches != 1 {
		t.Fatal(f.out.String())
	}
}

func TestBatchBadTicketLookupDoesNotBlockOthers(t *testing.T) {
	f := batchFixture(t)
	original := f.p.client.Transport
	if original == nil {
		original = http.DefaultTransport
	}
	f.p.client.Transport = previewTransport(func(r *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		r.Body = io.NopCloser(bytes.NewReader(b))
		if strings.Contains(string(b), `"id":"issue-id"`) && strings.Contains(string(b), "query History") {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"errors":[{}]}`))}, nil
		}
		return original.RoundTrip(r)
	})
	f.poll(t)
	ds := batchDecisions(t, f)
	if len(ds) != 4 || ds[0].Reason != "ticket authorization lookup failed" || ds[3].Decision != "dispatched" || len(ds[3].Keys) != 2 {
		t.Fatal(ds)
	}
}
func TestBatchTicketBackoffAndUnsafeValue(t *testing.T) {
	f := batchFixture(t)
	i := f.issue()
	name := lease(i, f.p.cfg.Status[0], f.entered)
	f.p.state.TicketFailures[name] = TicketFailure{Count: 1, Skip: 1}
	f.extraIssues[0].URL = "bad\nurl"
	f.poll(t)
	ds := batchDecisions(t, f)
	if ds[0].Reason != "ticket failure backoff" || ds[1].Reason != "template value contains control characters" || len(ds[3].Keys) != 1 || ds[3].Keys[0] != "TEST-3" {
		t.Fatal(ds)
	}
}

func TestBatchSameDayDuplicateDoesNotConsumeState(t *testing.T) {
	for _, source := range []string{"inventory", "admitted"} {
		t.Run(source, func(t *testing.T) {
			f := batchFixture(t)
			f.poll(t)
			run := f.p.state.Batches[0]
			now := f.p.now()
			f.p.now = func() time.Time { return now.Add(7 * time.Hour) }
			if source == "inventory" {
				delete(f.p.admitted, run.Name)
				t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"done"}]}}`, run.Name))
			}
			f.out.Reset()
			f.poll(t)
			if !strings.Contains(f.out.String(), "dispatch record exists") || f.p.day().Dispatches != 1 || len(f.p.state.Batches) != 1 || f.p.state.Batches[0] != run || len(f.p.state.TicketFailures) != 0 || len(f.p.state.Pending) != 0 {
				t.Fatal(f.out.String(), f.p.state)
			}
		})
	}
}

func TestBatchStalePendingDoesNotBlockOtherRule(t *testing.T) {
	f := batchFixture(t)
	now := f.p.now()
	f.p.state.Batches = append(f.p.state.Batches, BatchRun{"Other", "other-old", "OTHER-1", now})
	f.p.state.Pending["other-old"] = "read"
	f.p.state.PendingSince["other-old"] = now
	f.p.state.Failures = 2
	f.poll(t)
	if f.p.day().Dispatches != 1 || f.p.state.Failures != 0 || !strings.Contains(f.out.String(), `"decision":"dispatched"`) {
		t.Fatal(f.out.String(), f.p.state)
	}
}
