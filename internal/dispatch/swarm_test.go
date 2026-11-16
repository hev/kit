package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// swarmRFCExample is RFC 0013's example swarm file.
func swarmRFCExample(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "..", "docs", "rfcs", "0013-swarms.md"))
	if e != nil {
		t.Fatal(e)
	}
	_, rest, ok := strings.Cut(string(b), "`logfix.md`:\n\n```markdown\n")
	if !ok {
		t.Fatal("RFC example missing")
	}
	example, _, _ := strings.Cut(rest, "\n```\n")
	return example + "\n"
}

// swarmPhase1Example removes the RFC example's later-phase fields.
func swarmPhase1Example(t *testing.T) string {
	t.Helper()
	text := swarmRFCExample(t)
	for _, pat := range []string{`standby: .*\n`, `host_grace: .*\n`, `plans: .*\n`, `max_runs: .*\n`, `, swarm_limit: 8`, `    placement: \{spread: true, os: linux\}\n`} {
		re := regexp.MustCompile(pat)
		if !re.MatchString(text) {
			t.Fatalf("RFC example lacks %s", pat)
		}
		text = re.ReplaceAllString(text, "")
	}
	return text
}

func swarmHome(t *testing.T) {
	t.Helper()
	orchHome(t)
	cfg := filepath.Join(os.Getenv("HOME"), "target.toml")
	if err := os.WriteFile(cfg, []byte("cap=8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	oldLaunch, oldOrch, oldWait, oldHook := swarmLaunch, orchestratorLaunch, swarmStopWait, swarmBeforeLaunch
	swarmLaunch = func(string) error { return nil }
	orchestratorLaunch = func(string) error { return nil }
	swarmStopWait = 2 * time.Second
	t.Cleanup(func() {
		swarmLaunch, orchestratorLaunch, swarmStopWait, swarmBeforeLaunch = oldLaunch, oldOrch, oldWait, oldHook
	})
}

// fakeSwarmOps is a dispatch target in memory. Like a real target it refuses
// a duplicate run name and a retry of a running order.
type fakeSwarmOps struct {
	rows      map[string]Row
	orders    map[string]string
	starts    []string
	stops     []string
	retries   []string
	notes     []string
	unpushed  map[string]bool
	progress  map[string]string
	stuck     map[string]bool
	maxActive int
}

func newFakeSwarmOps() *fakeSwarmOps {
	return &fakeSwarmOps{rows: map[string]Row{}, orders: map[string]string{}, unpushed: map[string]bool{}, progress: map[string]string{}, stuck: map[string]bool{}}
}

func (f *fakeSwarmOps) active() int {
	n := 0
	for _, r := range f.rows {
		if r.State == "running" {
			n++
		}
	}
	return n
}

func (f *fakeSwarmOps) List(context.Context) (map[string]Row, error) {
	out := map[string]Row{}
	for k, v := range f.rows {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSwarmOps) Start(_ context.Context, name, order string) error {
	if _, ok := f.rows[name]; ok {
		return fmt.Errorf("%w: %s", errRunExists, name)
	}
	if _, e := Parse(order); e != nil {
		return e
	}
	f.starts = append(f.starts, name)
	f.orders[name] = order
	f.rows[name] = Row{Kind: "run", Meta: Meta{Order: Order{Name: name}, Branch: "d/" + name, Start: time.Now().UTC()}, State: "running"}
	f.maxActive = max(f.maxActive, f.active())
	return nil
}

func (f *fakeSwarmOps) Stop(_ context.Context, name string) error {
	f.stops = append(f.stops, name)
	r, ok := f.rows[name]
	if !ok {
		return fmt.Errorf("order not found: %s", name)
	}
	if r.State == "running" && !f.stuck[name] {
		r.State = "failed"
		f.rows[name] = r
	}
	return nil
}

func (f *fakeSwarmOps) Show(_ context.Context, name string) (Response, error) {
	r, ok := f.rows[name]
	if !ok {
		return Response{}, fmt.Errorf("order not found: %s", name)
	}
	res := Result{ExitCode: 1, End: time.Now()}
	if r.State == "running" {
		res.End = time.Time{}
	}
	b, _ := json.Marshal(res)
	return Response{Active: r.State == "running", Files: map[string]string{"result.json": string(b)}}, nil
}

func (f *fakeSwarmOps) Retry(_ context.Context, name, note string) error {
	r, ok := f.rows[name]
	if !ok {
		return fmt.Errorf("order not found: %s", name)
	}
	if r.State == "running" {
		return fmt.Errorf("stop running order before rework")
	}
	f.retries = append(f.retries, name)
	f.notes = append(f.notes, note)
	r.State, r.Start = "running", time.Now().UTC()
	f.rows[name] = r
	f.maxActive = max(f.maxActive, f.active())
	return nil
}

func (f *fakeSwarmOps) Pushed(_ context.Context, r Row) bool {
	return r.Head != "" && !f.unpushed[r.Name]
}

func (f *fakeSwarmOps) Progress(_ context.Context, name string) string { return f.progress[name] }

func (f *fakeSwarmOps) end(name, state string) {
	r := f.rows[name]
	r.State = state
	if state == "done" {
		r.Head = fmt.Sprintf("%040x", len(name)*7919+len(f.starts))
	}
	f.rows[name] = r
}

func swarmUnits(t *testing.T, name string) map[string]SwarmUnit {
	t.Helper()
	_, st, e := loadSwarm(name)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]SwarmUnit{}
	for _, u := range st.Units {
		out[u.Role+"/"+u.Unit] = u
	}
	return out
}

func swarmNotices(t *testing.T, name string) []OrchestratorNotice {
	t.Helper()
	files, e := loadOrchestratorNotices(name)
	if e != nil {
		t.Fatal(e)
	}
	var out []OrchestratorNotice
	for _, f := range files {
		if f.n.Kind == "swarm" {
			out = append(out, f.n)
		}
	}
	return out
}

// swarmFile writes a small swarm with the given roles block and body.
func swarmFile(roles, body string) string {
	return "---\nkind: swarm\nname: sw\ngoal: Do the thing.\nreply: owner\nmax_lifetime: 1d\nbudget: {concurrent_runs: 8}\nrestart: {limit: 2, backoff: 2m, max_backoff: 30m}\nroles:\n" + roles + "---\n## Problem\n\nFix it.\n\n## Acceptance\n- fixed\n" + body
}

func roleBody(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "\n## Role: %s\n\nDo %s.\n\n### Acceptance\n- %s done\n", n, n, n)
	}
	return b.String()
}

func swarmUp(t *testing.T, text string) {
	t.Helper()
	if _, e := SwarmUp(text, false); e != nil {
		t.Fatal(e)
	}
}

func tick(t *testing.T, ops SwarmOps, now time.Time) {
	t.Helper()
	if e := SwarmTick(context.Background(), "sw", ops, now); e != nil {
		t.Fatal(e)
	}
}

const partitionRole = "  - name: migrate\n    shape: partition\n    replicas: 3\n    pieces: [api, worker, cli, storage, web]\n    order: {repo: app, timeout: 30}\n"

func TestSwarmRFCExampleDecodesAndCompiles(t *testing.T) {
	swarmHome(t)
	if _, e := ParseSwarm(swarmRFCExample(t)); e != nil {
		t.Fatalf("full RFC example must decode: %v", e)
	}
	if _, e := CompileSwarm(swarmRFCExample(t)); e == nil || !strings.Contains(e.Error(), "phase 4") {
		t.Fatalf("full RFC example must be refused naming its phase: %v", e)
	}
	plan, e := SwarmUp(swarmPhase1Example(t), true)
	if e != nil {
		t.Fatal(e)
	}
	if plan.Orch.Manages.Runs.Selector != "swarm=logfix" || !slices.Contains(plan.Orch.Authority, "stop") || plan.Orch.Mode != "loop" || plan.Orch.Until != "goal" {
		t.Fatalf("compiled spec %+v", plan.Orch)
	}
	if _, e := ApplyOrchestrator(plan.OrchSpec, true); e != nil {
		t.Fatalf("orchestrate apply --dry-run refuses the compiled spec: %v", e)
	}
	if len(plan.Orders) != 6 {
		t.Fatalf("orders %d", len(plan.Orders))
	}
	for _, o := range plan.Orders {
		parsed, e := Parse(o.Text)
		if e != nil || parsed.Name != o.Run || parsed.Reply != "owner-session" || parsed.Host != "build-1" {
			t.Fatalf("%s: %+v %v", o.Run, parsed, e)
		}
		for _, want := range []string{"## Problem", "## Role: " + o.Role, "## Assignment", "## Acceptance"} {
			if !strings.Contains(o.Text, want) {
				t.Fatalf("%s lacks %s:\n%s", o.Run, want, o.Text)
			}
		}
	}
	integrate := plan.Orders[5]
	if integrate.Run != "logfix-integrate-g1" || !strings.Contains(integrate.Text, "d/logfix-migrate-storage-g1") || strings.Contains(integrate.Text, "do not open a pull request") {
		t.Fatalf("integrate order:\n%s", integrate.Text)
	}
	if !strings.Contains(plan.Orders[0].Text, "Piece: api") || !strings.Contains(plan.Orders[0].Text, "Do not open a pull request") {
		t.Fatalf("piece order:\n%s", plan.Orders[0].Text)
	}
	for _, p := range []string{Root(), OrchestratorConfigDir()} {
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatalf("dry run wrote %s", p)
		}
	}
}

func TestSwarmRefusals(t *testing.T) {
	swarmHome(t)
	base := swarmPhase1Example(t)
	var many strings.Builder
	for i := 0; i < 33; i++ {
		fmt.Fprintf(&many, "p%d, ", i)
	}
	pieces := strings.TrimSuffix(many.String(), ", ")
	for _, tt := range []struct{ name, old, new, want string }{
		{"unknown top field", "every: 30m", "every: 30m\nevry: 1h", "evry"},
		{"unknown role field", "    stall: 45m", "    stal: 45m", "stal"},
		{"unknown order key", "class: code}", "class: code, name: x}", "name"},
		{"replicas over 8", "replicas: 3", "replicas: 9", "ceiling of 8"},
		{"units over 32", "pieces: [api, worker, cli, storage, web]", "pieces: [" + pieces + "]", "ceiling of 32"},
		{"after cycle", "    stall: 45m", "    stall: 45m\n    after: [integrate]", "cycle in after"},
		{"standby", "every: 30m", "every: 30m\nstandby: [build-2]", "phase 4"},
		{"state_repo", "every: 30m", "every: 30m\nstate_repo: private", "phase 4"},
		{"host_grace", "every: 30m", "every: 30m\nhost_grace: 10m", "phase 3"},
		{"plans", "every: 30m", "every: 30m\nplans: {hold_at: 85}", "phase 2"},
		{"max_runs", "every: 30m", "every: 30m\nmax_runs: 24", "phase 2"},
		{"swarm_limit", "max_backoff: 30m}", "max_backoff: 30m, swarm_limit: 8}", "phase 2"},
		{"pieces orchestrator", "pieces: [api, worker, cli, storage, web]", "pieces: orchestrator", "phase 2"},
		{"spread", "    stall: 45m", "    stall: 45m\n    placement: {spread: true}", "phase 3"},
		{"other host", "placement: {hosts: [build-1]}", "placement: {hosts: [build-3]}", "phase 3"},
		{"two pr roles", "    stall: 45m", "    stall: 45m\n    effects: pr", "effects: pr"},
		{"missing role acceptance", "### Acceptance\n- Your package", "- Your package", "### Acceptance"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(base, tt.old) {
				t.Fatalf("fixture lacks %q", tt.old)
			}
			_, e := SwarmUp(strings.Replace(base, tt.old, tt.new, 1), true)
			if e == nil || !strings.Contains(e.Error(), tt.want) {
				t.Fatalf("want refusal naming %q, got %v", tt.want, e)
			}
		})
	}
}

func TestSwarmSchemaMatchesDecoder(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "..", "schemas", "swarm.schema.json"))
	if e != nil {
		t.Fatal(e)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if e = json.Unmarshal(b, &schema); e != nil {
		t.Fatal(e)
	}
	keys := func(v any) []string {
		var out []string
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			tag, _, _ := strings.Cut(rt.Field(i).Tag.Get("yaml"), ",")
			if tag != "" && tag != "-" {
				out = append(out, tag)
			}
		}
		slices.Sort(out)
		return out
	}
	names := func(m any) []string {
		var out []string
		for _, k := range reflect.ValueOf(m).MapKeys() {
			out = append(out, k.String())
		}
		slices.Sort(out)
		return out
	}
	if got, want := names(schema.Properties), keys(SwarmSpec{}); !slices.Equal(got, want) {
		t.Fatalf("top-level schema %v, decoder %v", got, want)
	}
	if got, want := names(schema.Defs["role"].Properties), keys(SwarmRole{}); !slices.Equal(got, want) {
		t.Fatalf("role schema %v, decoder %v", got, want)
	}
	order := map[string]bool{}
	for k := range schema.Defs["order"].Properties {
		order[k] = true
	}
	var orderNames []string
	for k := range order {
		orderNames = append(orderNames, k)
	}
	slices.Sort(orderNames)
	if want := keys(SwarmOrder{}); !slices.Equal(orderNames, want) {
		t.Fatalf("order schema %v, decoder %v", orderNames, want)
	}
	if got, want := names(schema.Defs["role"].Properties["placement"].Properties), keys(SwarmPlacement{}); !slices.Equal(got, want) {
		t.Fatalf("placement schema %v, decoder %v", got, want)
	}
}

func TestSwarmPartitionNeverExceedsReplicas(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile(partitionRole, roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		tick(t, ops, now)
		if ops.active() > 3 {
			t.Fatalf("tick %d: %d units running", i, ops.active())
		}
		// Finish the oldest running unit each round.
		for _, n := range ops.starts {
			if ops.rows[n].State == "running" {
				ops.end(n, "done")
				break
			}
		}
		now = now.Add(time.Minute)
	}
	if ops.maxActive != 3 {
		t.Fatalf("max active %d, want 3", ops.maxActive)
	}
	if len(ops.starts) != 5 {
		t.Fatalf("started %v", ops.starts)
	}
	for k, u := range swarmUnits(t, "sw") {
		if u.State != "done" || u.Head == "" {
			t.Fatalf("%s: %+v", k, u)
		}
	}
}

func TestSwarmRaceRecordsOneChoice(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: try\n    shape: race\n    replicas: 3\n    order: {repo: app}\n", roleBody("try")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	if len(ops.starts) != 3 {
		t.Fatalf("race started %v", ops.starts)
	}
	// Every replica gets the same task: orders differ only in their names.
	strip := func(s string) string {
		return regexp.MustCompile(`sw-try-v[0-9]-g1|variant `+"`v[0-9]`").ReplaceAllString(s, "X")
	}
	for _, n := range ops.starts[1:] {
		if strip(ops.orders[n]) != strip(ops.orders[ops.starts[0]]) {
			t.Fatalf("race orders differ:\n%s\n%s", ops.orders[ops.starts[0]], ops.orders[n])
		}
	}
	ops.end("sw-try-v2-g1", "done")
	ops.end("sw-try-v1-g1", "done")
	ops.end("sw-try-v3-g1", "done")
	tick(t, ops, now.Add(time.Minute))
	inbox, _ := os.ReadFile(filepath.Join(OrchestratorDir("sw"), "inbox.md"))
	if !strings.Contains(string(inbox), "race try is ready") {
		t.Fatalf("round not asked to choose: %q", inbox)
	}
	if e := SwarmChoose("sw", "try/v1", "", "person"); e == nil {
		t.Fatal("a person chose for an orchestrator race")
	}
	round := t.TempDir()
	if e := os.WriteFile(filepath.Join(round, "choice.json"), []byte(`{"role":"try","unit":"v2","reason":"smallest diff"}`), 0600); e != nil {
		t.Fatal(e)
	}
	applySwarmRoundChoice("sw", round)
	if e := SwarmChoose("sw", "try/v3", "", "orchestrator"); e == nil || !strings.Contains(e.Error(), "already chose v2") {
		t.Fatalf("second choice: %v", e)
	}
	chosen := 0
	for _, u := range swarmUnits(t, "sw") {
		if u.Chosen {
			chosen++
			if u.Unit != "v2" {
				t.Fatalf("chose %s", u.Unit)
			}
		}
	}
	_, st, _ := loadSwarm("sw")
	if chosen != 1 || st.Roles[0].Chosen != "v2" || st.Roles[0].ChosenBy != "orchestrator" || st.Roles[0].ChoiceReason != "smallest diff" {
		t.Fatalf("choice record %+v", st.Roles[0])
	}
}

func TestSwarmFirstGreenStopsTheOthers(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: try\n    shape: race\n    choose: first-green\n    variants: [{model: a}, {model: b}, {harness: claude, model: c}]\n    order: {repo: app}\n", roleBody("try")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	if len(ops.starts) != 3 || !strings.Contains(ops.orders["sw-try-v3-g1"], "harness: claude") || !strings.Contains(ops.orders["sw-try-v2-g1"], "model: b") {
		t.Fatalf("variants: %v", ops.orders)
	}
	ops.end("sw-try-v3-g1", "done")
	tick(t, ops, now.Add(time.Minute))
	units := swarmUnits(t, "sw")
	if !units["try/v3"].Chosen || units["try/v1"].State != "superseded" || units["try/v2"].State != "superseded" {
		t.Fatalf("first-green: %+v", units)
	}
	if !slices.Contains(ops.stops, "sw-try-v1-g1") || !slices.Contains(ops.stops, "sw-try-v2-g1") {
		t.Fatalf("losers not stopped: %v", ops.stops)
	}
}

const afterRoles = "  - name: migrate\n    shape: partition\n    replicas: 2\n    pieces: [api, cli]\n    order: {repo: app}\n  - name: integrate\n    shape: single\n    after: [migrate]\n    effects: pr\n    order: {repo: app}\n"

func TestSwarmAfterWaitsForEveryUnit(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile(afterRoles, roleBody("migrate", "integrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	ops.end("sw-migrate-api-g1", "done")
	tick(t, ops, now.Add(time.Minute))
	if slices.Contains(ops.starts, "sw-integrate-g1") {
		t.Fatal("integrate started before every piece finished")
	}
	ops.end("sw-migrate-cli-g1", "done")
	tick(t, ops, now.Add(2*time.Minute))
	if !slices.Contains(ops.starts, "sw-integrate-g1") {
		t.Fatalf("integrate not started: %v", ops.starts)
	}
	order := ops.orders["sw-integrate-g1"]
	if !strings.Contains(order, "d/sw-migrate-api-g1") || !strings.Contains(order, "d/sw-migrate-cli-g1") || !strings.Contains(order, ops.rows["sw-migrate-cli-g1"].Head) {
		t.Fatalf("integrate inputs:\n%s", order)
	}
}

func TestSwarmAfterNeverStartsWhenAPieceFailedForGood(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile(afterRoles, roleBody("migrate", "integrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	ops.end("sw-migrate-api-g1", "done")
	// cli fails past its restart limit of 2.
	for i := 0; i < 8; i++ {
		ops.end("sw-migrate-cli-g1", "failed")
		now = now.Add(time.Hour)
		tick(t, ops, now)
	}
	if u := swarmUnits(t, "sw")["migrate/cli"]; u.State != "held" || u.Restarts != 2 {
		t.Fatalf("cli: %+v", u)
	}
	if slices.Contains(ops.starts, "sw-integrate-g1") {
		t.Fatal("integrate started after a piece failed for good")
	}
}

func TestSwarmRepairInPlaceWithBackoffAndLimit(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    replicas: 1\n    pieces: [api]\n    order: {repo: app}\n", roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	run := "sw-migrate-api-g1"
	ops.end(run, "failed") // the worker was killed
	tick(t, ops, now)
	u := swarmUnits(t, "sw")["migrate/api"]
	if u.State != "failed" || !u.NextTry.Equal(now.Add(2*time.Minute)) || !slices.Contains(ops.stops, run) || len(ops.retries) != 0 {
		t.Fatalf("after kill: %+v stops=%v", u, ops.stops)
	}
	tick(t, ops, now.Add(time.Minute))
	if len(ops.retries) != 0 {
		t.Fatal("retried before the backoff elapsed")
	}
	tick(t, ops, now.Add(2*time.Minute))
	u = swarmUnits(t, "sw")["migrate/api"]
	if len(ops.retries) != 1 || u.State != "live" || u.Restarts != 1 || u.Gen != 1 || len(ops.starts) != 1 {
		t.Fatalf("repair in place: %+v retries=%v starts=%v", u, ops.retries, ops.starts)
	}
	if !strings.Contains(ops.notes[0], "failed, exit 1") {
		t.Fatalf("retry note %q", ops.notes[0])
	}
	// The backoff doubles.
	now = now.Add(3 * time.Minute)
	ops.end(run, "failed")
	tick(t, ops, now)
	if u = swarmUnits(t, "sw")["migrate/api"]; !u.NextTry.Equal(now.Add(4 * time.Minute)) {
		t.Fatalf("second backoff %v", u.NextTry.Sub(now))
	}
	tick(t, ops, now.Add(4*time.Minute))
	// At the limit (2) the unit is held with one notice to reply.
	ops.end(run, "failed")
	for i := 0; i < 3; i++ {
		tick(t, ops, now.Add(time.Duration(10+i)*time.Minute))
	}
	u = swarmUnits(t, "sw")["migrate/api"]
	if u.State != "held" || !strings.Contains(u.Reason, "restart limit 2") || len(ops.retries) != 2 {
		t.Fatalf("limit: %+v retries=%v", u, ops.retries)
	}
	if n := swarmNotices(t, "sw"); len(n) != 1 || n[0].To != "owner" || !strings.Contains(n[0].Message, "restart limit") {
		t.Fatalf("notices: %+v", n)
	}
}

func TestSwarmStallAndTimeout(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    replicas: 2\n    pieces: [api, cli]\n    stall: 30m\n    order: {repo: app}\n", roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	stalled, slow := "sw-migrate-api-g1", "sw-migrate-cli-g1"
	ops.progress[stalled] = "same"
	// cli keeps writing; api writes nothing.
	for i := 1; i <= 3; i++ {
		ops.progress[slow] = fmt.Sprint(i)
		tick(t, ops, now.Add(time.Duration(i)*15*time.Minute))
	}
	units := swarmUnits(t, "sw")
	if units["migrate/api"].State != "failed" || !strings.HasPrefix(units["migrate/api"].Reason, "stall") || units["migrate/cli"].State != "live" {
		t.Fatalf("stall: %+v", units)
	}
	if !slices.Contains(ops.stops, stalled) || slices.Contains(ops.stops, slow) {
		t.Fatalf("stops %v", ops.stops)
	}
	now = now.Add(time.Hour)
	ops.progress[slow] = "4"
	tick(t, ops, now) // retry in place after the backoff
	if len(ops.retries) != 1 {
		t.Fatalf("retries %v", ops.retries)
	}
	// A second stall is judged by a round before a third attempt.
	ops.progress[slow] = "5"
	tick(t, ops, now.Add(time.Minute))
	ops.progress[slow] = "6"
	tick(t, ops, now.Add(40*time.Minute))
	units = swarmUnits(t, "sw")
	if units["migrate/api"].State != "held" || !strings.Contains(units["migrate/api"].Reason, "second stall") || len(ops.retries) != 1 {
		t.Fatalf("second stall: %+v", units["migrate/api"])
	}
	inbox, _ := os.ReadFile(filepath.Join(OrchestratorDir("sw"), "inbox.md"))
	if !strings.Contains(string(inbox), "stalled twice") || !exists(filepath.Join(OrchestratorDir("sw"), "wake")) {
		t.Fatalf("round not woken: %q", inbox)
	}
	// A unit timeout is a hang the supervisor already ended: stop, confirm, repair.
	ops.end(slow, "timeout")
	later := now.Add(41 * time.Minute)
	tick(t, ops, later)
	if u := swarmUnits(t, "sw")["migrate/cli"]; u.State != "failed" || !strings.HasPrefix(u.Failure, "timeout") || !slices.Contains(ops.stops, slow) {
		t.Fatalf("timeout: %+v", u)
	}
	tick(t, ops, later.Add(2*time.Minute))
	if !slices.Contains(ops.retries, slow) {
		t.Fatalf("timeout not repaired: %v", ops.retries)
	}
}

func TestSwarmStopThatCannotConfirmHolds(t *testing.T) {
	swarmHome(t)
	swarmStopWait = 200 * time.Millisecond
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    pieces: [api]\n    stall: 10m\n    order: {repo: app}\n", roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	tick(t, ops, now)
	ops.stuck["sw-migrate-api-g1"] = true
	tick(t, ops, now.Add(time.Minute))
	tick(t, ops, now.Add(12*time.Minute))
	if u := swarmUnits(t, "sw")["migrate/api"]; u.State != "held" || !strings.Contains(u.Reason, "could not confirm") || len(ops.retries) != 0 {
		t.Fatalf("%+v", u)
	}
}

func TestSwarmAttemptingRecordLaunchesOnce(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    replicas: 2\n    pieces: [api, cli]\n    order: {repo: app}\n", roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	crash := errors.New("crash")
	// Crash after the attempting record, before the launch.
	swarmBeforeLaunch = func(u SwarmUnit) error {
		if u.Unit == "api" {
			return crash
		}
		return nil
	}
	if e := SwarmTick(context.Background(), "sw", ops, now); !errors.Is(e, crash) {
		t.Fatal(e)
	}
	if u := swarmUnits(t, "sw")["migrate/api"]; u.State != "attempting" || u.Run != "sw-migrate-api-g1" || len(ops.starts) != 0 {
		t.Fatalf("attempting record not written before launch: %+v", u)
	}
	swarmBeforeLaunch = nil
	tick(t, ops, now)
	tick(t, ops, now)
	if !slices.Equal(ops.starts, []string{"sw-migrate-api-g1", "sw-migrate-cli-g1"}) {
		t.Fatalf("starts %v", ops.starts)
	}
	// Crash after the launch, before anything recorded it: the next tick
	// sends the same start, the target refuses the duplicate, and the unit
	// becomes live with one run.
	_, st, _ := loadSwarm("sw")
	for i := range st.Units {
		if st.Units[i].Unit == "cli" {
			st.Units[i].State, st.Units[i].Op = "attempting", "start"
		}
	}
	if e := saveSwarmState(&st); e != nil {
		t.Fatal(e)
	}
	tick(t, ops, now)
	if u := swarmUnits(t, "sw")["migrate/cli"]; u.State != "live" || len(ops.starts) != 2 {
		t.Fatalf("%+v starts %v", u, ops.starts)
	}
	n := 0
	for name := range ops.rows {
		if strings.HasPrefix(name, "sw-migrate-api") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("api has %d runs", n)
	}
}

func TestSwarmPauseResumeDown(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    replicas: 1\n    pieces: [api, cli]\n    order: {repo: app}\n", roleBody("migrate")))
	ops := newFakeSwarmOps()
	now := time.Now().UTC()
	if e := SwarmPause("sw"); e != nil {
		t.Fatal(e)
	}
	tick(t, ops, now)
	if len(ops.starts) != 0 {
		t.Fatal("paused swarm launched")
	}
	if e := SwarmResume("sw"); e != nil {
		t.Fatal(e)
	}
	tick(t, ops, now)
	if len(ops.starts) != 1 {
		t.Fatalf("resume: %v", ops.starts)
	}
	// Pause holds repairs; the running worker continues.
	if e := SwarmPause("sw"); e != nil {
		t.Fatal(e)
	}
	ops.end("sw-migrate-api-g1", "failed")
	tick(t, ops, now.Add(time.Hour))
	if len(ops.stops) != 0 || len(ops.retries) != 0 {
		t.Fatal("paused swarm repaired")
	}
	if e := SwarmResume("sw"); e != nil {
		t.Fatal(e)
	}
	tick(t, ops, now.Add(time.Hour))
	tick(t, ops, now.Add(2*time.Hour))
	if len(ops.retries) != 1 {
		t.Fatalf("resume did not repair: %v", ops.retries)
	}
	rep, e := SwarmDown(context.Background(), "sw", ops)
	if e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(rep.Stopped, []string{"sw-migrate-api-g1"}) || ops.rows["sw-migrate-api-g1"].State == "running" {
		t.Fatalf("down: %+v", rep)
	}
	if !exists(filepath.Join(OrchestratorDir("sw"), "stop")) {
		t.Fatal("down left the orchestrator running")
	}
	view, e := SwarmPS("sw")
	if e != nil || !view.Down || view.Orchestrator != "stopped" || len(view.Units) != 2 || view.Units[0].State != "held" || view.Units[0].Reason != "down" {
		t.Fatalf("ps after down: %+v %v", view, e)
	}
	before := len(ops.starts)
	tick(t, ops, now.Add(3*time.Hour))
	if len(ops.starts) != before {
		t.Fatal("tick launched after down")
	}
}

func TestSwarmSelectorScope(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: migrate\n    shape: partition\n    pieces: [api]\n    order: {repo: app}\n", roleBody("migrate")))
	s, e := LoadOrchestrator("sw")
	if e != nil {
		t.Fatal(e)
	}
	if !s.Includes(Order{Name: "x", Repo: "app", Swarm: "sw"}) || s.Includes(Order{Name: "x", Repo: "app"}) || s.Includes(Order{Name: "x", Repo: "app", Swarm: "other"}) {
		t.Fatal("swarm selector scope")
	}
	bad := strings.Replace(orchestratorExample, `selector: "issue=REL-*"`, `selector: "swarm=other"`, 1)
	if _, e := ParseOrchestrator(bad); e == nil {
		t.Fatal("a swarm selector may only name its own orchestrator")
	}
	write := func(name, swarm string) {
		dir := filepath.Join(Root(), name)
		if e := os.MkdirAll(filepath.Join(dir, "wt"), 0700); e != nil {
			t.Fatal(e)
		}
		m := Meta{Order: Order{Name: name, Repo: "app", Swarm: swarm, Timeout: 1}, Branch: "d/" + name}
		if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
			t.Fatal(e)
		}
		if e := WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1, End: time.Now()}); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: "+name+"\nrepo: app\n---\n## Acceptance\n- x\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("outsider", "")
	write("neighbour", "other")
	write("sw-migrate-api-g1", "sw")
	for _, run := range []string{"outsider", "neighbour"} {
		for _, q := range []Request{{Action: "stop"}, {Action: "say", Note: "hi"}, {Action: "rework", Note: "again"}, {Action: "retry"}} {
			q.Name, q.Orchestrator = run, "sw"
			if _, e := Handle(q); e == nil || !strings.Contains(e.Error(), "outside manages scope") {
				t.Fatalf("%s %s through swarm authority: %v", q.Action, run, e)
			}
		}
	}
	if _, e := Handle(Request{Action: "stop", Name: "sw-migrate-api-g1", Orchestrator: "sw"}); e != nil {
		t.Fatalf("swarm cannot stop its own run: %v", e)
	}
	// A sender cannot claim a swarm: the start's metadata gets none.
	text := "---\nname: claimer\nrepo: app\n---\n## Acceptance\n- x\n"
	o, _ := Parse(text)
	m := Meta{Order: o}
	m.Swarm = "sw"
	_, _ = Handle(Request{Action: "start", Name: "claimer", Order: text, Meta: m})
	var got Meta
	if e := readJSON(filepath.Join(Root(), "claimer", "meta.json"), &got); e != nil || got.Swarm != "" {
		t.Fatalf("claimed swarm %q: %v", got.Swarm, e)
	}
}

func TestSwarmUpKeepsRecordsAcrossLifetimes(t *testing.T) {
	swarmHome(t)
	text := swarmFile("  - name: migrate\n    shape: partition\n    pieces: [api]\n    order: {repo: app}\n", roleBody("migrate"))
	swarmUp(t, text)
	ops := newFakeSwarmOps()
	tick(t, ops, time.Now())
	ops.end("sw-migrate-api-g1", "done")
	tick(t, ops, time.Now())
	if _, e := SwarmDown(context.Background(), "sw", ops); e != nil {
		t.Fatal(e)
	}
	swarmUp(t, strings.Replace(text, "pieces: [api]", "pieces: [api, cli]", 1))
	units := swarmUnits(t, "sw")
	if units["migrate/api"].State != "done" || units["migrate/cli"].State != "pending" {
		t.Fatalf("records after up: %+v", units)
	}
	if exists(filepath.Join(OrchestratorDir("sw"), "stop")) {
		t.Fatal("up did not start a new lifetime")
	}
}

// The supervisor runs the reconcile tick between rounds every
// reconcile_every, and a round sees the unit table and can record a choice.
func TestSwarmSupervisorTicksBetweenRounds(t *testing.T) {
	swarmHome(t)
	swarmUp(t, swarmFile("  - name: try\n    shape: race\n    replicas: 2\n    order: {repo: app}\n", roleBody("try")))
	clock := time.Now().UTC()
	var ticks []time.Time
	rounds, waits := 0, 0
	d := OrchestratorDriver{
		Now:    func() time.Time { return clock },
		Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil },
		Tick: func(_ context.Context, name string) error {
			if name != "sw" {
				t.Fatalf("tick for %s", name)
			}
			ticks = append(ticks, clock)
			return nil
		},
		Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
			rounds++
			if len(ticks) == 0 {
				t.Fatal("round before the first reconcile tick")
			}
			var st SwarmState
			if e := json.Unmarshal(p.Swarm, &st); e != nil || st.Name != "sw" || len(st.Units) != 2 {
				t.Fatalf("packet swarm %s: %v", p.Swarm, e)
			}
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Waiting on the race.\n"), 0600)
		},
		Wait: func(context.Context) error {
			waits++
			clock = clock.Add(time.Minute)
			if waits == 10 {
				return OrchestratorControl("sw", "stop", "")
			}
			return nil
		},
	}
	if e := SuperviseOrchestrator(context.Background(), "sw", d); e != nil {
		t.Fatal(e)
	}
	if rounds != 1 || len(ticks) < 5 || len(ticks) > 6 {
		t.Fatalf("rounds %d ticks %v", rounds, ticks)
	}
	for i := 1; i < len(ticks); i++ {
		if ticks[i].Sub(ticks[i-1]) != 2*time.Minute {
			t.Fatalf("tick cadence %v", ticks[i].Sub(ticks[i-1]))
		}
	}
}
