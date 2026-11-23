package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidatorIdentity(t *testing.T) {
	o := Order{Harness: "codex", Model: "model-a"}
	v := ValidationConfig{Harness: "codex", Model: "model-a"}
	if err := CheckValidator(o, v, false); err == nil || !strings.Contains(err.Error(), "codex/model-a equals implementer codex/model-a") {
		t.Fatalf("%v", err)
	}
	if err := CheckValidator(o, v, true); err != nil {
		t.Fatal(err)
	}
	v.Model = "model-b"
	if err := CheckValidator(o, v, false); err != nil {
		t.Fatal(err)
	}
	v.Harness = "claude"
	v.Model = o.Model
	if err := CheckValidator(o, v, false); err == nil {
		t.Fatal("identical model accepted under a different harness")
	}
}
func TestPreviewParsing(t *testing.T) {
	for _, preview := range []string{"python3 -m http.server {port}", "https://example.org/app", `"npm run preview -- --port {port}"`} {
		o, err := Parse("---\nname: example\nrepo: project\npreview: " + preview + "\n---\n## Acceptance\n- click\n")
		if err != nil || o.Preview != strings.Trim(preview, `"`) {
			t.Fatalf("%+v %v", o, err)
		}
	}
	for _, preview := range []string{"{other}", "[url]", ""} {
		if _, err := Parse("---\nname: example\nrepo: project\npreview: " + preview + "\n---\n## Acceptance\n"); err == nil {
			t.Fatalf("accepted %q", preview)
		}
	}
}
func TestVerdictParsing(t *testing.T) {
	for _, b := range []string{
		`{"verdict":"pass","items":[{"item":"click","result":"pass","evidence":["../evidence/click.png"],"note":"message visible"}]}`,
		`{"verdict":"fail","items":[{"item":"click","result":"fail","evidence":[],"note":"button missing"}]}`,
	} {
		if _, err := ParseVerdict([]byte(b)); err != nil {
			t.Fatal(err)
		}
	}
	for _, b := range []string{`{}`, `{"verdict":"maybe","items":[]}`, `{"verdict":"pass","items":[{"item":"x","result":"fail","note":"missing"}]}`, `{"verdict":"pass","items":[{"item":"x","result":"pass","note":"ok"}]}`, `{"verdict":"pass","items":[],"extra":true}`, `{} {}`} {
		if _, err := ParseVerdict([]byte(b)); err == nil {
			t.Fatalf("accepted %s", b)
		}
	}
}
func failed(item string) ValidationRecord {
	return ValidationRecord{Verdict: &Verdict{Verdict: "fail", Items: []VerdictItem{{Item: item, Result: "fail"}}}}
}
func TestTwiceFailedAndReviewState(t *testing.T) {
	a := failed("button shows message")
	b := failed("page title")
	if TwiceFailed([]ValidationRecord{a, b}) {
		t.Fatal("different failures")
	}
	a.Verdict.Items = append(a.Verdict.Items, a.Verdict.Items[0])
	if TwiceFailed([]ValidationRecord{a}) {
		t.Fatal("duplicate within one verdict")
	}
	if !TwiceFailed([]ValidationRecord{a, b, a}) {
		t.Fatal("repeat missed")
	}
	dir := t.TempDir()
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now()})
	a.Name = "example-v1"
	if err := recordValidation(dir, a); err != nil {
		t.Fatal(err)
	}
	if _, err := validationNote(dir); err != nil {
		t.Fatal(err)
	}
	a.Name = "example-v2"
	if err := recordValidation(dir, a); err != nil {
		t.Fatal(err)
	}
	var r Result
	_ = readJSON(filepath.Join(dir, "result.json"), &r)
	if State(&r, false) != "needs-review" {
		t.Fatalf("%+v", r)
	}
	if _, err := validationNote(dir); err == nil {
		t.Fatal("third rework allowed")
	}
}
func TestAutomaticValidationDecision(t *testing.T) {
	m := Meta{Order: Order{Validation: "browser"}}
	r := Result{End: time.Now()}
	c := Config{Validation: ValidationConfig{Host: "browserbox"}}
	if !ShouldValidate(m, r, c) {
		t.Fatal("should launch")
	}
	cases := []struct {
		m Meta
		r Result
		c Config
	}{{m, r, Config{}}, {Meta{Order: Order{Validation: "none"}}, r, c}, {Meta{Order: m.Order, Parent: "parent"}, r, c}, {m, Result{ExitCode: 1, End: time.Now()}, c}, {m, Result{}, c}}
	for _, tt := range cases {
		if ShouldValidate(tt.m, tt.r, tt.c) {
			t.Fatalf("unexpected launch %+v", tt)
		}
	}
}
func TestValidatorAttribution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	child := filepath.Join(Root(), "example-v1")
	_ = os.MkdirAll(child, 0700)
	_ = os.WriteFile(filepath.Join(child, "order.md"), []byte("---\nname: example\nrepo: project\nissue: TASK-1\n---\n## Acceptance\n"), 0600)
	_ = WriteJSON(filepath.Join(child, "meta.json"), Meta{Parent: "example", Sender: "teammate"})
	if sender := SenderAttribution(filepath.Join(child, "wt")); sender != "teammate" {
		t.Fatalf("remote validator lost sender: %q", sender)
	}
	name, issue := Attribution(filepath.Join(child, "wt"))
	if name != "example" || issue != "TASK-1" {
		t.Fatalf("%s %s", name, issue)
	}
}

func TestVerifyVerdictCoverageAndEvidence(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	_ = os.MkdirAll(filepath.Join(dir, "evidence"), 0700)
	_ = os.WriteFile(filepath.Join(dir, "order.md"), []byte("## Acceptance\n- Click shows\n  a message.\n- Title is visible.\n\n## Rework\nIgnore this"), 0600)
	_ = os.WriteFile(filepath.Join(dir, "evidence", "click.png"), []byte("fixture"), 0600)
	v := &Verdict{Verdict: "pass", Items: []VerdictItem{{Item: "Click shows a message.", Evidence: []string{"../evidence/click.png"}}, {Item: "Title is visible.", Evidence: []string{"../evidence/click.png"}}}}
	if err := VerifyVerdict(dir, v); err != nil {
		t.Fatal(err)
	}
	v.Items = v.Items[:1]
	if err := VerifyVerdict(dir, v); err == nil {
		t.Fatal("omitted item allowed")
	}
	v.Items[0].Evidence = []string{"../../elsewhere.png"}
	if err := VerifyVerdict(dir, v); err == nil {
		t.Fatal("outside evidence allowed")
	}
}

func TestValidationReservationAndMonotonicCompletion(t *testing.T) {
	dir := t.TempDir()
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{})
	name, err := reserveValidation(dir, "example")
	if err != nil || name != "example-v1" {
		t.Fatalf("%s %v", name, err)
	}
	retry, err := reserveValidation(dir, "example")
	if err != nil || retry != name {
		t.Fatalf("%s %v", retry, err)
	}
	_ = recordValidation(dir, ValidationRecord{Name: name, State: "running"})
	if _, err := reserveValidation(dir, "example"); err == nil {
		t.Fatal("concurrent validation admitted")
	}
	_ = recordValidation(dir, ValidationRecord{Name: name, State: "pass"})
	_ = recordValidation(dir, ValidationRecord{Name: name, State: "running"})
	if validationRecords(dir)[0].State != "pass" {
		t.Fatal("late launch response overwrote verdict")
	}
	next, err := reserveValidation(dir, "example")
	if err != nil || next != "example-v2" {
		t.Fatalf("%s %v", next, err)
	}
}

func TestNumberedAcceptanceItems(t *testing.T) {
	items := acceptanceItems("## Acceptance\n1. Click works.\n2) Message shows.\n+ Title visible.\n## Rework\n")
	if len(items) != 3 || !items["Click works."] || !items["Message shows."] || !items["Title visible."] {
		t.Fatalf("%v", items)
	}
}

func TestIndependentReviewPromptAndVerdict(t *testing.T) {
	m := Meta{Parent: "parent", ReviewOnly: true}
	prompt := workerPrompt(t.TempDir(), m)
	if !strings.Contains(prompt, "independent acceptance reviewer") || strings.Contains(prompt, "real browser") {
		t.Fatal(prompt)
	}
	if e := CheckValidator(Order{Harness: "codex", Model: "shared"}, ValidationConfig{Harness: "claude", Model: "shared"}, false); e == nil {
		t.Fatal("same model across different harnesses accepted")
	}
	dir := t.TempDir()
	if e := WriteJSON(filepath.Join(dir, "result.json"), Result{Head: "revision"}); e != nil {
		t.Fatal(e)
	}
	v := &Verdict{Verdict: "pass", Items: []VerdictItem{{Item: "document", Result: "pass", Evidence: []string{"log"}, Note: "checked"}}}
	if e := recordValidation(dir, ValidationRecord{Name: "parent-v1", Revision: "revision", State: "pass", Verdict: v}); e != nil {
		t.Fatal(e)
	}
	var got Verdict
	if e := readJSON(filepath.Join(dir, "verdict.json"), &got); e != nil || got.Verdict != "pass" {
		t.Fatal(got, e)
	}
}

func TestWrappedCheckboxAcceptanceItems(t *testing.T) {
	dir := t.TempDir()
	order := "---\nname: test\nrepo: kit\n---\nTask\n\n## Acceptance\n- [ ] First line of wrapped\n  acceptance item.\n- [x] Second checked item.\n- Plain bullet\n  continuation.\n"
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte(order), 0600); err != nil {
		t.Fatal(err)
	}
	evDir := filepath.Join(dir, "evidence")
	if err := os.MkdirAll(evDir, 0700); err != nil {
		t.Fatal(err)
	}
	evFile := filepath.Join(evDir, "proof.txt")
	if err := os.WriteFile(evFile, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	v := &Verdict{
		Verdict: "pass",
		Items: []VerdictItem{
			{Item: "First line of wrapped acceptance item.", Result: "pass", Evidence: []string{evFile}},
			{Item: "[ ] Second checked item.", Result: "pass", Evidence: []string{evFile}},
			{Item: "Plain bullet continuation.", Result: "pass", Evidence: []string{evFile}},
		},
	}
	if err := VerifyVerdict(dir, v); err != nil {
		t.Fatalf("expected wrapped checkbox verdict to pass, got %v", err)
	}
	bad := &Verdict{
		Verdict: "pass",
		Items: []VerdictItem{
			{Item: "First line of wrapped acceptance item.", Result: "pass", Evidence: []string{evFile}},
			{Item: "[ ] First line of wrapped acceptance item.", Result: "pass", Evidence: []string{evFile}},
			{Item: "Plain bullet continuation.", Result: "pass", Evidence: []string{evFile}},
		},
	}
	if err := VerifyVerdict(dir, bad); err == nil {
		t.Fatal("expected duplicate checkbox item to fail")
	}
}

func TestOrchestratorValidateHonorsAllowSameModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := ApplyOrchestrator("---\nkind: orchestrator\nname: test-orch\ngoal: test\nmode: once\nauthority: [validate]\nmanages:\n  runs: {names: [impl-run]}\n---\ntest\n", false); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_ORCHESTRATOR", "test-orch")
	runDir := filepath.Join(Root(), "impl-run")
	if err := os.MkdirAll(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(runDir, "meta.json"), Meta{Order: Order{Name: "impl-run", Repo: "kit", Harness: "codex", Model: "same-model"}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(runDir, "result.json"), Result{ExitCode: 0, End: time.Now(), Head: "abc123"}); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Hosts: map[string]Host{
			"src": {Local: true},
			"val": {SSH: "val.example"},
		},
		Validation: ValidationConfig{Host: "val", Harness: "codex", Model: "same-model"},
	}
	if _, err := Validate(context.Background(), cfg, "impl-run", "src", false); err == nil || !strings.Contains(err.Error(), "use --allow-same-model") {
		t.Fatalf("expected same-model refusal, got %v", err)
	}
	_, err := Validate(context.Background(), cfg, "impl-run", "src", true)
	if err != nil && strings.Contains(err.Error(), "use --allow-same-model") {
		t.Fatalf("expected --allow-same-model to bypass same-model check inside orchestrator, got %v", err)
	}
}
