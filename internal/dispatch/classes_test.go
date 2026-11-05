package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func classOrder(name, class string) string {
	front := "---\nname: " + name + "\nrepo: app\nissue: REL-1\n"
	if class != "" {
		front += "class: " + class + "\n"
	}
	return front + "---\nDo the work.\n\n## Acceptance\n- done\n"
}

// classHost applies two dispatching orchestrators and a target with
// classes.code = 1, and returns a start function for any caller.
func classHost(t *testing.T) (start func(orch, name, class string) error, end func(name string)) {
	t.Helper()
	orchHome(t)
	for _, n := range []string{"alpha", "beta"} {
		if _, e := ApplyOrchestrator(strings.Replace(orchestratorExample, "name: release-train", "name: "+n, 1), false); e != nil {
			t.Fatal(e)
		}
	}
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(cfg, []byte("cap = 10\nclasses = {code = 1}\ndefault_host = 'worker'\n[hosts.worker]\nlocal = true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	workspace := t.TempDir()
	origin := filepath.Join(t.TempDir(), "origin.git")
	git := func(dir string, args ...string) {
		t.Helper()
		if _, e := command(dir, append([]string{"git", "-c", "user.name=Example", "-c", "user.email=example@example.com"}, args...)...); e != nil {
			t.Fatal(e)
		}
	}
	git("", "init", "--bare", "--initial-branch=main", origin)
	git("", "clone", origin, filepath.Join(workspace, "app"))
	git(filepath.Join(workspace, "app"), "commit", "--allow-empty", "-m", "Initial")
	git(filepath.Join(workspace, "app"), "push", "origin", "HEAD:main")
	end = fakeLiveLaunches(t)
	start = func(orch, name, class string) error {
		text := classOrder(name, class)
		o, e := Parse(text)
		if e != nil {
			t.Fatal(e)
		}
		_, e = Handle(Request{Action: "start", Name: name, Order: text, Orchestrator: orch, Meta: Meta{Order: o}, Workspace: workspace})
		return e
	}
	return start, end
}

func classFull(e error) bool { return e != nil && strings.Contains(e.Error(), "class code full") }

func TestDispatchClassCapAcrossOrchestratorsAndCLI(t *testing.T) {
	start, _ := classHost(t)
	if e := start("alpha", "alpha-one", "code"); e != nil {
		t.Fatal(e)
	}
	if e := start("beta", "beta-one", "code"); !classFull(e) {
		t.Fatalf("second orchestrator exceeded classes.code: %v", e)
	}
	if e := start("", "cli-one", "code"); !classFull(e) {
		t.Fatalf("direct start exceeded classes.code: %v", e)
	}
	// Other classes and unclassed orders are held only by the host cap.
	if e := start("beta", "beta-docs", "docs"); e != nil {
		t.Fatal(e)
	}
	if e := start("", "cli-plain", ""); e != nil {
		t.Fatal(e)
	}
	// A launch the supervisor has not confirmed still holds its slot.
	dir := filepath.Join(Root(), "pending")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "pending", Repo: "app", Class: "lint"}}); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(filepath.Join(dir, "result.json"), Result{LaunchUnconfirmed: true, Start: time.Now().UTC(), ExitCode: 1}); e != nil {
		t.Fatal(e)
	}
	if e := admitClass(map[string]int{"lint": 1}, "lint", "other", map[string]bool{}); e == nil {
		t.Fatal("launch-unconfirmed run did not occupy its class")
	}
}

func TestDispatchClassCapRetryAndRework(t *testing.T) {
	start, end := classHost(t)
	if e := start("alpha", "alpha-one", "code"); e != nil {
		t.Fatal(e)
	}
	end("alpha-one")
	if e := start("", "cli-one", "code"); e != nil {
		t.Fatal(e)
	}
	for _, q := range []Request{
		{Action: "retry", Name: "alpha-one"},
		{Action: "retry", Name: "alpha-one", Orchestrator: "alpha"},
		{Action: "rework", Name: "alpha-one", Note: "Again."},
		{Action: "rework", Name: "alpha-one", Note: "Again.", Orchestrator: "beta"},
		{Action: "say", Name: "alpha-one", Note: "Again."},
	} {
		if _, e := Handle(q); !classFull(e) {
			t.Fatalf("%s from %q exceeded classes.code: %v", q.Action, q.Orchestrator, e)
		}
	}
	// An amend restart is a launch too.
	var m Meta
	if e := readJSON(filepath.Join(Root(), "alpha-one", "meta.json"), &m); e != nil {
		t.Fatal(e)
	}
	live, e := liveNames()
	if e != nil {
		t.Fatal(e)
	}
	if _, e := checkAmendAdmission(Request{Name: "alpha-one"}, m, live, true); !classFull(e) {
		t.Fatalf("amend restart exceeded classes.code: %v", e)
	}
	end("cli-one")
	if _, e := Handle(Request{Action: "retry", Name: "alpha-one", Orchestrator: "alpha"}); e != nil {
		t.Fatal(e)
	}
	if e := start("", "cli-two", "code"); !classFull(e) {
		t.Fatalf("retried run did not occupy its class: %v", e)
	}
}

func TestDispatchClassSourceAttemptingReceiptOccupiesSlot(t *testing.T) {
	s := sourceFixture(t)
	if e := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(strings.Replace(sourceTemplate, "issue: {{.Event.ID}}\n", "issue: {{.Event.ID}}\nclass: code\n", 1)), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(os.Getenv("HEV_DISPATCH_CONFIG"), []byte("cap = 10\nclasses = {code = 1}\ndefault_host = 'worker'\n[hosts.worker]\nlocal = true\ncap = 4\n"), 0600); e != nil {
		t.Fatal(e)
	}
	direct := func(name string) error {
		text := classOrder(name, "code")
		o, _ := Parse(text)
		_, e := Handle(Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: o}, Workspace: t.TempDir()})
		return e
	}
	if e := direct("before"); classFull(e) {
		t.Fatal("class full before any receipt", e)
	}
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.AdmissionOnly || q.Action == "list" {
			return Response{}, nil
		}
		return Response{}, transportError{errors.New("connection lost")}
	}
	st, _ := orchestratorState(s.Name)
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e == nil {
		t.Fatal("uncertain attempt accepted")
	}
	receipts, e := loadSourceReceipts(s)
	if e != nil || len(receipts) != 1 {
		t.Fatal(receipts, e)
	}
	var run string
	for _, r := range receipts {
		if r.Mode != "attempting" || r.Class != "code" {
			t.Fatalf("receipt %+v", r)
		}
		run = r.Run
	}
	if e := direct("after"); !classFull(e) {
		t.Fatalf("attempting receipt did not occupy its class: %v", e)
	}
	// The attempt itself is admitted against its own slot, not refused by it.
	if e := direct(run); classFull(e) {
		t.Fatalf("attempting receipt blocked its own run: %v", e)
	}
}

func TestDispatchClassConfigAndOrder(t *testing.T) {
	for _, text := range []string{"classes = {code = 0}\n", "classes = {Bad_Name = 1}\n"} {
		p := filepath.Join(t.TempDir(), "dispatch.toml")
		if e := os.WriteFile(p, []byte(text), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Load(p); e == nil {
			t.Fatalf("accepted %q", text)
		}
		if _, e := loadTarget(p, "here"); e == nil {
			t.Fatalf("target accepted %q", text)
		}
	}
	if _, e := Parse(classOrder("bad-class", "Not Valid")); e == nil {
		t.Fatal("accepted invalid class")
	}
	o, e := Parse(classOrder("good-class", "code"))
	if e != nil || o.Class != "code" {
		t.Fatal(o, e)
	}
}

func timeNowUTC() time.Time { return time.Now().UTC() }

// The order schema lists exactly the frontmatter keys Parse accepts.
func TestOrderSchemaListsParsedKeys(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("..", "..", "schemas", "order.schema.json"))
	if e != nil {
		t.Fatal(e)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if e = json.Unmarshal(b, &schema); e != nil {
		t.Fatal(e)
	}
	if _, ok := schema.Properties["class"]; !ok {
		t.Fatal("schema does not list class")
	}
	values := map[string]string{"timeout": "5", "every": "1m", "until": "done", "max_lifetime": "1h", "harness": "claude", "validation": "none"}
	for key := range schema.Properties {
		v := values[key]
		if v == "" {
			v = "value"
		}
		front := "---\nname: schema\nrepo: app\nevery: 1m\n"
		if key != "name" && key != "repo" && key != "every" {
			front += key + ": " + v + "\n"
		}
		if _, e := Parse(front + "---\n## Acceptance\n- done\n"); e != nil && strings.Contains(e.Error(), "unknown key") {
			t.Fatalf("schema lists %s, which Parse rejects", key)
		}
	}
	if _, e := Parse("---\nname: schema\nrepo: app\nunlisted: x\n---\n## Acceptance\n- done\n"); e == nil {
		t.Fatal("Parse accepted an unlisted key")
	}
}
