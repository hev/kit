package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
)

type pickOptions struct{ workspace, harness string }

func newPickCmd() *cobra.Command {
	var opt pickOptions
	var status, emit, refresh, traceDetails, wall, keyed bool
	cmd := &cobra.Command{Use: "pick [--status left SESSION_ID | right]", Short: "Open the workspace session picker", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := workspace.Read(layerenv.ConfigPath())
		if err != nil {
			return err
		}
		opt = pickContext(cfg, opt)
		switch {
		case status:
			snapshot := readPickerSnapshot()
			if len(args) == 1 && args[0] == "right" {
				fmt.Fprintln(cmd.OutOrStdout(), pickerRight(cfg, snapshot))
				queuePickerRefresh()
				return nil
			}
			if len(args) == 2 && args[0] == "left" {
				for _, pane := range snapshot.Panes {
					if pane.ID == args[1] || pane.Name == args[1] {
						fmt.Fprintf(cmd.OutOrStdout(), "#[bg=%s,fg=#171923,bold]  %s  #[default]\n", pickerColor(cfg, pane), pickerStatusText(pane.Name))
						break
					}
				}
				return nil
			}
			return fmt.Errorf("expected --status left SESSION_ID or --status right")
		case len(args) > 0:
			return fmt.Errorf("unexpected arguments %q", args)
		case traceDetails:
			return pickTraceDetails(json.NewEncoder(cmd.OutOrStdout()))
		case emit || refresh:
			if refresh {
				if err := refreshPicker(cfg); err != nil {
					return err
				}
			}
			if emit {
				emitPickerItems(cmd.OutOrStdout(), cfg, opt, readPickerSnapshot(), keyed)
			}
			return nil
		case wall:
			return openPickerWall(cfg, opt.workspace)
		default:
			return interactivePicker(cfg, opt)
		}
	}}
	cmd.Flags().StringVar(&opt.workspace, "workspace", "", "Workspace to display")
	cmd.Flags().StringVar(&opt.harness, "harness", "", "Harness to launch")
	cmd.Flags().BoolVar(&status, "status", false, "Print cached tmux status")
	cmd.Flags().BoolVar(&emit, "emit-items", false, "Print cached picker rows")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Refresh cached picker data")
	cmd.Flags().BoolVar(&traceDetails, "trace-details", false, "Read trace descriptions for cached panes")
	cmd.Flags().BoolVar(&wall, "wall", false, "Open the workspace agent wall")
	_ = cmd.Flags().MarkHidden("trace-details")
	cmd.Flags().BoolVar(&keyed, "keyed", false, "Include internal picker row keys")
	_ = cmd.Flags().MarkHidden("keyed")
	return cmd
}

func pickContext(cfg workspace.Config, opt pickOptions) pickOptions {
	var saved pickOptions
	if data, err := os.ReadFile(pickerCachePath() + ".context"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) == 2 {
			saved = pickOptions{parts[0], parts[1]}
		}
	}
	if opt.workspace == "" {
		opt.workspace = saved.workspace
	}
	if row, ok := cfg.Workspaces[opt.workspace]; !ok || row.Archived {
		opt.workspace = workspace.Default
	}
	if opt.harness == "" {
		opt.harness = saved.harness
	}
	if !slices.Contains(pickerHarnesses, opt.harness) {
		opt.harness = pickerHarnesses[0]
	}
	return opt
}

func rememberPickerContext(opt pickOptions) {
	path := pickerCachePath() + ".context"
	if os.MkdirAll(filepath.Dir(path), 0700) == nil {
		_ = os.WriteFile(path, []byte(opt.workspace+" "+opt.harness+"\n"), 0600)
	}
}

func queuePickerRefresh() {
	if stat, err := os.Stat(pickerCachePath()); err == nil && time.Since(stat.ModTime()) < 20*time.Second {
		return
	}
	if exe, err := os.Executable(); err == nil {
		cmd := exec.Command(exe, "pick", "--refresh")
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
		}
	}
}

func cleanPickerText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func pickerStatusText(value string) string {
	return strings.ReplaceAll(cleanPickerText(value), "#", "##")
}

func belongsToPickerWorkspace(cfg workspace.Config, pane pickerPane) string {
	if pane.Workspace != "" {
		return pane.Workspace
	}
	best, depth := workspace.Default, 0
	for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces)) {
		if name == workspace.Default {
			continue
		}
		if pane.Name == name || strings.HasPrefix(pane.Name, name+"-") || pane.Name == "foh-"+name || strings.HasPrefix(pane.Name, "foh-"+name+"-") {
			if len(name) > depth {
				best, depth = name, len(name)
			}
		}
	}
	if depth > 0 {
		return best
	}
	for name, w := range cfg.Workspaces {
		if name == workspace.Default || w.Dir == "" {
			continue
		}
		dir := filepath.Clean(workspace.Expand(w.Dir))
		if (pane.Dir == dir || strings.HasPrefix(pane.Dir, dir+string(filepath.Separator))) && len(dir) > depth {
			best, depth = name, len(dir)
		}
	}
	return best
}

func pickerColor(cfg workspace.Config, pane pickerPane) string {
	w := cfg.Workspaces[belongsToPickerWorkspace(cfg, pane)]
	if p, ok := w.Projects[pane.Project]; ok {
		return workspace.Color(p.Color)
	}
	return workspace.Color(w.Color)
}

func emitPickerItems(w io.Writer, cfg workspace.Config, opt pickOptions, snapshot pickerSnapshot, keyed bool) {
	row := func(key, label string) {
		if keyed {
			fmt.Fprint(w, key, "\t")
		}
		fmt.Fprintln(w, cleanPickerText(label))
	}
	row("new", "new session")
	if opt.workspace == workspace.Default {
		row("shell", "shell")
		for _, name := range slices.Sorted(maps.Keys(cfg.Pick.Frequents)) {
			if name != "shell" {
				row("frequent:"+name, name)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces[opt.workspace].Projects)) {
			project := cfg.Workspaces[opt.workspace].Projects[name]
			row("place:"+name, strings.TrimSpace(project.Icon+" "+name))
		}
	}
	row("heading", "── clients ──")
	for _, name := range activePickerWorkspaces(cfg)[1:] {
		ws := cfg.Workspaces[name]
		row("client:"+name, strings.TrimSpace(ws.Icon+" "+name)+" · new session")
	}
	row("new-client", "new client")
	row("archive", "archive")
	if len(cfg.Pick.CheckIn) > 0 {
		row("check-in", "check-in")
	}
	row("wall", "agent wall")
	row("resume", "resume")
	row("clear", "clear tmux")
	seen := make(map[string]bool)
	for _, p := range snapshot.Panes {
		if seen[p.ID] || belongsToPickerWorkspace(cfg, p) != opt.workspace {
			continue
		}
		seen[p.ID] = true
		label := p.Name
		for _, part := range []string{p.Harness, p.Title, p.Prompt} {
			if part != "" {
				label += " · " + part
			}
		}
		row("session:"+p.ID, label)
	}
}

func activePickerWorkspaces(cfg workspace.Config) []string {
	active := []string{workspace.Default}
	for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces)) {
		if name != workspace.Default && !cfg.Workspaces[name].Archived && workspace.ValidName(name) {
			active = append(active, name)
		}
	}
	return active
}

func pickerHostBadges(snapshot pickerSnapshot) []string {
	var badges []string
	for _, host := range snapshot.Hosts {
		if host.Self {
			continue
		}
		icon := map[string]string{"darwin": "", "linux": "🐧", "windows": "⊞"}[host.OS]
		badges = append(badges, strings.TrimSpace(icon+" "+host.Name+" "+host.State+" "+host.Activity.Short()))
	}
	return badges
}

func pickerRight(cfg workspace.Config, snapshot pickerSnapshot) string {
	line := fmt.Sprintf("peers %d", snapshot.Peers)
	if !cfg.Pick.HideBOH {
		for _, badge := range pickerHostBadges(snapshot) {
			line += "  │  " + pickerStatusText(badge)
		}
	}
	return line
}

func pickerChoose(input string, args ...string) (string, error) {
	cmd := exec.Command("fzf", args...)
	cmd.Stdin, cmd.Stderr = strings.NewReader(input), os.Stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 130 || exit.ExitCode() == 1) {
		return "", nil
	}
	return strings.TrimRight(string(out), "\n"), err
}

func interactivePicker(cfg workspace.Config, opt pickOptions) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	for {
		snapshot := readPickerSnapshot()
		var items bytes.Buffer
		emitPickerItems(&items, cfg, opt, snapshot, true)
		reload := pickerShellCommand([]string{exe, "pick", "--refresh", "--emit-items", "--workspace", opt.workspace, "--harness", opt.harness, "--keyed"})
		args := []string{"--delimiter=\t", "--with-nth=2..", "--expect=ctrl-a,ctrl-o,ctrl-n,ctrl-x,delete,ctrl-l,ctrl-s,ctrl-w", "--bind=start:reload(" + reload + ")", "--header=" + opt.workspace + " · " + opt.harness + "   ctrl-a harness · ctrl-o workspace · ctrl-n name · ctrl-w wall"}
		if !cfg.Pick.HideBOH {
			if badges := pickerHostBadges(snapshot); len(badges) > 0 {
				args = append(args, "--footer=BOH  "+strings.Join(badges, "  │  "))
			}
		}
		out, err := pickerChoose(items.String(), args...)
		if err != nil || out == "" {
			return err
		}
		pressed, item, _ := strings.Cut(out, "\n")
		key, _, _ := strings.Cut(item, "\t")
		kind, target, _ := strings.Cut(key, ":")
		switch pressed {
		case "ctrl-a":
			opt.harness = pickerHarnesses[(slices.Index(pickerHarnesses, opt.harness)+1)%len(pickerHarnesses)]
			rememberPickerContext(opt)
			continue
		case "ctrl-o":
			active := activePickerWorkspaces(cfg)
			opt.workspace = active[(slices.Index(active, opt.workspace)+1)%len(active)]
			rememberPickerContext(opt)
			continue
		case "ctrl-s":
			if _, err := pickerChoose(strings.Join(pickerHostBadges(readPickerSnapshot()), "\n")); err != nil {
				return err
			}
			continue
		case "ctrl-w":
			return openPickerWall(cfg, opt.workspace)
		case "ctrl-x", "delete", "ctrl-l":
			if pressed == "ctrl-l" || kind == "session" {
				if err := closePickerSessions(cfg, opt.workspace, target, pressed == "ctrl-l"); err != nil {
					return err
				}
			}
			continue
		}
		if kind == "heading" {
			continue
		}
		if kind == "clear" {
			if err := closePickerSessions(cfg, opt.workspace, "", true); err != nil {
				return err
			}
			continue
		}
		if kind == "wall" {
			return openPickerWall(cfg, opt.workspace)
		}
		if kind == "session" {
			return attachPickerSession(target)
		}
		if kind == "check-in" {
			return terminalRun(exec.Command(cfg.Pick.CheckIn[0], cfg.Pick.CheckIn[1:]...))
		}

		launch := fohOptions{workspace: opt.workspace, harness: opt.harness}
		switch kind {
		case "client":
			launch.workspace = target
		case "place":
			launch.project = target
			if !cfg.Pick.ProjectTables {
				return openPickerPlain(workspace.Expand(cfg.Workspaces[opt.workspace].Projects[target].Dir), target, opt.harness)
			}
		case "shell", "frequent":
			dir := "~"
			if kind == "frequent" {
				dir = cfg.Pick.Frequents[target]
			}
			return openPickerPlain(workspace.Expand(dir), kind+target, "")
		case "archive":
			var names []string
			for _, name := range slices.Sorted(maps.Keys(cfg.Workspaces)) {
				if cfg.Workspaces[name].Archived {
					names = append(names, name)
				}
			}
			selected, err := pickerChoose(strings.Join(names, "\n"))
			if err != nil || selected == "" {
				return err
			}
			launch.workspace = selected
		case "new-client":
			name, err := pickerInput("Workspace name")
			if err != nil || name == "" {
				return err
			}
			if !workspace.ValidName(name) {
				return fmt.Errorf("invalid workspace name %q", name)
			}
			dir := filepath.Join(workspace.Expand(cfg.Workspaces[workspace.Default].Dir), "clients", name)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return err
			}
			err = workspace.Update(layerenv.ConfigPath(), func(current map[string]workspace.Workspace) error {
				if _, exists := current[name]; exists {
					return fmt.Errorf("workspace %q already exists", name)
				}
				current[name] = workspace.Workspace{Project: workspace.Project{Dir: dir}}
				return nil
			})
			if err != nil {
				return err
			}
			cfg, err = workspace.Read(layerenv.ConfigPath())
			if err != nil {
				return err
			}
			launch.workspace = name
		case "resume":
			return terminalRun(pickerTmux("attach-session"))
		case "new":
		default:
			continue
		}
		if pressed == "ctrl-n" {
			name, err := pickerInput("Session name")
			if err != nil || name == "" {
				return err
			}
			launch.session = name
		}
		return openFOH(cfg, launch)
	}
}

func pickerInput(prompt string) (string, error) {
	cmd := exec.Command("gum", "input", "--placeholder", prompt)
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 130 {
		return "", nil
	}
	return strings.TrimSpace(string(out)), err
}

func openPickerPlain(dir, name, harness string) error {
	args := []string{"new-session", "-d", "-c", dir, "-P", "-F", "#{session_id}"}
	if workspace.ValidName(name) {
		names, _ := tmuxOutput("list-sessions", "-F", "#{session_name}")
		base := name
		for i := 2; strings.Contains("\n"+names+"\n", "\n"+name+"\n"); i++ {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		args = append(args, "-s", name)
	}
	if harness != "" {
		args = append(args, harness)
	}
	id, err := tmuxOutput(args...)
	if err != nil {
		return err
	}
	return attachPickerSession(id)
}

func closePickerSessions(cfg workspace.Config, context, target string, all bool) error {
	if err := terminalRun(exec.Command("gum", "confirm", "Close selected workspace sessions?")); err != nil {
		return nil
	}
	for _, pane := range livePickerPanes() {
		if belongsToPickerWorkspace(cfg, pane) != context || (!all && pane.ID != target) {
			continue
		}
		if pane.Role == "foh" {
			route := cfg.Pick.CloseRoutes[pane.Project]
			if route == "" {
				route = cfg.Pick.CloseRoutes[context]
			}
			if route != "" {
				screen, err := tmuxOutput("capture-pane", "-p", "-t", pane.Pane, "-S", "-30")
				if err != nil {
					return err
				}
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				msg := "Owner closed " + pane.Name + " from the picker. Continue managing its runs.\n\n" + screen
				if err := terminalRun(exec.Command(executable, "peers", "send", route, msg, "--from", pane.Name)); err != nil {
					return err
				}
			}
		}
		if _, err := tmuxOutput("kill-session", "-t", pane.ID); err != nil {
			return err
		}
		if !all {
			break
		}
	}
	return nil
}
