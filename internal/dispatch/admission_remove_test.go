package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRemoveAdmissionReposPreservesFile(t *testing.T) {
	const id = "github.com/ExampleOrg/gone"
	for name, tc := range map[string]struct{ in, want string }{
		"inline-first": {
			"# owner policy\n[admission]\nrepos = [\"https://github.com/ExampleOrg/gone\", \"https://github.com/ExampleOrg/a\"] # keep\nadd_from = [\"https://github.com/ExampleOrg/\"]\n",
			"# owner policy\n[admission]\nrepos = [\"https://github.com/ExampleOrg/a\"] # keep\nadd_from = [\"https://github.com/ExampleOrg/\"]\n",
		},
		"inline-last": {
			"[admission]\nrepos = [\"https://github.com/ExampleOrg/a\", 'git@github.com:ExampleOrg/gone.git']\n",
			"[admission]\nrepos = [\"https://github.com/ExampleOrg/a\"]\n",
		},
		"only": {
			"[admission]\nrepos = [\"https://github.com/ExampleOrg/gone\"] # list\n",
			"[admission]\nrepos = [] # list\n",
		},
		"multiline-with-comments": {
			"[admission]\nrepos = [\n    \"https://github.com/ExampleOrg/a\", # first\n    \"https://github.com/ExampleOrg/gone\", # added by alex\n    'https://github.com/ExampleOrg/b',\n] # end\n[hosts.x]\nssh = \"x\"\n",
			"[admission]\nrepos = [\n    \"https://github.com/ExampleOrg/a\", # first\n    'https://github.com/ExampleOrg/b',\n] # end\n[hosts.x]\nssh = \"x\"\n",
		},
		"multiline-last-no-comma": {
			"[admission]\nrepos = [\n  \"https://github.com/ExampleOrg/a\",\n  \"https://github.com/ExampleOrg/gone\"\n]\n",
			"[admission]\nrepos = [\n  \"https://github.com/ExampleOrg/a\",\n]\n",
		},
		"duplicates-and-crlf": {
			"[admission]\r\nrepos = [\"ssh://git@github.com/ExampleOrg/gone\",\r\n  \"https://github.com/ExampleOrg/a\",\r\n  \"https://github.com/ExampleOrg/gone.git\",\r\n]\r\n",
			"[admission]\r\nrepos = [\r\n  \"https://github.com/ExampleOrg/a\",\r\n]\r\n",
		},
		"decoy-elsewhere": {
			"notify = [\"\"\"\n[admission]\nrepos = [\"https://github.com/ExampleOrg/gone\"]\n\"\"\"]\n[admission]\nrepos = [\"https://github.com/ExampleOrg/gone\", \"https://github.com/ExampleOrg/gone-too\"]\n",
			"notify = [\"\"\"\n[admission]\nrepos = [\"https://github.com/ExampleOrg/gone\"]\n\"\"\"]\n[admission]\nrepos = [\"https://github.com/ExampleOrg/gone-too\"]\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := removeAdmissionRepos(tc.in, id)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
	for name, in := range map[string]string{
		"dotted":       "admission.repos = [\"https://github.com/ExampleOrg/gone\"]\n",
		"inline-table": "admission = { repos = [\"https://github.com/ExampleOrg/gone\"] }\n",
	} {
		if got, err := removeAdmissionRepos(in, id); err == nil {
			t.Errorf("%s: edited unsupported form: %q", name, got)
		}
	}
}

func TestRemoveRepoGrantAddFromAttributionIdempotency(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	cfg, workspace := repoTestHost(t, "[admission]\nrepos = [\n  \"https://example.test/Elsewhere/tool\", # owner\n  \"https://github.com/ExampleOrg/kit\",\n]\nadd_from = [\"https://github.com/ExampleOrg/\"]\n")
	teammate := RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}
	if _, err := AddRepo(context.Background(), "https://github.com/ExampleOrg/widget.git", teammate); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cfg)

	// No capability, outside add_from, and hostile spellings: refused, unchanged.
	if _, err := RemoveRepo("https://github.com/ExampleOrg/widget", RepoCaller{Sender: "alex", Restricted: true}); err == nil || !strings.Contains(err.Error(), "your access does not include removing repos") {
		t.Fatalf("expected capability refusal: %v", err)
	}
	if _, err := RemoveRepo("https://example.test/Elsewhere/tool", teammate); err == nil || !strings.Contains(err.Error(), "was not added from an allowed organization; ask the owner") || strings.Contains(err.Error(), "your access") {
		t.Fatalf("expected add_from refusal: %v", err)
	}
	for _, hostile := range []string{"https://github.com/ExampleOrg.evil/widget", "https://user:pw@github.com/ExampleOrg/widget", "file:///tmp/widget", "https://github.com/ExampleOrg/../Other/widget", ""} {
		if _, err := RemoveRepo(hostile, teammate); err == nil {
			t.Fatalf("removed %q", hostile)
		}
	}
	if after, _ := os.ReadFile(cfg); string(after) != string(before) {
		t.Fatalf("refusals changed config:\n%s", after)
	}

	// Any spelling of an admitted origin removes it; the clone stays.
	r, err := RemoveRepo("git@github.com:ExampleOrg/widget", teammate)
	if err != nil || !strings.Contains(r.Message, "removed https://github.com/ExampleOrg/widget.git") || !strings.Contains(r.Message, "clone widget stays") || !strings.Contains(r.Message, "keep running") {
		t.Fatalf("%+v %v", r, err)
	}
	after, _ := os.ReadFile(cfg)
	if want := strings.Replace(string(before), "  \"https://github.com/ExampleOrg/widget.git\",\n", "", 1); string(after) != want {
		t.Fatalf("admission file not preserved:\n%s\nwant\n%s", after, want)
	}
	if _, err = os.Stat(filepath.Join(workspace, "widget", ".git")); err != nil {
		t.Fatalf("clone removed: %v", err)
	}
	if err = admitTargetRepo(filepath.Join(workspace, "widget"), "widget", "here"); err == nil {
		t.Fatal("removed repo still admitted")
	}

	// Not admitted: success, no change, no log entry.
	r, err = RemoveRepo("https://github.com/ExampleOrg/widget", teammate)
	if err != nil || !strings.Contains(r.Message, "not admitted; nothing to remove") {
		t.Fatalf("%+v %v", r, err)
	}
	if again, _ := os.ReadFile(cfg); string(again) != string(after) {
		t.Fatal("no-op removal edited config")
	}
	ledger := readRepoLedger(cfg)
	if len(ledger) != 2 || ledger[1].Action != "remove" || ledger[1].RemovedBy != "alex" || ledger[1].RemovedAt.IsZero() || ledger[1].Origin != "https://github.com/ExampleOrg/widget.git" {
		t.Fatalf("log %+v", ledger)
	}

	// Owners remove outside add_from, and entries no teammate could spell.
	if _, err = RemoveRepo("https://example.test/Elsewhere/tool.git", RepoCaller{Sender: "local:owner"}); err != nil {
		t.Fatal(err)
	}
	if repos := decodedRepos(t, cfg); len(repos) != 1 || repos[0] != "https://github.com/ExampleOrg/kit" {
		t.Fatalf("repos %v", repos)
	}
	if last := readRepoLedger(cfg); last[len(last)-1].RemovedBy != "local:owner" {
		t.Fatalf("owner removal not attributed: %+v", last)
	}

	// Re-adding attributes the new addition; the removal clears the old one.
	if _, err = AddRepo(context.Background(), "https://github.com/ExampleOrg/widget.git", RepoCaller{Sender: "sam", Restricted: true, CanAdd: true}); err != nil {
		t.Fatal(err)
	}
	ls, _ := ListRepos(teammate)
	if p := ls.RepoPolicy; len(p.Repos) != 2 || p.Repos[1].AddedBy != "sam" {
		t.Fatalf("ls %+v", p)
	}
	if calls.Load() != 1 {
		t.Fatalf("re-add cloned again: %d", calls.Load())
	}
}

func TestRemoveRepoOwnerLocalOrigin(t *testing.T) {
	cfg, _ := repoTestHost(t, "[admission]\nrepos = [\"/srv/git/local-only\", \"https://github.com/ExampleOrg/kit\"]\n")
	if _, err := RemoveRepo("/srv/git/local-only", RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}); err == nil {
		t.Fatal("teammate removed a local path entry")
	}
	if _, err := RemoveRepo("/srv/git/local-only", RepoCaller{Sender: "local:owner"}); err != nil {
		t.Fatal(err)
	}
	if repos := decodedRepos(t, cfg); len(repos) != 1 {
		t.Fatalf("repos %v", repos)
	}
}

func TestRemoveRepoAdmitAllHost(t *testing.T) {
	for _, admission := range []string{"", "[admission]\nadd_from = [\"https://github.com/ExampleOrg/\"]\n"} {
		cfg, _ := repoTestHost(t, admission)
		before, _ := os.ReadFile(cfg)
		callers := []RepoCaller{{Sender: "local:owner"}}
		if admission != "" {
			callers = append(callers, RepoCaller{Sender: "alex", Restricted: true, CanAdd: true})
		}
		for _, caller := range callers {
			if _, err := RemoveRepo("https://github.com/ExampleOrg/kit", caller); err == nil || !strings.Contains(err.Error(), "admits every workspace repository") {
				t.Fatalf("%q %+v: %v", admission, caller, err)
			}
		}
		if after, _ := os.ReadFile(cfg); string(after) != string(before) {
			t.Fatalf("admit-all host edited: %s", after)
		}
	}
}

func TestConcurrentAddRemoveRepo(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	initial := []string{}
	for i := 0; i < 8; i++ {
		initial = append(initial, fmt.Sprintf("  \"https://github.com/ExampleOrg/old%d\", # seed %d\n", i, i))
	}
	cfg, _ := repoTestHost(t, "[admission]\nadd_from = [\"https://github.com/ExampleOrg/\"]\nrepos = [\n"+strings.Join(initial, "")+"]\n")
	caller := RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			_, err := RemoveRepo(fmt.Sprintf("https://github.com/ExampleOrg/old%d", i), caller)
			errs <- err
		}(i)
		go func(i int) {
			defer wg.Done()
			_, err := AddRepo(context.Background(), fmt.Sprintf("https://github.com/ExampleOrg/new%d", i), caller)
			errs <- err
		}(i)
		go func() {
			defer wg.Done()
			_, err := RemoveRepo("git@github.com:ExampleOrg/old0.git", caller)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	repos := decodedRepos(t, cfg)
	if len(repos) != 8 {
		t.Fatalf("repos=%v", repos)
	}
	for _, r := range repos {
		if !strings.Contains(r, "/new") {
			t.Fatalf("repos=%v", repos)
		}
	}
	removals := 0
	for _, e := range readRepoLedger(cfg) {
		if e.Action == "remove" {
			removals++
		}
	}
	if removals != 8 {
		t.Fatalf("removals logged %d", removals)
	}
	if text, _ := os.ReadFile(cfg); strings.Contains(string(text), "# seed") {
		t.Fatalf("removed lines left comments:\n%s", text)
	}
}

func TestHandleGrantRemove(t *testing.T) {
	cfg, _ := repoTestHost(t, "[admission]\nrepos = [\"https://github.com/ExampleOrg/widget\", \"https://example.test/Other/tool\"]\nadd_from = [\"https://github.com/ExampleOrg/\"]\n")
	if _, err := HandleGrant(Request{Action: "admission-remove", URL: "https://github.com/ExampleOrg/widget"}, "alex", nil); err == nil {
		t.Fatal("removed without capability")
	}
	// Forged owner fields do not lift the add_from rule.
	q := Request{Action: "admission-remove", URL: "https://example.test/Other/tool", Meta: Meta{Sender: "local:owner", ConfigPath: cfg}}
	if _, err := HandleGrant(q, "alex", []string{RepoCapability}); err == nil || !strings.Contains(err.Error(), "not added from an allowed organization") {
		t.Fatalf("forged owner fields: %v", err)
	}
	q.URL = "https://github.com/ExampleOrg/widget"
	if _, err := HandleGrant(q, "alex", []string{RepoCapability}); err != nil {
		t.Fatal(err)
	}
	if ledger := readRepoLedger(cfg); len(ledger) != 1 || ledger[0].RemovedBy != "alex" {
		t.Fatalf("attribution %+v", ledger)
	}
	if repos := decodedRepos(t, cfg); len(repos) != 1 || repos[0] != "https://example.test/Other/tool" {
		t.Fatalf("repos %v", repos)
	}
}
