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

	"github.com/BurntSushi/toml"
)

func TestParseAddURL(t *testing.T) {
	for raw, want := range map[string][3]string{
		"https://github.com/ExampleOrg/widget.git":                     {"https://github.com/ExampleOrg/widget.git", "github.com/ExampleOrg/widget", "widget"},
		"https://GitHub.com/ExampleOrg/widget/":                        {"https://github.com/ExampleOrg/widget", "github.com/ExampleOrg/widget", "widget"},
		"git@github.com:ExampleOrg/widget.git":                         {"git@github.com:ExampleOrg/widget.git", "github.com/ExampleOrg/widget", "widget"},
		"ssh://git@example.test:2222/ExampleOrg/widget":                {"ssh://git@example.test:2222/ExampleOrg/widget", "example.test:2222/ExampleOrg/widget", "widget"},
		"https://dev.example.test/ExampleOrg/Proj/_git/Repo":           {"https://dev.example.test/ExampleOrg/Proj/_git/Repo", "dev.example.test/ExampleOrg/Proj/_git/Repo", "Repo"},
		"https://dev.example.test/ExampleOrg/My%20Proj/_git/My%20Repo": {"https://dev.example.test/ExampleOrg/My%20Proj/_git/My%20Repo", "dev.example.test/ExampleOrg/My Proj/_git/My Repo", "My-Repo"},
	} {
		a, err := parseAddURL(raw, true)
		if err != nil {
			t.Errorf("%s: %v", raw, err)
			continue
		}
		if got := [3]string{a.Clone, a.Identity, a.Name}; got != want {
			t.Errorf("%s: got %q want %q", raw, got, want)
		}
	}
	for _, raw := range []string{
		"",
		"file:///srv/git/widget.git",
		"/srv/git/widget.git",
		"./widget",
		"../widget",
		`C:\src\widget`,
		"C:/src/widget",
		"c:widget",
		"http://github.com/ExampleOrg/widget",
		"git://github.com/ExampleOrg/widget",
		"ext::sh -c touch% /tmp/pwned",
		"fd::17",
		"https::github.com/ExampleOrg/widget",
		"https://user:secret@github.com/ExampleOrg/widget",
		"https://token@github.com/ExampleOrg/widget",
		"https://github.com@evil.example.test/ExampleOrg/widget",
		"ssh://git:secret@github.com/ExampleOrg/widget",
		"ssh://-oProxyCommand=touch%20x/widget",
		"-uhttps://github.com/ExampleOrg/widget",
		"--upload-pack=touch x",
		"https://github.com/ExampleOrg/../Other/widget",
		"https://github.com/ExampleOrg/./widget",
		"https://github.com/ExampleOrg/%2e%2e/widget",
		"https://github.com/ExampleOrg%2fOther/widget",
		"https://github.com/ExampleOrg//widget",
		"https://github.com/ExampleOrg/widget?ref=main",
		"https://github.com/ExampleOrg/widget#main",
		"https://github.com/ExampleOrg/widget branch",
		"https://github.com/ExampleOrg/wid\nget",
		"https://github.com/ExampleOrg/.git",
		"https://github.com/ExampleOrg/-widget",
		"https://github.com/ExampleOrg/CON",
		"https://github.com/",
		"https://[::1]/ExampleOrg/widget",
		"https://github.com:/ExampleOrg/widget",
		"git@github.com:ExampleOrg/wid%20get",
		"-oProxyCommand=x@github.com:ExampleOrg/widget",
		"https://github.com/ExampleOrg/$(id)",
		"https://github.com/ExampleOrg/wïdget",
	} {
		if a, err := parseAddURL(raw, true); err == nil {
			t.Errorf("accepted %q as %+v", raw, a)
		}
	}
}

func TestAddFromPrefixMatching(t *testing.T) {
	addFrom := []string{"https://github.com/ExampleOrg/", "https://dev.example.test/ExampleOrg/Proj", "git@git.example.test:SshOrg", "not a url", "file:///srv/"}
	for raw, allowed := range map[string]bool{
		"https://github.com/ExampleOrg/widget":              true,
		"git@github.com:ExampleOrg/widget.git":              false, // https entry: no SSH keys
		"ssh://git@github.com/ExampleOrg/widget":            false,
		"github.com:ExampleOrg/widget":                      false,
		"https://github.com/ExampleOrg":                     false, // the organization itself
		"https://github.com/ExampleOrg.git":                 false,
		"https://github.com/ExampleOrg/":                    false,
		"https://dev.example.test/ExampleOrg/Proj":          false,
		"git@git.example.test:SshOrg/widget.git":            true,
		"ssh://git@git.example.test/SshOrg/widget":          true,
		"https://git.example.test/SshOrg/widget":            false, // ssh entry: no HTTPS credential
		"ssh://other@git.example.test/SshOrg/widget":        false,
		"git.example.test:SshOrg/widget":                    false,
		"git@git.example.test:SshOrg":                       false,
		"https://GITHUB.com/ExampleOrg/widget":              true,
		"https://dev.example.test/ExampleOrg/Proj/_git/Rep": true,
		"https://github.com/ExampleOrg.evil/widget":         false,
		"https://github.com/ExampleOrgX/widget":             false,
		"https://github.com/exampleorg/widget":              false,
		"https://github.com.evil.example.test/ExampleOrg/w": false,
		"https://evil.example.test/github.com/ExampleOrg/w": false,
		"ssh://git@github.com:2222/ExampleOrg/widget":       false,
		"https://dev.example.test/ExampleOrg/Proj2/_git/R":  false,
		"https://dev.example.test/ExampleOrg/Other/_git/R":  false,
	} {
		a, err := parseAddURL(raw, true)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := addFromAllows(addFrom, a); got != allowed {
			t.Errorf("%s: allowed=%v", raw, got)
		}
	}
	a, _ := parseAddURL("https://github.com/ExampleOrg/widget", true)
	for _, none := range [][]string{nil, {}, {""}, {"file:///"}} {
		if addFromAllows(none, a) {
			t.Errorf("%q allowed an add", none)
		}
	}
	if !addFromAllows([]string{"https://github.com/"}, a) {
		t.Error("host-wide prefix refused")
	}
}

func TestAppendAdmissionRepoPreservesFile(t *testing.T) {
	const origin = "https://github.com/ExampleOrg/new"
	for name, tc := range map[string]struct{ in, want string }{
		"inline": {
			"# owner policy\ncap = 3 # slots\n\n[admission]\nrepos = [\"https://github.com/ExampleOrg/a\"] # keep\nadd_from = [\"https://github.com/ExampleOrg/\"]\n\n[hosts.box]\nlocal = true\n",
			"# owner policy\ncap = 3 # slots\n\n[admission]\nrepos = [\"https://github.com/ExampleOrg/a\", \"https://github.com/ExampleOrg/new\"] # keep\nadd_from = [\"https://github.com/ExampleOrg/\"]\n\n[hosts.box]\nlocal = true\n",
		},
		"empty": {
			"[admission]\nrepos = []\n",
			"[admission]\nrepos = [\"https://github.com/ExampleOrg/new\"]\n",
		},
		"multiline-trailing-comma": {
			"[admission]\nrepos = [\n    \"https://github.com/ExampleOrg/a\", # first\n    'https://github.com/ExampleOrg/b',\n] # end\n[hosts.x]\nssh = \"x\"\n",
			"[admission]\nrepos = [\n    \"https://github.com/ExampleOrg/a\", # first\n    'https://github.com/ExampleOrg/b',\n    \"https://github.com/ExampleOrg/new\",\n] # end\n[hosts.x]\nssh = \"x\"\n",
		},
		"multiline-no-comma-comment": {
			"[admission]\nrepos = [\n  \"https://github.com/ExampleOrg/a\" # first ] [\n]\n",
			"[admission]\nrepos = [\n  \"https://github.com/ExampleOrg/a\", # first ] [\n  \"https://github.com/ExampleOrg/new\"\n]\n",
		},
		"multiline-empty": {
			"[admission]\nrepos = [\n  # none yet\n]\n",
			"[admission]\nrepos = [\n  \"https://github.com/ExampleOrg/new\",\n  # none yet\n]\n",
		},
		"crlf-and-decoys": {
			"notify = [\"x\", \"\"\"\n[admission]\nrepos = [\"decoy\"]\n\"\"\"]\r\n[admission]\r\nadd_from = [\"https://github.com/ExampleOrg/\"]\r\nrepos = [\"https://github.com/ExampleOrg/a\",\r\n]\r\n",
			"notify = [\"x\", \"\"\"\n[admission]\nrepos = [\"decoy\"]\n\"\"\"]\r\n[admission]\r\nadd_from = [\"https://github.com/ExampleOrg/\"]\r\nrepos = [\"https://github.com/ExampleOrg/a\",\r\n\"https://github.com/ExampleOrg/new\",\r\n]\r\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := appendAdmissionRepo(tc.in, origin)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
	for name, in := range map[string]string{
		"dotted":       "admission.repos = []\n",
		"inline-table": "admission = { repos = [] }\n",
		"quoted-key":   "[admission]\n\"repos\" = []\n",
		"no-repos":     "[admission]\nadd_from = []\n",
	} {
		if got, err := appendAdmissionRepo(in, origin); err == nil {
			t.Errorf("%s: edited unsupported form: %q", name, got)
		}
	}
}

// fakeCloner creates a repository with the requested origin and counts clones.
func fakeCloner(t *testing.T, calls *atomic.Int32) {
	t.Helper()
	previous := repoCloner
	repoCloner = func(_ context.Context, url, dest string) error {
		calls.Add(1)
		if _, err := command("", "git", "init", "-q", dest); err != nil {
			return err
		}
		_, err := command("", "git", "-C", dest, "remote", "add", "origin", url)
		return err
	}
	t.Cleanup(func() { repoCloner = previous })
}

func repoTestHost(t *testing.T, admission string) (cfg, workspace string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workspace = filepath.Join(home, "workspace")
	cfg = filepath.Join(home, "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	body := "# host policy\ncap = 2\nworkspace = " + fmt.Sprintf("%q", filepath.ToSlash(workspace)) + "\n" + admission + "\n[hosts.buildbox] # this host\nlocal = true\n"
	if err := os.WriteFile(cfg, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return cfg, workspace
}

func decodedRepos(t *testing.T, cfg string) []string {
	t.Helper()
	var c struct{ Admission Admission }
	if _, err := toml.DecodeFile(cfg, &c); err != nil {
		t.Fatal(err)
	}
	return c.Admission.Repos
}

func TestAddRepoGrantAddFromAttributionIdempotency(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	cfg, workspace := repoTestHost(t, "[admission]\nrepos = [\"https://github.com/ExampleOrg/kit\"] # owner list\nadd_from = [\"https://github.com/ExampleOrg/\", \"git@github.com:ExampleOrg\"]\n")
	before, _ := os.ReadFile(cfg)
	ctx := context.Background()
	allowed := "https://github.com/ExampleOrg/widget.git"

	// No capability: refused before any clone or edit.
	if _, err := AddRepo(ctx, allowed, RepoCaller{Sender: "alex", Restricted: true}); err == nil || !strings.Contains(err.Error(), "your access does not include adding repos") {
		t.Fatalf("expected capability refusal: %v", err)
	}
	// Outside add_from: refused, nothing cloned.
	for _, outside := range []string{"https://github.com/ExampleOrg.evil/widget", "https://github.com/Other/widget", "file:///tmp/widget"} {
		if _, err := AddRepo(ctx, outside, RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}); err == nil {
			t.Fatalf("added %s", outside)
		}
	}
	if after, _ := os.ReadFile(cfg); string(after) != string(before) || calls.Load() != 0 {
		t.Fatalf("refusals changed state: clones=%d\n%s", calls.Load(), after)
	}
	if entries, _ := os.ReadDir(workspace); len(entries) != 0 {
		t.Fatalf("refusals left workspace entries: %v", entries)
	}

	r, err := AddRepo(ctx, allowed, RepoCaller{Sender: "alex", Restricted: true, CanAdd: true})
	if err != nil || !strings.Contains(r.Message, "repo: widget") {
		t.Fatalf("%+v %v", r, err)
	}
	after, _ := os.ReadFile(cfg)
	want := strings.Replace(string(before), `"https://github.com/ExampleOrg/kit"]`, `"https://github.com/ExampleOrg/kit", "https://github.com/ExampleOrg/widget.git"]`, 1)
	if string(after) != want {
		t.Fatalf("admission file not preserved:\n%s", after)
	}
	if got := origin(t, filepath.Join(workspace, "widget")); got != allowed {
		t.Fatalf("origin %q", got)
	}
	// Equivalent spellings are idempotent: no second clone, no second entry.
	for _, again := range []string{allowed, "git@github.com:ExampleOrg/widget.git", "https://github.com/ExampleOrg/widget"} {
		if _, err := AddRepo(ctx, again, RepoCaller{Sender: "sam", Restricted: true, CanAdd: true}); err != nil {
			t.Fatal(err)
		}
	}
	if repos := decodedRepos(t, cfg); len(repos) != 2 || calls.Load() != 1 {
		t.Fatalf("repos=%v clones=%d", repos, calls.Load())
	}
	ls, err := ListRepos(RepoCaller{Sender: "sam", Restricted: true})
	if err != nil {
		t.Fatal(err)
	}
	p := ls.RepoPolicy
	if p.AdmitAll || p.CanAdd || len(p.Repos) != 2 || p.Repos[1].AddedBy != "alex" || p.Repos[1].AddedAt.IsZero() || p.Repos[1].Name != "widget" || p.Repos[0].AddedBy != "" {
		t.Fatalf("ls %+v", p)
	}

	// Owners skip the grant and add_from checks, and record their identity.
	if _, err = AddRepo(ctx, "https://example.test/Elsewhere/tool", RepoCaller{Sender: "local:owner"}); err != nil {
		t.Fatal(err)
	}
	ls, _ = ListRepos(RepoCaller{Sender: "local:owner"})
	if last := ls.RepoPolicy.Repos[len(ls.RepoPolicy.Repos)-1]; last.AddedBy != "local:owner" || last.Name != "tool" || !ls.RepoPolicy.CanAdd {
		t.Fatalf("owner add %+v", ls.RepoPolicy)
	}

	// A different origin under an existing name is never replaced.
	if _, err = AddRepo(ctx, "https://github.com/ExampleOrg/sub/widget", RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}); err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("expected name conflict: %v", err)
	}
}

func origin(t *testing.T, repo string) string {
	t.Helper()
	got, err := command("", "git", "-C", repo, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAddRepoWithoutAddFromOrAdmissionList(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	for _, admission := range []string{"", "[admission]\nrepos = []\n", "[admission]\nrepos = []\nadd_from = []\n"} {
		cfg, _ := repoTestHost(t, admission)
		calls.Store(0)
		if _, err := AddRepo(context.Background(), "https://github.com/ExampleOrg/widget", RepoCaller{Sender: "alex", Restricted: true, CanAdd: true}); err == nil || !strings.Contains(err.Error(), "not in an allowed organization") {
			t.Fatalf("%q: self-add without add_from: %v", admission, err)
		}
		if calls.Load() != 0 {
			t.Fatal("cloned without add_from")
		}
		// An absent repos list admits everything; owners clone without editing.
		if admission == "" {
			before, _ := os.ReadFile(cfg)
			if _, err := AddRepo(context.Background(), "https://github.com/ExampleOrg/widget", RepoCaller{Sender: "local:owner"}); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(cfg)
			ls, _ := ListRepos(RepoCaller{Sender: "alex", Restricted: true, CanAdd: true})
			if string(before) != string(after) || !ls.RepoPolicy.AdmitAll || ls.RepoPolicy.CanAdd {
				t.Fatalf("admit-all host edited or misreported: %s %+v", after, ls.RepoPolicy)
			}
		}
	}
}

func TestConcurrentAddRepo(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	cfg, _ := repoTestHost(t, "[admission]\nadd_from = [\"https://github.com/ExampleOrg/\", \"git@github.com:ExampleOrg\"]\nrepos = [\n  \"https://github.com/ExampleOrg/kit\",\n]\n")
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 8; i++ {
		for _, url := range []string{fmt.Sprintf("https://github.com/ExampleOrg/r%d", i), "https://github.com/ExampleOrg/shared", "git@github.com:ExampleOrg/shared.git"} {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				_, err := AddRepo(context.Background(), url, RepoCaller{Sender: "alex", Restricted: true, CanAdd: true})
				errs <- err
			}(url)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	repos := decodedRepos(t, cfg)
	seen := map[string]int{}
	for _, r := range repos {
		seen[repoIdentity(r)]++
	}
	if len(repos) != 10 || seen["github.com/ExampleOrg/shared"] != 1 || seen["github.com/ExampleOrg/kit"] != 1 {
		t.Fatalf("repos=%v", repos)
	}
	if n := len(readRepoLedger(cfg)); n != 9 {
		t.Fatalf("ledger entries %d", n)
	}
}

func TestHandleGrantRepoActions(t *testing.T) {
	var calls atomic.Int32
	fakeCloner(t, &calls)
	cfg, _ := repoTestHost(t, "[admission]\nrepos = []\nadd_from = [\"https://github.com/ExampleOrg/\"]\n")
	if _, err := HandleGrant(Request{Action: "admission-ls"}, "", nil); err == nil {
		t.Fatal("anonymous caller")
	}
	if r, err := HandleGrant(Request{Action: "admission-ls"}, "alex", nil); err != nil || r.RepoPolicy == nil || r.RepoPolicy.CanAdd {
		t.Fatalf("%+v %v", r, err)
	}
	// Request fields other than the URL cannot widen a restricted add.
	q := Request{Action: "admission-add", URL: "https://example.test/Other/widget", Workspace: "/tmp", Meta: Meta{Sender: "local:owner", ConfigPath: cfg}}
	if _, err := HandleGrant(q, "alex", []string{RepoCapability}); err == nil {
		t.Fatal("forged owner fields bypassed add_from")
	}
	q.URL = "https://github.com/ExampleOrg/widget"
	if _, err := HandleGrant(q, "alex", nil); err == nil {
		t.Fatal("added without capability")
	}
	if _, err := HandleGrant(q, "alex", []string{"other", RepoCapability}); err != nil {
		t.Fatal(err)
	}
	if ledger := readRepoLedger(cfg); len(ledger) != 1 || ledger[0].AddedBy != "alex" {
		t.Fatalf("attribution %+v", ledger)
	}
	// The capability adds nothing to other teammate actions.
	if _, err := HandleGrant(Request{Action: "clean", Name: "x"}, "alex", []string{RepoCapability}); err == nil {
		t.Fatal("capability widened run actions")
	}
}

// Teammates get a short refusal when git fails; its output stays in a host log.
func TestAddRepoHidesCloneOutputFromTeammates(t *testing.T) {
	previous := repoCloner
	t.Cleanup(func() { repoCloner = previous })
	const secret = "fatal: credential helper /srv/host-only/helper said no"
	repoCloner = func(context.Context, string, string) error {
		return &cloneError{Summary: "git clone failed", Detail: "exit status 128: " + secret}
	}
	repoTestHost(t, "[admission]\nrepos = []\nadd_from = [\"https://github.com/ExampleOrg/\"]\n")
	_, err := AddRepo(context.Background(), "https://github.com/ExampleOrg/widget", RepoCaller{Sender: "alex", Restricted: true, CanAdd: true})
	if err == nil || strings.Contains(err.Error(), "host-only") || !strings.Contains(err.Error(), "git clone failed") || !strings.Contains(err.Error(), "admission-clone.log") {
		t.Fatalf("teammate refusal: %v", err)
	}
	if b, _ := os.ReadFile(cloneLogPath()); !strings.Contains(string(b), secret) || !strings.Contains(string(b), "alex") {
		t.Fatalf("host log: %q", b)
	}
	// Owners see git's own words.
	if _, err = AddRepo(context.Background(), "https://github.com/ExampleOrg/widget", RepoCaller{Sender: "local:owner"}); err == nil || !strings.Contains(err.Error(), secret) {
		t.Fatalf("owner error: %v", err)
	}
}
