package skillsources

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0755); e != nil {
		t.Fatal(e)
	}
}
func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	b, e := git(repo, args...)
	if e != nil {
		t.Fatal(e)
	}
	return strings.TrimSpace(string(b))
}
func repository(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	runGit(t, r, "init", "-b", "main")
	runGit(t, r, "config", "user.name", "Test")
	runGit(t, r, "config", "user.email", "test@example.invalid")
	return r
}
func commit(t *testing.T, r string) string {
	t.Helper()
	runGit(t, r, "add", ".")
	return runGit(t, r, "-c", "commit.gpgsign=false", "commit", "-m", "skills")
}
func apply(t *testing.T, h string, c Config, sync bool) {
	t.Helper()
	var out bytes.Buffer
	if e := Run(h, c, sync, &out); e != nil {
		t.Fatalf("%v\n%s", e, &out)
	}
}
func content(t *testing.T, p, want string) {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil || string(b) != want {
		t.Fatalf("%s: %q %v, want %q", p, b, e, want)
	}
}
func TestSourcePinningUpdatePrune(t *testing.T) {
	r := repository(t)
	h := t.TempDir()
	skill := "---\nmetadata:\n  harnesses: all\n---\nfirst\n"
	put(t, filepath.Join(r, "skills", "example", "SKILL.md"), skill)
	put(t, filepath.Join(r, "skills", "example", "bin", "hello.cmd"), "echo first\n")
	commit(t, r)
	c := Config{Sources: []Source{{Path: r, Subdir: "skills"}}}
	apply(t, h, c, true)
	apply(t, h, c, false)
	var oldTarget string
	for _, a := range []string{"claude", "codex", "agents"} {
		p := filepath.Join(h, "."+a, "skills", "example")
		content(t, filepath.Join(p, "SKILL.md"), skill)
		if runtime.GOOS != "windows" {
			target, e := os.Readlink(p)
			if e != nil {
				t.Fatal(e)
			}
			oldTarget = target
		}
	}
	content(t, filepath.Join(h, ".local", "bin", "hello.cmd"), "echo first\n")
	put(t, filepath.Join(r, "skills", "example", "SKILL.md"), "uncommitted")
	apply(t, h, c, false)
	content(t, filepath.Join(h, ".claude", "skills", "example", "SKILL.md"), skill)
	updated := strings.ReplaceAll(skill, "first", "second")
	put(t, filepath.Join(r, "skills", "example", "SKILL.md"), updated)
	commit(t, r)
	if e := Run(h, c, false, &bytes.Buffer{}); e == nil {
		t.Fatal("missed drift")
	}
	apply(t, h, c, true)
	content(t, filepath.Join(h, ".codex", "skills", "example", "SKILL.md"), updated)
	if oldTarget != "" {
		content(t, filepath.Join(oldTarget, "SKILL.md"), skill)
	}
	put(t, filepath.Join(r, "skills", "keep", "SKILL.md"), "keep")
	if e := os.RemoveAll(filepath.Join(r, "skills", "example")); e != nil {
		t.Fatal(e)
	}
	commit(t, r)
	apply(t, h, c, true)
	for _, p := range []string{filepath.Join(h, ".claude", "skills", "example"), filepath.Join(h, ".codex", "skills", "example"), filepath.Join(h, ".agents", "skills", "example"), filepath.Join(h, ".local", "bin", "hello.cmd")} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			t.Fatalf("retired link remains: %s %v", p, e)
		}
	}
	// Removing a configured source also prunes only its owned installs.
	apply(t, h, Config{}, true)
	if _, e := os.Lstat(filepath.Join(h, ".claude", "skills", "keep")); !os.IsNotExist(e) {
		t.Fatal("source removal not pruned")
	}
}
func TestMetadataTargetingAndTwoHalves(t *testing.T) {
	r := t.TempDir()
	put(t, filepath.Join(r, "split", "SKILL.md"), "---\nmetadata:\n  harnesses: [claude, codex]\n  roles: sender\n---\nsender")
	put(t, filepath.Join(r, "split", "bin", "send"), "send")
	put(t, filepath.Join(r, "split", "boh", "SKILL.md"), "---\nmetadata:\n  harnesses:\n    - agents\n  machines: [worker]\n  roles: host\n---\nhost")
	put(t, filepath.Join(r, "split", "boh", "bin", "work"), "work")
	h := t.TempDir()
	c := Config{Role: "sender", Machine: "desktop", Sources: []Source{{Path: r}}}
	apply(t, h, c, true)
	content(t, filepath.Join(h, ".claude", "skills", "split", "SKILL.md"), "---\nmetadata:\n  harnesses: [claude, codex]\n  roles: sender\n---\nsender")
	c.Role = "host"
	c.Machine = "worker"
	apply(t, h, c, true)
	content(t, filepath.Join(h, ".agents", "skills", "split", "SKILL.md"), "---\nmetadata:\n  harnesses:\n    - agents\n  machines: [worker]\n  roles: host\n---\nhost")
	content(t, filepath.Join(h, ".local", "bin", "work"), "work")
	if _, e := os.Lstat(filepath.Join(h, ".local", "bin", "send")); !os.IsNotExist(e) {
		t.Fatal("sender bin survived role change")
	}
	c.Machine = "other"
	apply(t, h, c, true)
	if _, e := os.Lstat(filepath.Join(h, ".agents", "skills", "split")); !os.IsNotExist(e) {
		t.Fatal("machine target not pruned")
	}
}
func TestForeignFilesAndLinksUntouched(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "ancestor"} {
		t.Run(kind, func(t *testing.T) {
			r := t.TempDir()
			h := t.TempDir()
			put(t, filepath.Join(r, "example", "SKILL.md"), "new")
			p := filepath.Join(h, ".claude", "skills", "example")
			switch kind {
			case "file":
				put(t, p, "foreign")
			case "directory":
				put(t, filepath.Join(p, "SKILL.md"), "foreign")
			case "symlink", "ancestor":
				if runtime.GOOS == "windows" {
					t.Skip("symlink privilege independent copy path tested separately")
				}
				target := t.TempDir()
				put(t, filepath.Join(target, "SKILL.md"), "foreign")
				if kind == "ancestor" {
					p = filepath.Dir(p)
				}
				if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(target, p); e != nil {
					t.Fatal(e)
				}
			}
			before, e := os.Lstat(p)
			if e != nil {
				t.Fatal(e)
			}
			if e = Run(h, Config{Sources: []Source{{Path: r}}}, true, &bytes.Buffer{}); e == nil {
				t.Fatal("foreign path accepted")
			}
			after, e := os.Lstat(p)
			if e != nil || !os.SameFile(before, after) {
				t.Fatal("foreign path changed")
			}
			if _, e = os.Lstat(filepath.Join(h, ".codex", "skills", "example")); !os.IsNotExist(e) {
				t.Fatal("partial install")
			}
		})
	}
}
func TestForeignReplacementBlocksPrune(t *testing.T) {
	r := t.TempDir()
	h := t.TempDir()
	put(t, filepath.Join(r, "example", "SKILL.md"), "ours")
	c := Config{Sources: []Source{{Path: r}}}
	apply(t, h, c, true)
	p := filepath.Join(h, ".claude", "skills", "example")
	if e := os.RemoveAll(p); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p, "SKILL.md"), "foreign")
	if e := Run(h, Config{}, true, &bytes.Buffer{}); e == nil {
		t.Fatal("pruned replacement")
	}
	content(t, filepath.Join(p, "SKILL.md"), "foreign")
}
func TestSourceFastForwardOnly(t *testing.T) {
	r := repository(t)
	h := t.TempDir()
	put(t, filepath.Join(r, "example", "SKILL.md"), "one")
	commit(t, r)
	first := runGit(t, r, "rev-parse", "HEAD")
	put(t, filepath.Join(r, "example", "SKILL.md"), "two")
	commit(t, r)
	c := Config{Sources: []Source{{URL: r, Ref: "refs/heads/main"}}}
	apply(t, h, c, true)
	runGit(t, r, "reset", "--hard", first)
	if e := Run(h, c, true, &bytes.Buffer{}); e == nil || !strings.Contains(e.Error(), "non-fast-forward") {
		t.Fatalf("rewrite accepted: %v", e)
	}
	content(t, filepath.Join(h, ".claude", "skills", "example", "SKILL.md"), "two")
}
func TestWindowsCopiesAndCommands(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows path")
	}
	r := t.TempDir()
	h := t.TempDir()
	put(t, filepath.Join(r, "example", "SKILL.md"), "---\nharnesses: all\n---\nwindows")
	put(t, filepath.Join(r, "example", "bin", "greet.cmd"), "@echo hello\r\n")
	c := Config{Sources: []Source{{Path: r}}}
	apply(t, h, c, true)
	apply(t, h, c, false)
	p := filepath.Join(h, ".agents", "skills", "example")
	info, e := os.Lstat(p)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected privilege-free directory copy")
	}
	cmd := exec.Command("cmd", "/c", filepath.Join(h, ".local", "bin", "greet.cmd"))
	if b, e := cmd.Output(); e != nil || strings.TrimSpace(string(b)) != "hello" {
		t.Fatalf("command copy: %q %v", b, e)
	}
}
func TestPublishRollback(t *testing.T) {
	h := t.TempDir()
	stage := t.TempDir()
	target := filepath.Join(t.TempDir(), "skill")
	put(t, target, "new")
	p := filepath.Join(h, "skill")
	put(t, p, "old")
	d, e := digest(target)
	if e != nil {
		t.Fatal(e)
	}
	e = publish(stage, map[string]entry{p: {}}, map[string]entry{p: {Target: target, Digest: d}}, func() error { return os.ErrPermission })
	if e == nil {
		t.Fatal("expected failure")
	}
	content(t, p, "old")
}
func TestConfigRelativePath(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "config.toml")
	put(t, p, "[skills]\nrole = 'host'\nmachine = 'worker'\n[[skills.sources]]\npath = 'repo'\nsubdir = 'skills'\n")
	c, e := Load(p)
	if e != nil || c.Role != "host" || c.Sources[0].Path != filepath.Join(d, "repo") {
		t.Fatalf("%+v %v", c, e)
	}
}

func TestUnsafeArchiveAndSubdir(t *testing.T) {
	for _, s := range []string{"../outside", "a/../../outside", "/absolute", `C:\outside`, "C:outside"} {
		if cleanSub(s) {
			t.Errorf("accepted %q", s)
		}
	}
	r := t.TempDir()
	h := t.TempDir()
	put(t, filepath.Join(r, "example", "SKILL.md"), "---\nharnesses: invalid\n---\n")
	if e := Run(h, Config{Sources: []Source{{Path: r}}}, true, nil); e == nil {
		t.Fatal("unknown harness accepted")
	}
}
func TestSourceCollisionBeforePublication(t *testing.T) {
	h := t.TempDir()
	r1 := t.TempDir()
	r2 := t.TempDir()
	for _, r := range []string{r1, r2} {
		put(t, filepath.Join(r, "example", "SKILL.md"), "example")
	}
	if e := Run(h, Config{Sources: []Source{{Path: r1}, {Path: r2}}}, true, nil); e == nil {
		t.Fatal("collision accepted")
	}
	if _, e := os.Lstat(filepath.Join(h, ".claude", "skills", "example")); !os.IsNotExist(e) {
		t.Fatal("partial publication")
	}
}
func TestBundledNameReserved(t *testing.T) {
	r := t.TempDir()
	put(t, filepath.Join(r, "hev-traces", "SKILL.md"), "replacement")
	if e := Run(t.TempDir(), Config{Sources: []Source{{Path: r}}}, true, nil); e == nil {
		t.Fatal("bundled skill collision accepted")
	}
}
func TestNoopDoesNotReplaceInstall(t *testing.T) {
	r := t.TempDir()
	h := t.TempDir()
	put(t, filepath.Join(r, "example", "SKILL.md"), "example")
	c := Config{Sources: []Source{{Path: r}}}
	apply(t, h, c, true)
	p := filepath.Join(h, ".claude", "skills", "example")
	before, e := os.Lstat(p)
	if e != nil {
		t.Fatal(e)
	}
	apply(t, h, c, true)
	after, e := os.Lstat(p)
	if e != nil || !os.SameFile(before, after) {
		t.Fatal("unchanged install replaced")
	}
}

func TestRemoveLastSkillAndSubdir(t *testing.T) {
	r := repository(t)
	h := t.TempDir()
	put(t, filepath.Join(r, "skills", "example", "SKILL.md"), "example")
	commit(t, r)
	c := Config{Sources: []Source{{Path: r, Subdir: "skills"}}}
	apply(t, h, c, true)
	runGit(t, r, "rm", "-r", "skills")
	commit(t, r)
	apply(t, h, c, true)
	apply(t, h, c, false)
	if _, e := os.Lstat(filepath.Join(h, ".claude", "skills", "example")); !os.IsNotExist(e) {
		t.Fatal("last skill not pruned")
	}
}
