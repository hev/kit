package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/dispatch"
)

// onboardSandbox is sandbox() plus fake harness binaries on PATH and no
// dispatch config override, so the flow sees a fresh machine.
func onboardSandbox(t *testing.T, harnesses ...string) (home, bin string) {
	home, log := sandbox(t, "0")
	bin = filepath.Dir(log)
	for _, h := range harnesses {
		if err := os.WriteFile(filepath.Join(bin, h), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"HEV_DISPATCH_CONFIG", "HEV_ORDERS_DIR", "HEV_ARCHIVE_DIR", "HEV_PEERS_STATE", "HEV_BOARD_DIR", "HEV_EVIDENCE_DIR"} {
		t.Setenv(name, "")
	}
	old := upWorkspace
	t.Cleanup(func() { upWorkspace = old })
	return home, bin
}

func onboard(t *testing.T, input string, noPicker bool) string {
	t.Helper()
	var out bytes.Buffer
	if err := runOnboard(context.Background(), onboardOptions{In: strings.NewReader(input), Out: &out, NoPicker: noPicker, NoDashboard: true}); err != nil {
		t.Fatalf("onboard: %v\n%s", err, out.String())
	}
	return out.String()
}

func readOnboard(t *testing.T) onboardState {
	t.Helper()
	s, ok, err := loadOnboardState()
	if err != nil || !ok {
		t.Fatalf("onboard state: %v %v", ok, err)
	}
	return s
}

func TestOnboardScriptedFirstRunAndIdempotentRerun(t *testing.T) {
	home, _ := onboardSandbox(t, "claude", "codex")
	if !unconfigured() {
		t.Fatal("fresh HOME reads as configured")
	}

	// Harness selection rejects an unknown name and asks again; the
	// workspace prompt takes a ~/ path; the confirmation defaults to yes.
	out := onboard(t, "bogus\nclaude\n~/ws\n\n", true)
	for _, want := range []string{"[✓] claude", "[✓] codex", `unknown harness "bogus"`, "Next steps:", "hev s", "hev dispatch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	s := readOnboard(t)
	if strings.Join(s.Harnesses, ",") != "claude" || s.Workspace != "~/ws" || s.CompletedAt == "" {
		t.Fatalf("state %+v", s)
	}
	if info, err := os.Stat(onboardStatePath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("onboard.toml mode: %v %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(home, "ws")); err != nil || !info.IsDir() {
		t.Fatal("workspace not created")
	}
	// The unified layout, and none of the legacy directories.
	for _, s := range []string{"traces", "board", "orders", "peers", "hooks", "evidence", "reviews"} {
		if info, err := os.Stat(filepath.Join(home, ".hev", s)); err != nil || !info.IsDir() {
			t.Fatalf("~/.hev/%s missing", s)
		}
	}
	for _, legacy := range []string{"dispatch", ".hev/archive", ".local/state/peers-v2"} {
		if _, err := os.Stat(filepath.Join(home, legacy)); !os.IsNotExist(err) {
			t.Fatalf("legacy %s created on a fresh install", legacy)
		}
	}
	// Local dispatch works with no hand-edited TOML.
	cfgPath := dispatch.ConfigPath("")
	info, err := os.Stat(cfgPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("dispatch config: %v %v", info, err)
	}
	cfg, err := dispatch.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if h := cfg.Hosts["here"]; !h.Local || h.Workspace != "~/ws" || cfg.DefaultHost != "here" {
		t.Fatalf("dispatch config %+v", cfg)
	}
	if unconfigured() {
		t.Fatal("onboarded machine reads as unconfigured")
	}

	// Re-run: the operator edited both configs; Enter everywhere keeps the
	// last answers and rewrites neither config.
	custom := []byte("# mine\ncap = 5\nworkspace = \"~/ws\"\ndefault_host = \"here\"\n\n[hosts.here]\nlocal = true\nworkspace = \"~/ws\"\n")
	if err := os.WriteFile(cfgPath, custom, 0o600); err != nil {
		t.Fatal(err)
	}
	kitCfg := os.Getenv("HEV_CONFIG")
	raw, _ := os.ReadFile(kitCfg)
	raw = append(raw, []byte("\n[operator]\nkeep = \"mine\"\n")...)
	if err := os.WriteFile(kitCfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out = onboard(t, "\n\n\n", true)
	if !strings.Contains(out, "onboarded before") || !strings.Contains(out, "[claude]") || !strings.Contains(out, "[~/ws]") {
		t.Fatalf("re-run did not offer the last answers:\n%s", out)
	}
	if again := readOnboard(t); strings.Join(again.Harnesses, ",") != "claude" || again.Workspace != "~/ws" {
		t.Fatalf("re-run changed answers: %+v", again)
	}
	if got, _ := os.ReadFile(cfgPath); !bytes.Equal(got, custom) {
		t.Fatalf("dispatch config clobbered:\n%s", got)
	}
	var kit map[string]any
	if _, err := toml.DecodeFile(kitCfg, &kit); err != nil {
		t.Fatal(err)
	}
	if c, _ := kit["operator"].(map[string]any); c["keep"] != "mine" {
		t.Fatalf("kit config clobbered: %+v", kit)
	}
}

func TestOnboardKeepsLegacyDirectories(t *testing.T) {
	home, _ := onboardSandbox(t)
	for _, legacy := range []string{"dispatch", ".hev/archive", ".local/state/peers-v2"} {
		if err := os.MkdirAll(filepath.Join(home, legacy), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	out := onboard(t, "none\n\ny\n", true)
	if !strings.Contains(out, "existing legacy directory, kept") {
		t.Fatalf("legacy directories not named:\n%s", out)
	}
	for _, s := range []string{"traces", "orders", "peers"} {
		if _, err := os.Stat(filepath.Join(home, ".hev", s)); !os.IsNotExist(err) {
			t.Fatalf("~/.hev/%s created beside its legacy directory", s)
		}
	}
	if s := readOnboard(t); len(s.Harnesses) != 0 {
		t.Fatalf("none chose %v", s.Harnesses)
	}
}

func TestOnboardDeclineWritesNothing(t *testing.T) {
	home, _ := onboardSandbox(t, "claude")
	out := onboard(t, "\n\nn\n", true)
	if !strings.Contains(out, "Nothing was written") {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(home, ".hev")); !os.IsNotExist(err) {
		t.Fatal("declined onboarding wrote ~/.hev")
	}
	if _, err := os.Stat(dispatch.ConfigPath("")); !os.IsNotExist(err) {
		t.Fatal("declined onboarding wrote a dispatch config")
	}
}

// Closed stdin takes every default rather than hanging.
func TestOnboardEOFTakesDefaults(t *testing.T) {
	onboardSandbox(t, "codex")
	onboard(t, "", true)
	if s := readOnboard(t); strings.Join(s.Harnesses, ",") != "codex" || s.Workspace != "~/workspace" {
		t.Fatalf("%+v", s)
	}
}

func TestOnboardPicker(t *testing.T) {
	home, bin := onboardSandbox(t)
	marker := filepath.Join(home, "picked")
	if err := os.WriteFile(filepath.Join(bin, "hev-pick"), []byte("#!/bin/sh\n: > \""+marker+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// --no-picker: installed, but not opened.
	out := onboard(t, "\n\n\n", true)
	if _, err := os.Stat(marker); !os.IsNotExist(err) || !strings.Contains(out, "Next steps:") {
		t.Fatalf("--no-picker opened the Picker:\n%s", out)
	}
	// Without it, an installed Picker is opened.
	out = onboard(t, "\n\n\n", false)
	if _, err := os.Stat(marker); err != nil || !strings.Contains(out, "Opening the Picker") {
		t.Fatalf("Picker not opened:\n%s", out)
	}
}

// up asks nothing without a terminal or with --non-interactive, even on an
// unconfigured machine; on a terminal it onboards only an unconfigured one.
func TestUpPromptsOnlyOnAnUnconfiguredTerminal(t *testing.T) {
	onboardSandbox(t, "claude")
	old := interactive
	t.Cleanup(func() { interactive = old; upNonInteractive, upNoPicker, upNoDashboard = false, false, false })
	run := func(tty bool, args ...string) string {
		interactive = func(io.Reader, io.Writer) bool { return tty }
		upNonInteractive, upNoPicker, upNoDashboard = false, false, false
		var out bytes.Buffer
		rootCmd.SetIn(strings.NewReader("\n\n\n"))
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs(append([]string{"up", "--no-dashboard", "--no-picker"}, args...))
		defer rootCmd.SetArgs(nil)
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("up %v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	if out := run(false); strings.Contains(out, "hev onboard") {
		t.Fatalf("up prompted without a terminal:\n%s", out)
	}
	if !unconfigured() {
		// The non-TTY run bootstrapped dispatch and wrote the config.
		os.RemoveAll(filepath.Dir(os.Getenv("HEV_CONFIG")))
		os.Remove(dispatch.ConfigPath(""))
	}
	if out := run(true, "--non-interactive"); strings.Contains(out, "hev onboard") {
		t.Fatalf("--non-interactive prompted:\n%s", out)
	}
	if _, err := os.Stat(dispatch.ConfigPath("")); err != nil {
		t.Fatal("non-interactive up did not bootstrap dispatch")
	}
	os.RemoveAll(filepath.Dir(os.Getenv("HEV_CONFIG")))
	os.Remove(dispatch.ConfigPath(""))
	if out := run(true); !strings.Contains(out, "hev onboard") {
		t.Fatalf("first terminal up did not onboard:\n%s", out)
	}
	if out := run(true); strings.Contains(out, "hev onboard") {
		t.Fatalf("configured terminal up onboarded again:\n%s", out)
	}
}

func TestOnboardRefusesWithoutTerminal(t *testing.T) {
	onboardSandbox(t)
	old := interactive
	t.Cleanup(func() { interactive = old })
	interactive = func(io.Reader, io.Writer) bool { return false }
	rootCmd.SetArgs([]string{"onboard", "--no-picker"})
	defer rootCmd.SetArgs(nil)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "needs a terminal") {
		t.Fatalf("onboard without a terminal: %v", err)
	}
}

// Switching to hev layer unloads the local dashboard `hev up` installed; a
// machine with no dashboard unit is left alone.
func TestProSwitchStopsLocalDashboard(t *testing.T) {
	home := t.TempDir()
	oldOS, oldSvc := hostOS, linuxService
	t.Cleanup(func() { hostOS, linuxService = oldOS, oldSvc })
	hostOS = "linux"
	fake := &fakeUserService{}
	linuxService = fake
	var out bytes.Buffer
	if err := stopLocalDashboard(&out, home); err != nil || len(fake.calls) != 0 {
		t.Fatalf("no unit: %v %v", err, fake.calls)
	}
	unit := serveUnitPath(home)
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte(managedUnitMarker+"\n[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stopLocalDashboard(&out, home); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(fake.calls, ";"), "disable --now "+serveUnit) {
		t.Fatalf("calls %v", fake.calls)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatal("dashboard unit left installed")
	}
}
