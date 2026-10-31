package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReview1ExpiredReservationReclaimed(t *testing.T) {
	dir := t.TempDir()
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{})
	_ = recordValidation(dir, ValidationRecord{Name: "example-v1", State: "running", Expires: time.Now().Add(-time.Minute)})
	name, err := reserveValidation(dir, "example")
	if err != nil || name != "example-v2" {
		t.Fatalf("%s %v", name, err)
	}
	if validationRecords(dir)[0].State != "fail" {
		t.Fatal("not terminal")
	}
}
func TestReview2StaleVerdictIgnored(t *testing.T) {
	dir := t.TempDir()
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{Head: "new", Validation: "pass", End: time.Now()})
	r := failed("click")
	r.Name = "example-v1"
	r.Revision = "old"
	r.State = "fail"
	if err := recordValidation(dir, r); err != nil {
		t.Fatal(err)
	}
	var result Result
	_ = readJSON(filepath.Join(dir, "result.json"), &result)
	if result.Validation != "pass" || result.NeedsReview || validationRecords(dir)[0].State != "stale" {
		t.Fatalf("%+v", result)
	}
	if _, err := validationNote(dir); err == nil {
		t.Fatal("stale verdict used for rework")
	}
}
func TestReview3HumanReworkResetsStreak(t *testing.T) {
	dir := t.TempDir()
	a := failed("click")
	a.Name = "example-v1"
	_ = recordValidation(dir, a)
	a.Name = "example-v2"
	_ = recordValidation(dir, a)
	if !TwiceFailed(validationRecords(dir)) {
		t.Fatal("missing review")
	}
	if err := resetValidation(dir); err != nil {
		t.Fatal(err)
	}
	a.Name = "example-v4"
	_ = recordValidation(dir, a)
	if TwiceFailed(validationRecords(dir)) {
		t.Fatal("human rework did not reset")
	}
}
func TestReview4NormalizedFailureIdentity(t *testing.T) {
	if !TwiceFailed([]ValidationRecord{failed("click  the\nbutton"), failed("click the button")}) {
		t.Fatal("whitespace bypass")
	}
}
func TestReview5NotifyErrorPreservesVerdict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = os.MkdirAll(Root(), 0700)
	dir := filepath.Join(Root(), "example")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{Head: "new", Validation: "pass", NeedsReview: true})
	if err := setNotifyError(dir, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	var r Result
	_ = readJSON(filepath.Join(dir, "result.json"), &r)
	if r.Head != "new" || r.Validation != "pass" || !r.NeedsReview || r.NotifyError == "" {
		t.Fatalf("%+v", r)
	}
}
func TestReview6CallbackCommitsBeforeNotificationFailure(t *testing.T) {
	original := validationNotificationLauncher
	launched := false
	validationNotificationLauncher = func(dir string, m Meta) { launched = true }
	t.Cleanup(func() { validationNotificationLauncher = original })
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(Root(), "example")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}, ConfigPath: filepath.Join(dir, "missing")})
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{Head: "sha", End: time.Now()})
	r := failed("click")
	r.Name = "example-v1"
	r.State = "fail"
	r.Revision = "sha"
	if _, err := Handle(Request{Action: "validation-result", Name: "example", ValidationRecord: r}); err != nil {
		t.Fatal(err)
	}
	if !launched || validationRecords(dir)[0].State != "fail" {
		t.Fatal("not committed before notification launch")
	}
	notifyValidation(dir, Meta{ConfigPath: filepath.Join(dir, "missing")})
	var result Result
	_ = readJSON(filepath.Join(dir, "result.json"), &result)
	if result.NotifyError == "" {
		t.Fatal("notify failure not recorded")
	}
}
func TestReview7MissingValidatorRouteFailsClearly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(cfg, []byte(""), 0600)
	_, err := Handle(Request{Action: "start", Name: "example-v1", Cap: 100, Order: "---\nname: example\nrepo: project\n---\n## Acceptance\n- Click.\n", Meta: Meta{Parent: "example", ParentHostName: "source", ConfigPath: cfg, Order: Order{Name: "example-v1"}}})
	if err == nil || !strings.Contains(err.Error(), "no parent route") {
		t.Fatalf("%v", err)
	}
	var result Result
	if e := readJSON(filepath.Join(Root(), "example-v1", "result.json"), &result); e != nil || result.Validation != "fail" || !strings.Contains(result.Error, "source") {
		t.Fatalf("missing local terminal failure: %+v %v", result, e)
	}
	rows, e := Handle(Request{Action: "list"})
	if e != nil || len(rows.Rows) != 1 || !VisibleByDefault(rows.Rows[0]) {
		t.Fatalf("failure hidden: %+v %v", rows, e)
	}
}
func TestReview8DefaultVisibility(t *testing.T) {
	for _, row := range []Row{{State: "running"}, {State: "died"}, {State: "needs-review"}, {State: "done", Validation: "pending"}, {State: "done", Validation: "running"}, {State: "done", Validation: "fail"}} {
		if !VisibleByDefault(row) {
			t.Fatalf("hidden %+v", row)
		}
	}
	if VisibleByDefault(Row{State: "done", Validation: "pass"}) {
		t.Fatal("completed visible")
	}
}
func TestReview9ExplicitSourceSkipsInventory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(Root(), "example")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example", Validation: "none"}})
	c := Config{Hosts: map[string]Host{"source": {Local: true}, "duplicate": {Local: true}, "validator": {Local: true}}, Validation: ValidationConfig{Host: "validator", AllowSameHost: true}}
	_, err := validateFrom(context.Background(), c, "example", "", false, "source")
	if err == nil || !strings.Contains(err.Error(), "browser implementation") {
		t.Fatalf("%v", err)
	}
}

func TestReview7ValidatorUsesItsOwnParentAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(cfg, []byte("[hosts.source]\nssh = 'validator-side-alias'\ncap = 1\n"), 0600)
	_, err := Handle(Request{Action: "start", Name: "example-v1", Cap: 100, Order: "---\nname: example\nrepo: project\n---\n## Acceptance\n- Click.\n", Meta: Meta{Parent: "example", ParentHostName: "source", ParentHost: Host{SSH: "caller-side-alias"}, ConfigPath: cfg, Order: Order{Name: "example-v1", Repo: filepath.Join(t.TempDir(), "missing-repo")}}})
	if err == nil {
		t.Fatal("expected missing scratch repo")
	}
	var m Meta
	if err = readJSON(filepath.Join(Root(), "example-v1", "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	if m.ParentHost.SSH != "validator-side-alias" {
		t.Fatalf("caller alias leaked: %+v", m.ParentHost)
	}
}

func TestReworkSameHostRequiresExplicitOptIn(t *testing.T) {
	for _, target := range []string{"source", "validator"} {
		c := Config{Hosts: map[string]Host{"source": {Local: true}, "validator": {Local: true}}, Validation: ValidationConfig{Host: target}}
		_, err := validateFrom(context.Background(), c, "example", "", false, "source")
		if err == nil || !strings.Contains(err.Error(), "host") {
			t.Fatalf("same-host accepted: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(path, []byte("[validation]\nallow_same_host = true\n"), 0600)
	cfg, err := Load(path)
	if err != nil || !cfg.Validation.AllowSameHost {
		t.Fatalf("opt-in not loaded: %+v %v", cfg, err)
	}
}
func TestReworkRejectedAdmissionPreservesStreak(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(Root(), "example")
	_ = os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}})
	for _, name := range []string{"v1", "v2"} {
		r := failed("click")
		r.Name = name
		_ = recordValidation(dir, r)
	}
	// Force reservation failure after the note and result renames.
	_ = os.WriteFile(filepath.Join(dir, "order.md"), []byte("order"), 0600)
	_ = os.Mkdir(filepath.Join(dir, "admission.json"), 0700)
	_, err := Handle(Request{Action: "rework", Name: "example", Note: "fix", Cap: 100})
	if err == nil || !TwiceFailed(validationRecords(dir)) {
		t.Fatalf("rejected rework reset failures: %v", err)
	}
}
