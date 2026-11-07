package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hev/kit/internal/nodes"
	"github.com/hev/kit/skills"
)

// setupHome isolates every path setup reads or writes.
func setupHome(t *testing.T, harnesses ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	for _, h := range harnesses {
		if err := os.MkdirAll(filepath.Join(home, h), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// joinNode records what `hev nodes join` leaves behind: a joined record and a
// dispatch host whose name and SSH alias are the node alias.
func joinNode(t *testing.T, home, alias string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "hev", "nodes")
	os.MkdirAll(dir, 0o700)
	b, _ := json.Marshal(nodes.Joined{Name: alias, Address: "build.example.test"})
	if err := os.WriteFile(filepath.Join(dir, "joined-"+alias+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	appendDispatchHost(t, home, alias, `ssh = "`+alias+`"`)
}

func appendDispatchHost(t *testing.T, home, name, body string) {
	t.Helper()
	path := filepath.Join(home, ".config", "hev", "dispatch.toml")
	os.MkdirAll(filepath.Dir(path), 0o700)
	old, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(old, []byte("\n[hosts."+name+"]\n"+body+"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func installedSkills(t *testing.T, home, harness string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(home, harness, "skills"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func bundledFor(t *testing.T, audience skillAudience) []string {
	t.Helper()
	entries, _ := fs.ReadDir(skills.FS, ".")
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, _ := skills.FS.ReadFile(e.Name() + "/SKILL.md")
		ok, err := skillTargeted(e.Name(), b, audience)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			names = append(names, e.Name())
		}
	}
	return names
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, _ := d.Info()
			b, _ := os.ReadFile(path)
			files[path] = info.ModTime().String() + "\x00" + string(b)
		}
		return nil
	})
	return files
}

func TestSkillsSetupIdempotent(t *testing.T) {
	home := setupHome(t, ".claude", ".codex")
	var first bytes.Buffer
	if err := runSkillsSetup(home, "", false, &first); err != nil {
		t.Fatalf("%v\n%s", err, first.String())
	}
	if !strings.Contains(first.String(), "role      sender (written to") || !strings.Contains(first.String(), "installed "+filepath.Join(home, ".claude", "skills", "hev-traces")) {
		t.Fatal(first.String())
	}
	before := snapshot(t, home)
	var second bytes.Buffer
	if err := runSkillsSetup(home, "", false, &second); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, home); !reflect.DeepEqual(before, after) {
		t.Fatalf("second setup changed files\nbefore %v\nafter %v", before, after)
	}
	if !strings.Contains(second.String(), "role      sender (unchanged)") || strings.Contains(second.String(), "installed") || strings.Contains(second.String(), "repaired") {
		t.Fatal(second.String())
	}
	want := bundledFor(t, ownerAudience)
	for _, h := range []string{".claude", ".codex"} {
		if got := installedSkills(t, home, h); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s skills %v, want %v", h, got, want)
		}
	}
}

// Setup never touches the service manager, Docker or capture state.
func TestSkillsSetupStartsNoDaemon(t *testing.T) {
	home, log := sandbox(t, "0")
	bin := filepath.Dir(log)
	for _, name := range []string{"systemctl", "loginctl"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho \""+name+" $*\" >> \""+log+"\"\n"), 0o755)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	var out bytes.Buffer
	if err := runSkillsSetup(home, "", false, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got := allCalls(t, log); got != "" {
		t.Fatalf("setup ran external commands:\n%s", got)
	}
	for _, p := range []string{".hev/daemon.lock", ".hev/index-state.json", "Library/LaunchAgents", ".config/systemd"} {
		if _, err := os.Stat(filepath.Join(home, p)); !os.IsNotExist(err) {
			t.Errorf("setup created %s", p)
		}
	}
}

func TestSkillsSetupRolePreservesConfig(t *testing.T) {
	home := setupHome(t, ".claude")
	path := os.Getenv("HEV_CONFIG")
	os.MkdirAll(filepath.Dir(path), 0o755)
	original := "# my settings\n[layer]\nendpoint = 'http://localhost:8787' # keep quoting\n\n[skills]\nmachine = \"laptop\" # label\n\n[capture]\nredact = true\n"
	os.WriteFile(path, []byte(original), 0o600)
	if err := runSkillsSetup(home, "", false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := strings.Replace(original, "[skills]\n", "[skills]\nrole = \"sender\"\n", 1)
	if string(b) != want {
		t.Fatalf("config:\n%s\nwant:\n%s", b, want)
	}
	// An explicit role replaces only the value; omitting it keeps a host a host.
	if err := runSkillsSetup(home, "host", false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := runSkillsSetup(home, "", false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if string(b) != strings.Replace(want, `role = "sender"`, `role = "host"`, 1) {
		t.Fatalf("config:\n%s", b)
	}
	if err := runSkillsSetup(home, "owner", false, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted an unknown role")
	}
}

func TestWriteSkillsRoleShapes(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct{ in, want string }{
		"missing file":  {"", "[skills]\nrole = \"sender\"\n"},
		"no table":      {"# c\n[layer]\nnamespace = \"n\"\n", "skills.role = \"sender\"\n# c\n[layer]\nnamespace = \"n\"\n"},
		"sources only":  {"[[skills.sources]]\npath = \"team\"\n", "skills.role = \"sender\"\n[[skills.sources]]\npath = \"team\"\n"},
		"inline table":  {"skills = { machine = \"m\" }\n", "skills = {role = \"sender\",  machine = \"m\" }\n"},
		"crlf":          {"[skills]\r\nmachine = \"m\"\r\n", "[skills]\r\nrole = \"sender\"\r\nmachine = \"m\"\r\n"},
		"replace value": {"[skills]\nrole = 'host' # was host\n", "[skills]\nrole = \"sender\" # was host\n"},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".toml")
		if tc.in != "" {
			os.WriteFile(path, []byte(tc.in), 0o600)
		}
		if changed, err := writeSkillsRole(path, "sender"); err != nil || !changed {
			t.Fatalf("%s: changed %v err %v", name, changed, err)
		}
		if b, _ := os.ReadFile(path); string(b) != tc.want {
			t.Errorf("%s:\n%q\nwant\n%q", name, b, tc.want)
		}
		if changed, err := writeSkillsRole(path, "sender"); err != nil || changed {
			t.Errorf("%s: rewrite changed %v err %v", name, changed, err)
		}
	}
}

func TestDetectSkillAudience(t *testing.T) {
	joined := []nodes.Joined{{Name: "hev-node-aaaa"}, {Name: "hev-node-bbbb"}}
	for name, tc := range map[string]struct {
		config string
		want   skillAudience
	}{
		"no config":   {"", skillAudience{Owner: true}},
		"no hosts":    {"cap = 2\n", skillAudience{Owner: true}},
		"none joined": {"[hosts.build]\nssh = \"build\"\n[hosts.mini]\nlocal = true\n", skillAudience{Owner: true}},
		"all joined":  {"[hosts.hev-node-aaaa]\nssh = \"hev-node-aaaa\"\n[hosts.shared]\nssh = \"hev-node-bbbb\"\n", skillAudience{Member: true}},
		"mixed":       {"[hosts.hev-node-aaaa]\nssh = \"hev-node-aaaa\"\n[hosts.build]\nssh = \"build\"\n", skillAudience{Owner: true, Member: true}},
		"local alias": {"[hosts.hev-node-aaaa]\nlocal = true\n", skillAudience{Owner: true}},
	} {
		path := filepath.Join(t.TempDir(), "dispatch.toml")
		if tc.config != "" {
			os.WriteFile(path, []byte(tc.config), 0o600)
		}
		got, err := detectSkillAudience(path, joined)
		if err != nil || got != tc.want {
			t.Errorf("%s: %+v %v, want %+v", name, got, err, tc.want)
		}
	}
	path := filepath.Join(t.TempDir(), "dispatch.toml")
	os.WriteFile(path, []byte("[hosts.hev-node-aaaa]\nssh = \"hev-node-aaaa\"\n"), 0o600)
	if got, _ := detectSkillAudience(path, nil); got != ownerAudience {
		t.Errorf("unjoined hev-node-* name treated as member: %+v", got)
	}
	os.WriteFile(path, []byte("[hosts\n"), 0o600)
	if _, err := detectSkillAudience(path, joined); err == nil {
		t.Error("invalid dispatch config accepted")
	}
}

func TestMemberSkillsReplaceOwnerSkills(t *testing.T) {
	home := setupHome(t, ".claude")
	// An owner-era install, then a local edit to one owner-only skill.
	if err := runSkillsSetup(home, "", false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(home, ".claude", "skills", "hev-board", "SKILL.md")
	os.WriteFile(edited, []byte("my board notes"), 0o644)
	joinNode(t, home, "hev-node-aaaa")

	var check bytes.Buffer
	states, err := syncSkills(home, machineSkillAudience(home, &check), false, false, &check)
	if err != nil || !strings.Contains(check.String(), "claude/hev-expedite: not for member") || !strings.Contains(check.String(), "claude/hev-member: missing") {
		t.Fatalf("%v\n%s", err, check.String())
	}
	if len(states) == 0 {
		t.Fatal("no states")
	}
	var out bytes.Buffer
	if err := runSkillsSetup(home, "", false, &out); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".claude", "skills")
	for _, want := range []string{"audience  member", "removed   " + filepath.Join(dest, "hev-expedite"), "removed   " + filepath.Join(dest, "hev-dispatch"), "installed " + filepath.Join(dest, "hev-member"), "left      " + filepath.Join(dest, "hev-board") + " (locally edited, not for member)", "kept      1 locally edited"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q:\n%s", want, out.String())
		}
	}
	if got, want := installedSkills(t, home, ".claude"), []string{"hev-board", "hev-member", "hev-traces"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("installed %v, want %v", got, want)
	}
	if b, _ := os.ReadFile(edited); string(b) != "my board notes" {
		t.Fatal("lost local edits")
	}
	if err := runSkillsSetup(home, "", true, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got, want := installedSkills(t, home, ".claude"), bundledFor(t, skillAudience{Member: true}); !reflect.DeepEqual(got, want) || reflect.DeepEqual(want, bundledFor(t, ownerAudience)) {
		t.Fatalf("forced member install %v, want %v", got, want)
	}
	for _, name := range bundledFor(t, skillAudience{Member: true}) {
		if name == "hev-expedite" || name == "hev-dispatch" {
			t.Fatalf("owner-only %s targets members", name)
		}
	}

	// A second, owned host makes the machine mixed: owner skills return beside hev-member.
	appendDispatchHost(t, home, "build", `ssh = "build"`)
	if err := runSkillsSetup(home, "", false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got, want := installedSkills(t, home, ".claude"), bundledFor(t, skillAudience{Owner: true, Member: true}); !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed install %v, want %v", got, want)
	}
}

// `hev up`, `hev join` and setup share installBundledSkills. On a machine with
// no joined nodes it installs exactly the owner set `hev up` always installed.
func TestUpSharesSkillsSetupPath(t *testing.T) {
	home := setupHome(t, ".claude")
	got, err := installSkills(home)
	if err != nil || !reflect.DeepEqual(got, []string{"claude"}) {
		t.Fatalf("%v %v", got, err)
	}
	owner := installedSkills(t, home, ".claude")
	var preexisting []string
	entries, _ := fs.ReadDir(skills.FS, ".")
	for _, e := range entries {
		if e.IsDir() && e.Name() != "hev-member" {
			preexisting = append(preexisting, e.Name())
		}
	}
	if !reflect.DeepEqual(owner, preexisting) {
		t.Fatalf("hev up installed %v, want the unchanged owner set %v", owner, preexisting)
	}
	if _, err := os.Stat(os.Getenv("HEV_CONFIG")); !os.IsNotExist(err) {
		t.Fatal("hev up's skill step wrote the config")
	}
	joinNode(t, home, "hev-node-aaaa")
	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	if got, want := installedSkills(t, home, ".claude"), bundledFor(t, skillAudience{Member: true}); !reflect.DeepEqual(got, want) {
		t.Fatalf("hev up on a member machine installed %v, want %v", got, want)
	}
}

func TestSkillsSetupReportsMissingHarness(t *testing.T) {
	home := setupHome(t)
	var out bytes.Buffer
	err := runSkillsSetup(home, "", false, &out)
	if err == nil || !strings.Contains(out.String(), "missing   claude: no "+filepath.Join(home, ".claude")) || !strings.Contains(out.String(), "missing   codex") {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatal("setup created a harness home")
	}
}

// pi and OpenCode share one ~/.agents/skills copy, written only when either is present.
func TestSkillsSetupSharedAgentsCopy(t *testing.T) {
	for _, h := range []string{".pi", filepath.Join(".config", "opencode")} {
		home := setupHome(t, h)
		var out bytes.Buffer
		if err := runSkillsSetup(home, "", false, &out); err != nil {
			t.Fatalf("%s: %v\n%s", h, err, out.String())
		}
		if got, want := installedSkills(t, home, ".agents"), bundledFor(t, ownerAudience); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: agents skills %v, want %v", h, got, want)
		}
		if !strings.Contains(out.String(), "harness   pi/OpenCode: "+filepath.Join(home, h)+" -> "+filepath.Join(home, ".agents", "skills")) {
			t.Fatal(out.String())
		}
		for _, other := range []string{".claude", ".codex"} {
			if _, err := os.Stat(filepath.Join(home, other)); !os.IsNotExist(err) {
				t.Fatalf("created %s", other)
			}
		}
	}
	home := setupHome(t, ".claude")
	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatal("wrote ~/.agents without pi or OpenCode")
	}
}

// Setup syncs configured team sources and prints each published destination.
func TestSkillsSetupSyncsTeamSources(t *testing.T) {
	home := setupHome(t, ".claude")
	team := filepath.Join(home, "team")
	os.MkdirAll(filepath.Join(team, "team-notes"), 0o755)
	os.WriteFile(filepath.Join(team, "team-notes", "SKILL.md"), []byte("---\nname: team-notes\ndescription: d\nmetadata:\n  roles: [sender]\n---\nbody\n"), 0o644)
	path := os.Getenv("HEV_CONFIG")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("[[skills.sources]]\npath = \""+filepath.ToSlash(team)+"\"\n"), 0o600)
	var out bytes.Buffer
	if err := runSkillsSetup(home, "", false, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "source    "+filepath.Join(home, ".claude", "skills", "team-notes")+" -> ") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "team-notes", "SKILL.md")); err != nil {
		t.Fatal("sender-role source skill not installed:", err)
	}
}

// Codex 0.162 lists a same-name skill from ~/.codex/skills and ~/.agents/skills
// twice, so the shared copy replaces owned Codex copies.
func TestSharedCopyServesCodex(t *testing.T) {
	home := setupHome(t, ".codex")
	if _, err := installSkills(home); err != nil {
		t.Fatal(err)
	}
	if got := installedSkills(t, home, ".codex"); !reflect.DeepEqual(got, bundledFor(t, ownerAudience)) {
		t.Fatalf("codex-only machine: %v", got)
	}
	os.MkdirAll(filepath.Join(home, ".pi"), 0o755)
	var out bytes.Buffer
	if err := runSkillsSetup(home, "", false, &out); err != nil {
		t.Fatal(err)
	}
	if got := installedSkills(t, home, ".codex"); len(got) != 0 {
		t.Fatalf("codex copies kept beside the shared copy: %v\n%s", got, out.String())
	}
	if got := installedSkills(t, home, ".agents"); !reflect.DeepEqual(got, bundledFor(t, ownerAudience)) {
		t.Fatalf("shared copy %v", got)
	}
	if !strings.Contains(out.String(), "removed   "+filepath.Join(home, ".codex", "skills", "hev-traces")) || !strings.Contains(out.String(), "harness   codex: "+filepath.Join(home, ".codex")+" -> "+filepath.Join(home, ".agents", "skills")+" (shared") {
		t.Fatal(out.String())
	}
	var check bytes.Buffer
	states, err := syncSkills(home, ownerAudience, false, false, &check)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.State != "current" {
			t.Fatalf("check after setup: %s", check.String())
		}
	}
}
