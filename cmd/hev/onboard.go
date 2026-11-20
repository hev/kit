package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/hevpaths"
	"github.com/hev/kit/internal/platformops"
	"github.com/spf13/cobra"
)

// upWorkspace is the workspace a bootstrapped dispatch config names. Onboarding
// sets it to the operator's answer; plain `hev up` keeps the default.
var upWorkspace = "~/workspace"

// interactive reports whether both ends are a terminal. It is a variable so
// tests can drive the interactive flow through a scripted reader.
var interactive = func(in io.Reader, out io.Writer) bool {
	return stdinIsTTY(in) && isTerminal(out)
}

// onboardStatePath is where onboarding records its answers. It is its own
// file so a re-run rewrites only what onboarding owns, never kit's config.
func onboardStatePath() string { return filepath.Join(hevpaths.Base(hevpaths.Home()), "onboard.toml") }

// unconfigured is a machine with no kit config, no dispatch config and no
// onboarding record: a fresh install. Any one of them means someone set this
// machine up already, and `hev up` must not start asking questions there.
func unconfigured() bool {
	for _, p := range []string{daemon.DefaultConfigPath(), dispatch.ConfigPath(""), onboardStatePath()} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// ensureSurfaces creates each surface's resolved directory and names where
// each one lives, saying so when a legacy directory is kept.
func ensureSurfaces(out io.Writer) error {
	rs, err := hevpaths.EnsureAll()
	if err != nil {
		return fmt.Errorf("create data directories: %w", err)
	}
	var legacy []string
	for _, r := range rs {
		if r.Source == hevpaths.SourceLegacy {
			legacy = append(legacy, fmt.Sprintf("%s at %s", r.Surface, r.Dir))
		}
	}
	tick(out, "data directories ready under %s", hevpaths.Base(hevpaths.Home()))
	if len(legacy) > 0 {
		fmt.Fprintf(out, "  • kept existing legacy directories: %s\n", strings.Join(legacy, ", "))
	}
	return nil
}

type bootstrapHost struct {
	Local     bool   `toml:"local"`
	Workspace string `toml:"workspace"`
}

type bootstrapConfig struct {
	Cap         int                      `toml:"cap"`
	Workspace   string                   `toml:"workspace"`
	DefaultHost string                   `toml:"default_host"`
	Hosts       map[string]bootstrapHost `toml:"hosts"`
}

// bootstrapDispatch writes a dispatch config naming this machine as the local
// host "here" when there is none, so `hev dispatch` works with no hand-edited
// TOML. An existing config, or an explicit HEV_DISPATCH_CONFIG, is never
// written: they belong to the operator.
func bootstrapDispatch(out io.Writer, workspace string) error {
	if os.Getenv("HEV_DISPATCH_CONFIG") != "" {
		return nil
	}
	path := dispatch.ConfigPath("")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return err
	}
	if workspace == "" {
		workspace = "~/workspace"
	}
	var buf bytes.Buffer
	buf.WriteString("# Written by hev up: dispatch runs on this machine (host \"here\").\n")
	buf.WriteString("# Run records live under " + displayHome(hevpaths.Dir(hevpaths.Orders)) + " (docs/dispatch.md).\n")
	if err := toml.NewEncoder(&buf).Encode(bootstrapConfig{Cap: 2, Workspace: workspace, DefaultHost: "here",
		Hosts: map[string]bootstrapHost{"here": {Local: true, Workspace: workspace}}}); err != nil {
		return err
	}
	if err := platformops.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	tick(out, "dispatch config written: this machine is host \"here\" (%s)", displayHome(path))
	return nil
}

// displayHome shortens a path under HOME to ~/..., for output.
func displayHome(p string) string {
	home := hevpaths.Home()
	if rel, err := filepath.Rel(home, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// onboardState is ~/.hev/onboard.toml.
type onboardState struct {
	Harnesses   []string `toml:"harnesses"`
	Workspace   string   `toml:"workspace"`
	CompletedAt string   `toml:"completed_at"`
}

func loadOnboardState() (onboardState, bool, error) {
	var s onboardState
	_, err := toml.DecodeFile(onboardStatePath(), &s)
	if os.IsNotExist(err) {
		return s, false, nil
	}
	if err != nil {
		return s, false, fmt.Errorf("read %s: %w", onboardStatePath(), err)
	}
	return s, true, nil
}

func writeOnboardState(s onboardState) error {
	var buf bytes.Buffer
	buf.WriteString("# Written by hev onboard. Run `hev onboard` again to change it.\n")
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return err
	}
	path := onboardStatePath()
	if err := platformops.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := platformops.CreateTemp(filepath.Dir(path), ".onboard-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && hostOS != "windows" {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return platformops.Rename(tmp.Name(), path)
}

// detectedHarness is one harness from the RFC 0020 registry and where it was
// seen on this machine ("" when it was not).
type detectedHarness struct {
	Name, Seen string
}

// lookPath is exec.LookPath, replaceable in tests.
var lookPath = exec.LookPath

// detectHarnesses lists every registry harness kit can run or capture, and
// marks the ones on PATH or with a config directory under home.
func detectHarnesses(home string) []detectedHarness {
	var out []detectedHarness
	for _, d := range harness.All() {
		if d.Dispatch == nil && d.Skills == nil {
			continue
		}
		h := detectedHarness{Name: d.Name}
		if p, err := lookPath(d.Binary); err == nil {
			h.Seen = p
		} else if d.Skills != nil && d.Skills.InstallMarker != "" {
			if info, err := os.Stat(filepath.Join(home, d.Skills.InstallMarker)); err == nil && info.IsDir() {
				h.Seen = "~/" + d.Skills.InstallMarker
			}
		}
		out = append(out, h)
	}
	return out
}

type onboardOptions struct {
	In          io.Reader
	Out         io.Writer
	NoPicker    bool
	NoDashboard bool
}

// maxPromptTries bounds re-asking after an answer kit cannot use.
const maxPromptTries = 5

type prompter struct {
	r   *bufio.Reader
	out io.Writer
	eof bool
}

// ask prints the question with its default and returns the trimmed answer,
// or the default for an empty line or end of input (so a closed stdin can
// never hang onboarding).
func (p *prompter) ask(question, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", question)
	}
	if p.eof {
		fmt.Fprintln(p.out)
		return def
	}
	line, err := p.r.ReadString('\n')
	if err != nil {
		p.eof = true
		if line == "" {
			fmt.Fprintln(p.out)
		}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func (p *prompter) confirm(question string, def bool) bool {
	d := "Y/n"
	if !def {
		d = "y/N"
	}
	for range maxPromptTries {
		switch strings.ToLower(p.ask(question, d)) {
		case strings.ToLower(d):
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		fmt.Fprintln(p.out, "  answer y or n")
	}
	return def
}

// parseHarnesses reads "claude, codex", "1 3" or "none" against the list.
func parseHarnesses(answer string, list []detectedHarness) ([]string, error) {
	if strings.EqualFold(strings.TrimSpace(answer), "none") {
		return []string{}, nil
	}
	var out []string
	for _, f := range strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		name := strings.ToLower(f)
		if n, err := strconv.Atoi(f); err == nil {
			if n < 1 || n > len(list) {
				return nil, fmt.Errorf("no harness numbered %d", n)
			}
			name = list[n-1].Name
		} else if d, ok := harness.Lookup(name); ok {
			name = d.Name
		}
		if !slices.ContainsFunc(list, func(h detectedHarness) bool { return h.Name == name }) {
			return nil, fmt.Errorf("unknown harness %q", f)
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// expandWorkspace turns an answer into an absolute directory, keeping a
// leading ~/ form for the config so it follows HOME.
func expandWorkspace(answer string) (configured, abs string, err error) {
	answer = strings.TrimSpace(answer)
	switch {
	case answer == "~" || strings.HasPrefix(answer, "~/"):
		return answer, dispatch.Expand(answer), nil
	case filepath.IsAbs(answer):
		return filepath.Clean(answer), filepath.Clean(answer), nil
	}
	return "", "", fmt.Errorf("give an absolute path or one under ~/")
}

// runOnboard is the first-run flow: harnesses, workspace and ~/.hev, then
// `hev up`, then the Picker. Every answer has a default, so pressing Enter
// throughout sets a machine up; a re-run offers the last answers and never
// rewrites an existing kit or dispatch config.
func runOnboard(ctx context.Context, o onboardOptions) error {
	out := o.Out
	p := &prompter{r: bufio.NewReader(o.In), out: out}
	home := hevpaths.Home()
	prev, rerun, err := loadOnboardState()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "hev onboard: set up kit on this machine")
	if rerun {
		fmt.Fprintf(out, "  (onboarded before; the defaults are your last answers from %s)\n", displayHome(onboardStatePath()))
	}

	// 1. Harnesses.
	list := detectHarnesses(home)
	fmt.Fprintln(out, "\n1. Agent harnesses (from kit's harness registry):")
	var detected []string
	for i, h := range list {
		mark, seen := " ", "not found"
		if h.Seen != "" {
			mark, seen = "✓", h.Seen
			detected = append(detected, h.Name)
		}
		fmt.Fprintf(out, "   %d. [%s] %-9s %s\n", i+1, mark, h.Name, seen)
	}
	def := detected
	if rerun {
		def = prev.Harnesses
	}
	defText := strings.Join(def, ",")
	if defText == "" {
		defText = "none"
	}
	var chosen []string
	for try := 0; ; try++ {
		chosen, err = parseHarnesses(p.ask("   Use which harnesses? (names or numbers, comma-separated)", defText), list)
		if err == nil {
			break
		}
		if try+1 >= maxPromptTries {
			return fmt.Errorf("harness selection: %w", err)
		}
		fmt.Fprintf(out, "   %s; choose from the list above\n", err)
	}

	// 2. Workspace and ~/.hev.
	fmt.Fprintln(out, "\n2. Workspace and data:")
	wsDef := "~/workspace"
	if rerun && prev.Workspace != "" {
		wsDef = prev.Workspace
	} else if cfg, err := dispatch.Load(dispatch.ConfigPath("")); err == nil && cfg.Workspace != "" {
		wsDef = cfg.Workspace
	}
	var wsConfigured, wsAbs string
	for try := 0; ; try++ {
		wsConfigured, wsAbs, err = expandWorkspace(p.ask("   Workspace directory (where your repositories live)", wsDef))
		if err == nil {
			break
		}
		if try+1 >= maxPromptTries {
			return fmt.Errorf("workspace: %w", err)
		}
		fmt.Fprintf(out, "   %s\n", err)
	}
	fmt.Fprintf(out, "   kit keeps its data under %s:\n", displayHome(hevpaths.Base(home)))
	for _, s := range hevpaths.All {
		r := hevpaths.Resolve(s)
		note := ""
		if r.Source == hevpaths.SourceLegacy {
			note = " (existing legacy directory, kept)"
		} else if r.Source == hevpaths.SourceEnv {
			note = " (from " + hevpaths.Env[s] + ")"
		}
		fmt.Fprintf(out, "     %-9s %s%s\n", s, displayHome(r.Dir), note)
	}
	if !p.confirm("   Create the workspace and these directories, write the config and start kit?", true) {
		fmt.Fprintln(out, "Nothing was written. Run `hev onboard` when you are ready.")
		return nil
	}
	if err := os.MkdirAll(wsAbs, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := writeOnboardState(onboardState{Harnesses: chosen, Workspace: wsConfigured, CompletedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return err
	}
	tick(out, "answers saved to %s", displayHome(onboardStatePath()))

	// 3. hev up: config, data directories, skills, hevd and the dashboard.
	fmt.Fprintln(out, "\n3. Starting kit (hev up):")
	upWorkspace = wsConfigured
	if err := runUp(ctx, out, o.NoDashboard); err != nil {
		return err
	}

	// 4. The Picker, or what to do next.
	fmt.Fprintln(out)
	if picker := pickerCommand(); picker != nil && !o.NoPicker {
		fmt.Fprintln(out, "Opening the Picker…")
		picker.Stdin, picker.Stdout, picker.Stderr = o.In, out, out
		return picker.Run()
	}
	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintln(out, "  hev s                       what kit is doing on this machine")
	fmt.Fprintln(out, "  http://127.0.0.1:"+strconv.Itoa(servePortOrDefault())+"       traces and board in your browser")
	fmt.Fprintln(out, "  hev dispatch ORDER.md       send work to an agent on host \"here\"")
	return nil
}

func servePortOrDefault() int {
	if cfg, err := daemon.LoadConfig(); err == nil && cfg.Local.ServePort != 0 {
		return cfg.Local.ServePort
	}
	return 8099
}

// pickerCommand finds the Picker: a `pick` subcommand of this hev, or a
// `hev-pick` plugin on PATH. Nil when neither is installed.
func pickerCommand() *exec.Cmd {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "pick" {
			if exe, err := os.Executable(); err == nil {
				return exec.Command(exe, "pick")
			}
		}
	}
	if p, err := lookPath("hev-pick"); err == nil {
		return exec.Command(p)
	}
	return nil
}

var onboardNoPicker, onboardNoDashboard bool

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Set kit up on this machine, step by step",
	Long: `Walk through first-run setup on a terminal:

  1. choose the agent harnesses to use (detected from kit's harness registry);
  2. confirm the workspace directory (default ~/workspace) and the data
     directories under ~/.hev/;
  3. run ` + "`hev up`" + `: config, data directories, bundled skills, hevd and the
     local dashboard (Traces and Board);
  4. open the Picker when it is installed (` + "`hev pick`" + ` or hev-pick on PATH),
     or print next steps.

Answers are kept in ~/.hev/onboard.toml (mode 0600). Running onboard again
offers them as defaults and never rewrites an existing kit or dispatch
config. ` + "`hev up`" + ` on a terminal runs onboard the first time; without a
terminal it never asks.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !interactive(cmd.InOrStdin(), cmd.OutOrStdout()) {
			return fmt.Errorf("hev onboard needs a terminal; run `hev up --non-interactive` to set this machine up with defaults")
		}
		return runOnboard(cmd.Context(), onboardOptions{In: cmd.InOrStdin(), Out: cmd.OutOrStdout(), NoPicker: onboardNoPicker, NoDashboard: onboardNoDashboard})
	},
}

func init() {
	onboardCmd.SilenceUsage, onboardCmd.SilenceErrors = true, true
	onboardCmd.Flags().BoolVar(&onboardNoPicker, "no-picker", false, "Do not open the Picker at the end (tests, headless machines)")
	onboardCmd.Flags().BoolVar(&onboardNoDashboard, "no-dashboard", false, "Skip the local dashboard service")
	rootCmd.AddCommand(onboardCmd)
}
