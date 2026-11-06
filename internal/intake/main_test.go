package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This test executable doubles as a real dispatch subprocess, preserving argv boundaries.
func TestDispatchHelper(t *testing.T) {
	if os.Getenv("INTAKE_HELPER") != "1" {
		return
	}
	args := os.Args
	sep := 0
	for j, a := range args {
		if a == "--" {
			sep = j + 1
			break
		}
	}
	args = args[sep:]
	if len(args) > 0 && args[0] == "ls" {
		fmt.Print(os.Getenv("INTAKE_INVENTORY"))
		os.Exit(0)
	}
	if len(args) != 3 || args[1] != "--host" || args[2] != "testhost" {
		os.Exit(9)
	}
	b, e := os.ReadFile(args[0])
	if e != nil {
		os.Exit(8)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "name: ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "name: "))
			marker := os.Getenv("INTAKE_CAPTURE") + "." + name
			file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				fmt.Fprintln(os.Stderr, "name already exists")
				os.Exit(1)
			}
			_ = file.Close()
		}
	}
	_ = os.WriteFile(os.Getenv("INTAKE_CAPTURE"), append([]byte(args[0]+"\n"), b...), 0600)
	if os.Getenv("FAKE_LINEAR_KEY") != "" {
		os.Exit(7)
	}
	os.Exit(0)
}

type fixture struct {
	stoppedHandler func(http.ResponseWriter, string)

	guest, transitionGuest                 bool
	assignmentBot, transitionBot, actorApp bool
	memberApp                              bool
	inactive, memberInactive               bool
	botActor, creatorBotActor              bool
	historyNodes                           []any
	extraIssues                            []Issue
	historyPage                            bool
	repeatHistoryCursor                    bool
	inactiveMemberID                       string
	emptyHistory                           bool
	creator                                *Actor
	cursors                                []any
	p                                      *Poller
	out                                    *bytes.Buffer
	calls, histories                       int
	member                                 bool
	entered                                time.Time
	updated                                string
	status                                 int
	errorBody                              string
	transitionOutside                      bool
	transitionSystem                       bool
	historyHeaders                         bool
	noIssues                               bool
	headers                                map[string]string
	issuePage                              bool
	graphErr                               bool
	bodyQueries                            []string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{out: &bytes.Buffer{}, member: true, entered: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), updated: "v1", headers: map[string]string{}}
	c := Config{Mode: "dry-run", Interval: "15m", StateDir: t.TempDir(), DailyCap: 2, Dispatch: []string{os.Args[0], "-test.run=^TestDispatchHelper$", "--"}}
	c.Linear.APIKeyEnv = "FAKE_LINEAR_KEY"
	c.Linear.Team = "TEST"
	c.Linear.Assignee = "testbot"
	c.Linear.AllowedActorTeam = "TEST"
	c.Limits.CodeJobs = 1
	c.Limits.MinRemaining = 20
	c.Status = []Rule{{Name: "Review", Action: "triage", Mode: "dry-run", Host: "testhost", Concurrency: "read"}}
	t.Setenv("INTAKE_HELPER", "1")
	t.Setenv("INTAKE_INVENTORY", `{"local":{"rows":[]}}`)
	t.Setenv("INTAKE_CAPTURE", filepath.Join(c.StateDir, "captured"))
	t.Setenv("FAKE_LINEAR_KEY", "fake-only")
	p, e := newPoller(c, "fake-only", f.out)
	if e != nil {
		t.Fatal(e)
	}
	f.p = p
	p.now = func() time.Time { return time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls++
		if r.Header.Get("Authorization") != "fake-only" {
			t.Error("authorization missing")
		}
		for k, v := range f.headers {
			w.Header().Set(k, v)
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			fmt.Fprint(w, f.errorBody)
			return
		}
		var q struct {
			Query     string
			Variables map[string]any
		}
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
		}
		if f.stoppedHandler != nil && (strings.Contains(q.Query, "StoppedTicket") || strings.Contains(q.Query, "StoppedComment")) {
			f.stoppedHandler(w, q.Query)
			return
		}
		f.bodyQueries = append(f.bodyQueries, q.Query)
		if unknownUserField(q.Query) {
			fmt.Fprint(w, `{"errors":[{"message":"Cannot query unknown field on type User"}]}`)
			return
		}
		if f.graphErr {
			fmt.Fprint(w, `{"errors":[{"message":"fake-only"}]}`)
			return
		}
		if strings.Contains(q.Query, "query Candidates") {
			if strings.Contains(q.Query, "description") || strings.Contains(q.Query, "comments") {
				t.Error("body fields queried")
			}
			filter := q.Variables["filter"].(map[string]any)
			if filter["team"] == nil || filter["assignee"] == nil || filter["state"] == nil {
				t.Error("missing filters")
			}
			issues := append([]Issue{f.issue()}, f.extraIssues...)
			if f.noIssues {
				issues = nil
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"issues": map[string]any{"nodes": issues, "pageInfo": Page{HasNextPage: f.issuePage}}, "users": map[string]any{"nodes": []any{map[string]string{"id": "bot-id"}}}}})
			return
		}
		if strings.Contains(q.Query, "query Membership") {
			teams := []any{}
			if f.member {
				teams = append(teams, map[string]string{"key": "TEST"})
			}
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user": map[string]any{"id": q.Variables["id"], "active": !f.memberInactive && q.Variables["id"] != f.inactiveMemberID, "app": f.memberApp, "guest": f.guest, "teams": map[string]any{"nodes": teams}}}})
			return
		}
		f.histories++
		f.cursors = append(f.cursors, q.Variables["after"])
		if q.Variables["after"] == "" {
			fmt.Fprint(w, `{"errors":[{"message":"Argument Validation Error","path":["issue","history"],"extensions":{"constraints":{"customValidation":"after is not a valid pagination cursor identifier"}}}]}`)
			return
		}
		if q.Variables["after"] != nil && q.Variables["after"] != "next-page" {
			t.Error("unexpected history cursor", q.Variables["after"])
		}
		if f.historyHeaders {
			w.Header().Set("X-RateLimit-Requests-Limit", "100")
			w.Header().Set("X-RateLimit-Requests-Remaining", "10")
		}
		teams := []any{}
		if f.member {
			teams = append(teams, map[string]string{"key": "TEST"})
		}
		node := map[string]any{"createdAt": f.entered.Format(time.RFC3339Nano), "toStateId": "state-id", "toAssigneeId": "bot-id", "actor": map[string]any{"id": "actor-id", "active": !f.inactive, "guest": f.guest, "teams": map[string]any{"nodes": teams}}}
		node["actor"].(map[string]any)["app"] = f.actorApp
		if f.botActor {
			node["botActor"] = map[string]string{"id": "integration"}
		}
		nodes := []any{node}
		if f.assignmentBot {
			nodes = append(nodes, map[string]any{"createdAt": f.entered.Add(time.Hour).Format(time.RFC3339Nano), "toAssigneeId": "bot-id", "actor": map[string]any{"id": "bot-id", "active": true, "teams": map[string]any{"nodes": teams}}})
		}
		if f.transitionBot {
			nodes = append(nodes, map[string]any{"createdAt": f.entered.Add(time.Hour).Format(time.RFC3339Nano), "toStateId": "state-id", "actor": map[string]any{"id": "bot-id", "active": true, "teams": map[string]any{"nodes": teams}}})
		}
		if f.transitionGuest {
			nodes = append(nodes, map[string]any{"createdAt": f.entered.Add(time.Hour).Format(time.RFC3339Nano), "toStateId": "state-id", "actor": map[string]any{"id": "actor-id", "guest": true, "active": true, "teams": map[string]any{"nodes": teams}}})
		}
		if f.transitionOutside {
			nodes = append(nodes, map[string]any{"createdAt": f.entered.Add(time.Hour).Format(time.RFC3339Nano), "toStateId": "state-id", "actor": map[string]any{"id": "actor-id", "active": true, "teams": map[string]any{"nodes": []any{}}}})
		}
		if f.transitionSystem {
			nodes[len(nodes)-1].(map[string]any)["actor"] = nil
		}
		if f.historyNodes != nil {
			nodes = f.historyNodes
		}
		if f.emptyHistory {
			nodes = nil
		}
		var issueBot any
		if f.creatorBotActor {
			issueBot = map[string]string{"id": "integration"}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"issue": map[string]any{"botActor": issueBot, "creator": f.creator, "createdAt": f.entered.Add(-time.Hour), "history": map[string]any{"nodes": nodes, "pageInfo": Page{HasNextPage: f.historyPage && (q.Variables["after"] == nil || f.repeatHistoryCursor), EndCursor: "next-page"}}}}})
	}))
	t.Cleanup(server.Close)
	p.endpoint = server.URL
	return f
}
func (f *fixture) issue() Issue {
	return Issue{ID: "issue-id", Identifier: "TEST-1", URL: "https://example.invalid/issue/TEST-1/private-title-slug", Title: "Synthetic title", UpdatedAt: f.updated, State: StateRef{ID: "state-id", Name: "Review"}}
}
func (f *fixture) poll(t *testing.T) {
	t.Helper()
	if e := f.p.poll(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) live(t *testing.T) {
	t.Helper()
	r := &f.p.cfg.Status[0]
	r.Mode = "live"
	r.Template = filepath.Join(f.p.cfg.StateDir, "template.md")
	if e := os.WriteFile(r.Template, []byte("---\nname: {{name}}\nrepo: synthetic\n---\n{{identifier}} {{url}} {{title}} {{status}}\n## Acceptance\n- Synthetic check\n"), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestDryRunQueryHistoryCacheAndCounters(t *testing.T) {
	f := setup(t)
	f.poll(t)
	if !strings.Contains(f.out.String(), `"decision":"would-dispatch"`) {
		t.Fatal(f.out.String())
	}
	if f.calls != 2 || f.histories != 1 || f.p.day().Requests != 2 {
		t.Fatal("request counters")
	}
	if _, e := os.Stat(os.Getenv("INTAKE_CAPTURE")); !os.IsNotExist(e) {
		t.Fatal("dry-run dispatched")
	}
	f.poll(t)
	if f.histories != 1 || f.calls != 3 {
		t.Fatal("history fetched twice")
	}
	if len(f.p.day().Decisions) != 2 || f.p.state.LastPoll.IsZero() {
		t.Fatal("poll counters")
	}
	b, e := os.ReadFile(filepath.Join(f.p.cfg.StateDir, "2026-02-01.jsonl"))
	if e != nil || !bytes.Equal(b, f.out.Bytes()) {
		t.Fatal("daily log differs")
	}
	p, e := newPoller(f.p.cfg, "", &bytes.Buffer{})
	if e != nil || len(p.state.Cache) != 1 {
		t.Fatal("state not persisted")
	}
}
func TestActorCheck(t *testing.T) {
	f := setup(t)
	f.member = false
	f.poll(t)
	if !strings.Contains(f.out.String(), "assigning actor outside allowed team") {
		t.Fatal(f.out.String())
	}
}
func TestLeaseAndExistingRecord(t *testing.T) {
	f := setup(t)
	name := lease(f.issue(), f.p.cfg.Status[0], f.entered)
	if !slug.MatchString(name) || name != lease(f.issue(), f.p.cfg.Status[0], f.entered) {
		t.Fatal(name)
	}
	if name == lease(f.issue(), f.p.cfg.Status[0], f.entered.Add(time.Hour)) {
		t.Fatal("same lease on re-entry")
	}
	t.Setenv("INTAKE_INVENTORY", fmt.Sprintf(`{"local":{"rows":[{"name":%q,"state":"done"}]}}`, name))
	f.poll(t)
	if !strings.Contains(f.out.String(), "dispatch record exists") {
		t.Fatal(f.out.String())
	}
	f.updated = "v2"
	f.entered = f.entered.Add(time.Hour)
	f.poll(t)
	if f.histories != 2 || !strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.out.String())
	}
}
func TestLiveTemplateAndCleanup(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.poll(t)
	b, e := os.ReadFile(os.Getenv("INTAKE_CAPTURE"))
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.SplitN(string(b), "\n", 2)
	if _, e = os.Stat(parts[0]); !os.IsNotExist(e) {
		t.Fatal("temp file retained")
	}
	for _, want := range []string{f.issue().Title, f.issue().URL, "TEST-1", "Review", lease(f.issue(), f.p.cfg.Status[0], f.entered)} {
		if !strings.Contains(parts[1], want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(parts[1], "{{") || f.p.day().Dispatches != 1 {
		t.Fatal("render or counter")
	}
}
func TestDailyCap(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.day().Dispatches = 2
	f.poll(t)
	if !strings.Contains(f.out.String(), "daily cap") {
		t.Fatal(f.out.String())
	}
	f.p.now = func() time.Time { return time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC) }
	f.poll(t)
	if f.p.day().Dispatches != 1 {
		t.Fatal("cap did not reset at UTC midnight")
	}
}
func TestCodeLimitAndReadParallelism(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status[0].Concurrency = "code"
	t.Setenv("INTAKE_INVENTORY", `{"local":{"rows":[{"name":"test-2-triage-123456","state":"running"}]}}`)
	f.poll(t)
	if !strings.Contains(f.out.String(), "code-job limit") {
		t.Fatal(f.out.String())
	}
	f.p.cfg.Status[0].Concurrency = "read"
	f.poll(t)
	if f.p.day().Dispatches != 1 {
		t.Fatal("read job blocked")
	}
}
func TestPaused(t *testing.T) {
	f := setup(t)
	f.p.cfg.PauseFile = filepath.Join(f.p.cfg.StateDir, "PAUSED")
	os.WriteFile(f.p.cfg.PauseFile, nil, 0600)
	f.poll(t)
	if f.calls != 0 || !strings.Contains(f.out.String(), "paused") {
		t.Fatal(f.out.String())
	}
}
func TestRateHeaderBackoff(t *testing.T) {
	for _, kind := range []string{"Requests", "Complexity"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.headers["X-RateLimit-"+kind+"-Limit"] = "100"
			f.headers["X-RateLimit-"+kind+"-Remaining"] = "10"
			f.poll(t)
			n := f.calls
			f.poll(t)
			if f.calls != n {
				t.Fatal("backoff called Linear")
			}
			f.headers = map[string]string{}
			f.poll(t)
			if f.calls != n+2 {
				t.Fatal("did not resume")
			}
		})
	}
}
func Test429WaitsUntilReset(t *testing.T) {
	f := setup(t)
	f.status = 429
	reset := f.p.now().Add(5 * time.Minute)
	f.headers["X-RateLimit-Requests-Remaining"] = "0"
	f.headers["X-RateLimit-Requests-Reset"] = fmt.Sprint(reset.UnixMilli())
	if e := f.p.poll(context.Background()); e == nil {
		t.Fatal("429 accepted")
	}
	f.poll(t)
	if f.calls != 1 {
		t.Fatal("retried before reset")
	}
	f.status = 0
	f.headers = map[string]string{}
	f.p.now = func() time.Time { return reset.Add(time.Second) }
	f.poll(t)
	if f.calls != 3 {
		t.Fatal("did not resume after reset")
	}
}
func TestRestartGuard(t *testing.T) {
	f := setup(t)
	for i := 0; i < 3; i++ {
		if e := f.p.start(); e != nil {
			t.Fatal(e)
		}
	}
	p, e := newPoller(f.p.cfg, "fake-only", f.out)
	if e != nil {
		t.Fatal(e)
	}
	p.now = f.p.now
	if e = p.start(); e == nil || !strings.Contains(e.Error(), "restart guard") {
		t.Fatal("guard failed")
	}
}
func TestFailClosed(t *testing.T) {
	for _, kind := range []string{"pagination", "graphql", "inventory"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			switch kind {
			case "pagination":
				f.issuePage = true
			case "graphql":
				f.graphErr = true
			case "inventory":
				t.Setenv("INTAKE_INVENTORY", `{"local":{"error":"unreachable"}}`)
			}
			if e := f.p.poll(context.Background()); e == nil {
				t.Fatal("accepted incomplete data")
			}
			if f.p.day().Dispatches != 0 {
				t.Fatal("dispatched")
			}
		})
	}
}
func TestUncertainDispatchReserved(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.command = func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(`{"local":{"rows":[]}}`), nil
		}
		return nil, fmt.Errorf("lost connection")
	}
	if e := f.p.poll(context.Background()); e != nil {
		t.Fatal(e)
	}
	f.poll(t)
	if f.p.day().Dispatches != 1 || !strings.Contains(f.out.String(), "requires reconciliation") {
		t.Fatal(f.out.String())
	}
}
func TestConfigAndDescribe(t *testing.T) {
	var out bytes.Buffer
	if e := run([]string{"--hev-describe"}, &out); e != nil || strings.Count(out.String(), "\n") != 1 {
		t.Fatal("describe")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	text := `state_dir = "state"
[linear]
api_key_env = "FAKE_LINEAR_KEY"
team = "TEST"
assignee = "testbot"
allowed_actor_team = "TEST"
[[status]]
name = "Review"
action = "triage"
concurrency = "read"
`
	os.WriteFile(path, []byte(text), 0600)
	c, e := loadConfig(path)
	if e != nil || c.Status[0].Mode != "dry-run" {
		t.Fatal(c, e)
	}
	os.WriteFile(path, []byte("unknown = true\n"+text), 0600)
	if _, e = loadConfig(path); e == nil {
		t.Fatal("unknown config accepted")
	}
}

func TestRetryAfterAndResetWithoutRemaining(t *testing.T) {
	for _, kind := range []string{"Retry-After", "Complexity"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.status = 429
			reset := f.p.now().Add(3 * time.Minute)
			if kind == "Retry-After" {
				f.headers[kind] = "180"
			} else {
				f.headers["X-RateLimit-Complexity-Reset"] = fmt.Sprint(reset.UnixMilli())
			}
			if e := f.p.poll(context.Background()); e == nil {
				t.Fatal("accepted 429")
			}
			if !f.p.state.BackoffUntil.Equal(reset) {
				t.Fatalf("reset: %s", f.p.state.BackoffUntil)
			}
		})
	}
}
func TestOffSkipsHistory(t *testing.T) {
	f := setup(t)
	f.p.cfg.Status[0].Mode = "off"
	f.poll(t)
	if f.histories != 0 || !strings.Contains(f.out.String(), "status off") {
		t.Fatal(f.out.String())
	}
}
func TestLiveRejectsMismatchedName(t *testing.T) {
	f := setup(t)
	f.live(t)
	os.WriteFile(f.p.cfg.Status[0].Template, []byte("---\nname: different\n---\n## Acceptance\n- Synthetic\n"), 0600)
	if e := f.p.poll(context.Background()); e != nil {
		t.Fatal(e)
	}
	if f.p.day().Dispatches != 0 {
		t.Fatal("reserved invalid order")
	}
	if !strings.Contains(f.out.String(), "invalid order or local preparation failed") {
		t.Fatal("missing rejection decision")
	}
}
func TestReadInventoryDoesNotConsumeCodeSlot(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status = append(f.p.cfg.Status, Rule{Name: "Synthetic ready", Action: "inspect", Concurrency: "read", Mode: "off"})
	f.p.cfg.Status[0].Concurrency = "code"
	t.Setenv("INTAKE_INVENTORY", `{"local":{"rows":[{"name":"test-2-inspect-a1b2c3","state":"running"}]}}`)
	f.poll(t)
	if f.p.day().Dispatches != 1 {
		t.Fatal(f.out.String())
	}
}
func TestStatusWithoutCredential(t *testing.T) {
	f := setup(t)
	f.p.now = time.Now
	f.poll(t)
	config := filepath.Join(t.TempDir(), "intake.toml")
	text := fmt.Sprintf(`state_dir = %q
[linear]
api_key_env = "UNSET_SYNTHETIC_INTAKE_KEY"
team = "TEST"
assignee = "testbot"
allowed_actor_team = "TEST"
[[status]]
name = "Review"
action = "triage"
concurrency = "read"
`, f.p.cfg.StateDir)
	os.WriteFile(config, []byte(text), 0600)
	var out bytes.Buffer
	if e := run([]string{"linear", "status", "--config", config}, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), `"linear_requests":2`) || !strings.Contains(out.String(), "would-dispatch") {
		t.Fatal(out.String())
	}
}

func TestHistoryPagination(t *testing.T) {
	f := setup(t)
	f.historyPage = true
	f.poll(t)
	if len(f.cursors) != 2 || f.cursors[0] != nil || f.cursors[1] != "next-page" {
		t.Fatal("history cursors", f.cursors)
	}
	if f.histories != 2 || f.p.day().Requests != 3 || !strings.Contains(f.out.String(), "would-dispatch") {
		t.Fatal(f.histories, f.out.String())
	}
}
func TestOverlappingActionNamesStillCountCode(t *testing.T) {
	f := setup(t)
	f.live(t)
	f.p.cfg.Status[0].Action = "hot-fix"
	f.p.cfg.Status[0].Concurrency = "code"
	f.p.cfg.Status = append(f.p.cfg.Status, Rule{Name: "Synthetic read", Action: "fix", Concurrency: "read", Mode: "off"})
	t.Setenv("INTAKE_INVENTORY", `{"local":{"rows":[{"name":"test-2-hot-fix-a1b2c3","state":"running"}]}}`)
	f.poll(t)
	if !strings.Contains(f.out.String(), "code-job limit") {
		t.Fatal(f.out.String())
	}
}

// Walk selection sets with the User types used by intake's fake schema.
// Arguments are removed first so input fields are not mistaken for selections.
func unknownUserField(query string) bool {
	tokens := regexp.MustCompile(`[A-Za-z_][A-Za-z_0-9]*|[{}():]`).FindAllString(query, -1)
	pos := 0
	var walk func(string) bool
	walk = func(kind string) bool {
		for pos < len(tokens) {
			field := tokens[pos]
			pos++
			if field == "}" {
				return false
			}
			if field == "{" {
				if walk("") {
					return true
				}
				continue
			}
			if pos < len(tokens) && tokens[pos] == ":" {
				pos++
				field = tokens[pos]
				pos++
			}
			if kind == "User" && field != "id" && field != "guest" && field != "app" && field != "active" && field != "teams" && field != "__typename" {
				return true
			}
			if pos < len(tokens) && tokens[pos] == "(" {
				depth := 0
				for pos < len(tokens) {
					token := tokens[pos]
					pos++
					if token == "(" {
						depth++
					}
					if token == ")" {
						depth--
						if depth == 0 {
							break
						}
					}
				}
			}
			next := ""
			switch field {
			case "user", "actor", "assignee", "creator":
				next = "User"
			case "users":
				next = "Users"
			case "nodes":
				if kind == "Users" {
					next = "User"
				}
			}
			if pos < len(tokens) && tokens[pos] == "{" {
				pos++
				if walk(next) {
					return true
				}
			}
		}
		return false
	}
	// Skip the operation name and variable declarations.
	depth := 0
	for pos < len(tokens) {
		token := tokens[pos]
		pos++
		if token == "(" {
			depth++
		}
		if token == ")" {
			depth--
		}
		if token == "{" && depth == 0 {
			return walk("")
		}
	}
	return false
}
