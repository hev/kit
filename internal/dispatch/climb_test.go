package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// climbRefusedFixture is every refused name a host might hold: the fixed
// list and the Layer write keys and settings resolveLaunch forwards today.
var climbRefusedFixture = []string{
	"HEV_DISPATCH_CONFIG", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "HEV_CONFIG",
	"HEVLAYER_API_KEY", "HEVLAYER_API_KEY_OP_REF", "HEVLAYER_API_KEY_REF", "HEVLAYER_NAMESPACE",
	"HEVLAYER_ENDPOINT", "HEVLAYER_STORE", "LAYER_API_KEY", "LAYER_API_KEY_OP_REF",
	"LAYER_NAMESPACE", "LAYER_ENDPOINT", "LAYER_STORE", "BOAT_API_KEY",
}

// climbKeptFixture is every name a minimal worker keeps when the host sets it.
var climbKeptFixture = []string{"PATH", "LANG", "LC_ALL", "TMPDIR", "SHELL"}

func setClimbHostEnv(t *testing.T) {
	t.Helper()
	for _, n := range climbRefusedFixture {
		t.Setenv(n, "host-"+strings.ToLower(n))
	}
	for _, n := range climbKeptFixture[1:] {
		t.Setenv(n, "kept-"+strings.ToLower(n))
	}
	t.Setenv("CUSTOM_HOST_KEY", "host-custom")
}

func TestOrderEnvAndClimbClass(t *testing.T) {
	order := func(lines string) string {
		return strings.Replace(good, "repo: kit", "repo: kit"+lines, 1)
	}
	for _, tc := range []struct {
		name    string
		text    string
		env     string
		minimal bool
		refused string
	}{
		{name: "absent keeps standard", text: good},
		{name: "explicit standard", text: order("\nenv: standard"), env: EnvStandard},
		{name: "minimal", text: order("\nenv: minimal"), env: EnvMinimal, minimal: true},
		{name: "climb implies minimal", text: order("\nclass: climb"), env: EnvMinimal, minimal: true},
		{name: "climb with minimal", text: order("\nclass: climb\nenv: minimal"), env: EnvMinimal, minimal: true},
		{name: "other class stays standard", text: order("\nclass: code"), env: ""},
		{name: "climb refuses standard", text: order("\nclass: climb\nenv: standard"), refused: "class: climb requires env: minimal"},
		{name: "unknown env refused", text: order("\nenv: none"), refused: "invalid env"},
		{name: "duplicate env refused", text: order("\nenv: minimal\nenv: minimal"), refused: "duplicate key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := Parse(tc.text)
			if tc.refused != "" {
				if err == nil || !strings.Contains(err.Error(), tc.refused) {
					t.Fatalf("want refusal %q, got %v", tc.refused, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if o.Env != tc.env || o.MinimalEnv() != tc.minimal {
				t.Fatalf("env=%q minimal=%v", o.Env, o.MinimalEnv())
			}
		})
	}
}

// A minimal or climb run records only kept names and the [climb] worker_env
// allowlist for its worker; a standard run's launch environment is unchanged.
func TestResolveLaunchMinimalWorkerNames(t *testing.T) {
	setClimbHostEnv(t)
	cfg := Config{
		Hosts: map[string]Host{"here": {Env: []string{"CUSTOM_HOST_KEY", "GH_TOKEN", "SSH_AUTH_SOCK"}}},
		Climb: ClimbConfig{WorkerEnv: []string{"HEVLAYER_READ_API_KEY", "GOMODCACHE"}},
	}
	standard := Meta{Order: Order{Host: "here"}}
	if err := resolveLaunch(&standard, cfg); err != nil {
		t.Fatal(err)
	}
	if standard.WorkerEnv != nil {
		t.Fatalf("standard run recorded worker names: %v", standard.WorkerEnv)
	}
	// Standard runs keep today's forwarding, refused names included.
	for _, n := range append(slices.Clone(climbRefusedFixture), "CUSTOM_HOST_KEY", "HOME") {
		if n == "BOAT_API_KEY" || (n == "HOME" && os.Getenv("HOME") == "") {
			continue // never forwarded; HOME only when set
		}
		if !slices.Contains(standard.LaunchEnv, n) {
			t.Errorf("standard launch env lost %s: %v", n, standard.LaunchEnv)
		}
	}
	for _, order := range []Order{{Host: "here", Env: EnvMinimal}, {Host: "here", Class: ClimbClass}} {
		m := Meta{Order: order}
		if err := resolveLaunch(&m, cfg); err != nil {
			t.Fatal(err)
		}
		if !m.LaunchResolved {
			t.Fatal("minimal launch not resolved")
		}
		for _, n := range append(slices.Clone(climbRefusedFixture), "CUSTOM_HOST_KEY", "HOME", "USERPROFILE") {
			if containsFold(m.WorkerEnv, n) {
				t.Errorf("%+v: worker gets refused %s", order, n)
			}
		}
		for _, n := range append(slices.Clone(climbKeptFixture), "HEVLAYER_READ_API_KEY", "GOMODCACHE") {
			if n == "PATH" && os.Getenv("PATH") == "" {
				continue
			}
			if !slices.Contains(m.WorkerEnv, n) {
				t.Errorf("%+v: worker lost kept %s: %v", order, n, m.WorkerEnv)
			}
			// The supervisor holds what it passes on.
			if !slices.Contains(m.LaunchEnv, n) {
				t.Errorf("%+v: supervisor lacks %s", order, n)
			}
		}
	}
}

func TestClimbWorkerEnvAllowlist(t *testing.T) {
	kept := []string{"HEVLAYER_READ_API_KEY", "LAYER_READ_KEY_OP_REF", "HEVLAYER_ENDPOINT", "HEVLAYER_NAMESPACE", "GOMODCACHE", "ANTHROPIC_API_KEY", "CARGO_HOME"}
	refused := []string{
		"HEV_DISPATCH_CONFIG", "SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN", "HEV_CONFIG",
		"gh_token", "Ssh_Auth_Sock", "BOAT_API_KEY", "XDG_CONFIG_HOME", "GIT_ASKPASS",
		"HEVLAYER_API_KEY", "HEVLAYER_API_KEY_OP_REF", "HEVLAYER_API_KEY_REF", "LAYER_API_KEY",
		"LAYER_API_KEY_OP_REF", "HEVLAYER_WRITE_TOKEN", "LAYER_SECRET",
		"HOME", "USERPROFILE", "HEV_RUN_CLASS", "HEV_ORCHESTRATOR", "HEV_PEERS_STATE",
		"BAD-NAME", "",
	}
	for _, n := range kept {
		if err := checkClimbWorkerEnv([]string{n}); err != nil {
			t.Errorf("refused safe %q: %v", n, err)
		}
	}
	if err := checkClimbWorkerEnv(kept); err != nil {
		t.Errorf("refused safe list: %v", err)
	}
	for _, n := range refused {
		err := checkClimbWorkerEnv([]string{"GOMODCACHE", n})
		if err == nil {
			t.Errorf("allowed forbidden %q", n)
			continue
		}
		if !strings.Contains(err.Error(), "climb.worker_env") {
			t.Errorf("%q: unclear refusal %v", n, err)
		}
	}

	// Admission of a minimal run refuses the list; a standard run ignores it.
	setClimbHostEnv(t)
	bad := Config{Climb: ClimbConfig{WorkerEnv: []string{"GH_TOKEN"}}}
	m := Meta{Order: Order{Host: "here", Class: ClimbClass}}
	if err := resolveLaunch(&m, bad); err == nil || !strings.Contains(err.Error(), "GH_TOKEN") {
		t.Fatalf("climb admitted a forbidden worker_env: %v", err)
	}
	if m.LaunchResolved {
		t.Fatal("refused launch left resolved")
	}
	if strings.Contains(strings.Join(append(m.LaunchEnv, m.WorkerEnv...), ","), "host-gh_token") {
		t.Fatal("refusal carried a value")
	}
	standard := Meta{Order: Order{Host: "here"}}
	if err := resolveLaunch(&standard, bad); err != nil {
		t.Fatalf("standard run refused by [climb]: %v", err)
	}
}

func TestClimbConfigParses(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("cap = 2\n[climb]\nworker_env = [\"HEVLAYER_READ_API_KEY\"]\n[hosts.here]\nlocal = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, load := range []func() (Config, error){func() (Config, error) { return Load(cfg) }, func() (Config, error) { return loadTarget(cfg, "here") }} {
		c, err := load()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(c.Climb.WorkerEnv, []string{"HEVLAYER_READ_API_KEY"}) {
			t.Fatalf("worker_env: %v", c.Climb.WorkerEnv)
		}
	}
}

func TestWithoutRefusedWorkerEnv(t *testing.T) {
	env := []string{"PATH=/bin", "GH_TOKEN=x", "gh_token=x", "HEVLAYER_API_KEY=x", "HEVLAYER_READ_API_KEY=r", "LAYER_NAMESPACE=n", "SSH_AUTH_SOCK=s", "HEV_RUN_CLASS=climb", "HOME=/scratch", "ANTHROPIC_API_KEY=a"}
	got := withoutRefusedWorkerEnv(env, []string{"HEVLAYER_READ_API_KEY"})
	want := []string{"PATH=/bin", "HEVLAYER_READ_API_KEY=r", "HEV_RUN_CLASS=climb", "HOME=/scratch", "ANTHROPIC_API_KEY=a"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// realHomeFixture fills a stand-in for the operator's HOME with everything a
// worker must not see and every harness's login.
func realHomeFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for path, body := range map[string]string{
		".config/hev/dispatch.toml":   "cap = 1\n",
		".config/gh/hosts.yml":        "github.com:\n  oauth_token: secret\n",
		".ssh/id_ed25519":             "private",
		".git-credentials":            "https://user:secret@example.com\n",
		".gitconfig":                  "[credential]\n\thelper = store\n",
		".claude/.credentials.json":   "{}",
		".claude.json":                "{}",
		".codex/auth.json":            "{}",
		".gemini/oauth_creds.json":    "{}",
		".config/git/credentials.txt": "secret",
	} {
		p := filepath.Join(home, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func stubGitIdentity(t *testing.T, name, email string) {
	t.Helper()
	old := gitIdentity
	gitIdentity = func() (string, string) { return name, email }
	t.Cleanup(func() { gitIdentity = old })
}

func TestPrepareScratchHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privilege on Windows")
	}
	real := realHomeFixture(t)
	stubGitIdentity(t, `Example "Q" Person`, "example@example.com")
	refused := []string{".config", ".ssh", ".git-credentials"}
	for _, tc := range []struct {
		harness string
		env     []string
		links   []string
	}{
		{harness: "claude", links: []string{".claude", ".claude.json"}},
		{harness: "claude", env: []string{"CLAUDE_CONFIG_DIR=/elsewhere"}},
		{harness: "codex", links: []string{".codex"}},
		{harness: "codex", env: []string{"CODEX_HOME=/elsewhere"}},
		{harness: "agy", links: []string{".gemini"}},
	} {
		t.Run(tc.harness+strings.Join(tc.env, ","), func(t *testing.T) {
			dir := t.TempDir()
			// A previous attempt's scratch HOME, with a link into the real one.
			stale := filepath.Join(dir, scratchHomeDir)
			if err := os.MkdirAll(filepath.Join(stale, ".ssh"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(real, ".claude"), filepath.Join(stale, ".claude")); err != nil {
				t.Fatal(err)
			}
			home, err := prepareScratchHome(dir, tc.harness, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if home != stale {
				t.Fatalf("scratch home %s", home)
			}
			info, err := os.Stat(home)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatalf("scratch home mode: %v %v", info, err)
			}
			// Removing the old scratch HOME never followed its links.
			if _, err := os.Stat(filepath.Join(real, ".claude", ".credentials.json")); err != nil {
				t.Fatalf("real login removed: %v", err)
			}
			entries, _ := os.ReadDir(home)
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			want := append([]string{".gitconfig"}, tc.links...)
			slices.Sort(want)
			if !slices.Equal(names, want) {
				t.Fatalf("scratch home holds %v, want %v", names, want)
			}
			for _, p := range refused {
				if _, err := os.Lstat(filepath.Join(home, p)); !os.IsNotExist(err) {
					t.Errorf("scratch home has %s", p)
				}
			}
			for _, p := range tc.links {
				target, err := os.Readlink(filepath.Join(home, p))
				if err != nil || target != filepath.Join(real, p) {
					t.Errorf("%s links to %q: %v", p, target, err)
				}
			}
			b, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
			if err != nil {
				t.Fatal(err)
			}
			cfg := string(b)
			if strings.Contains(cfg, "store") || !strings.Contains(cfg, "[credential]\n\thelper =\n") {
				t.Fatalf("git credential helper not cleared: %q", cfg)
			}
			if !strings.Contains(cfg, `name = "Example \"Q\" Person"`) || !strings.Contains(cfg, `email = "example@example.com"`) {
				t.Fatalf("git identity: %q", cfg)
			}
		})
	}
}

// A climb unit's run control is refused whichever way it is recognized;
// reads, and every action from an ordinary caller, pass admission.
func TestAdmitCallerRefusesClimbUnits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	unsetEnv(t, RunClassEnv)
	for _, action := range []string{"start", "say", "rework", "stop", "retry", "amend", "peer-send", "steer", "orchestrator-apply"} {
		if err := admitCaller(Request{Action: action}); err != nil {
			t.Errorf("ordinary %s refused: %v", action, err)
		}
		err := admitCaller(Request{Action: action, CallerClass: ClimbClass})
		if err == nil || !strings.Contains(err.Error(), "refused: "+action+" from inside a climb unit") {
			t.Errorf("carried climb %s: %v", action, err)
		}
	}
	for _, action := range []string{"list", "show", "logs", "validation-reserve"} {
		if err := admitCaller(Request{Action: action, CallerClass: ClimbClass}); err != nil {
			t.Errorf("climb %s refused: %v", action, err)
		}
	}
	t.Setenv(RunClassEnv, ClimbClass)
	q := Request{Action: "start"}
	markCallerClass(&q)
	if q.CallerClass != ClimbClass {
		t.Fatal("marker not carried on the request")
	}
	if err := admitCaller(Request{Action: "stop"}); err == nil {
		t.Fatal("climb marker admitted stop")
	}
	if err := admitCaller(Request{Action: "show"}); err != nil {
		t.Fatalf("climb marker refused show: %v", err)
	}
	t.Setenv(RunClassEnv, "")
	q = Request{Action: "start"}
	markCallerClass(&q)
	if q.CallerClass != "" {
		t.Fatal("ordinary caller marked")
	}
}

// A remote target sees only the request: rpcTransport refuses before any
// transport, so nothing reaches SSH.
func TestRPCTransportRefusesClimbCaller(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv(RunClassEnv, ClimbClass)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, action := range []string{"start", "say", "rework", "stop"} {
		_, err := rpcTransport(t.Context(), Host{SSH: "unreachable.invalid"}, Request{Action: action, Name: "example"})
		if err == nil || !strings.Contains(err.Error(), "refused: "+action) {
			t.Errorf("%s: %v", action, err)
		}
	}
}
