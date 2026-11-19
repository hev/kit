package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
)

const fohNote = "You are front of house (peers role foh). Before routing, planning or sending work anywhere, load the front-of-house skill, and hev-dispatch for any work that runs on another host."

var pickerHarnesses = []string{"claude", "codex", "pi", "agy", "opencode"}

type fohOptions struct {
	workspace, project, harness, session string
	detach                               bool
}

func newFOHCmd() *cobra.Command {
	opt := fohOptions{}
	cmd := &cobra.Command{Use: "foh", Short: "Open a front-of-house tmux session", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := workspace.Read(layerenv.ConfigPath())
		if err != nil {
			return err
		}
		return openFOH(cfg, opt)
	}}
	cmd.Flags().StringVar(&opt.workspace, "workspace", workspace.Default, "Workspace to open")
	cmd.Flags().StringVar(&opt.project, "project", "", "Project within the workspace")
	cmd.Flags().StringVar(&opt.harness, "harness", "claude", "Harness to launch")
	cmd.Flags().StringVar(&opt.session, "session", "", "Explicit tmux session name")
	cmd.Flags().BoolVar(&opt.detach, "detach", false, "Create the session without attaching")
	return cmd
}

func pickerTmux(args ...string) *exec.Cmd {
	if socket := os.Getenv("HEV_TMUX_SOCKET"); socket != "" {
		args = append([]string{"-L", socket}, args...)
	}
	return exec.Command("tmux", args...)
}

func tmuxOutput(args ...string) (string, error) {
	out, err := pickerTmux(args...).Output()
	if exit, ok := err.(*exec.ExitError); ok {
		err = fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
	}
	return strings.TrimRight(string(out), "\n"), err
}

func terminalRun(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func attachPickerSession(target string) error {
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	return terminalRun(pickerTmux(verb, "-t", target))
}

func openFOH(cfg workspace.Config, opt fohOptions) error {
	row, ok := cfg.Workspaces[opt.workspace]
	if !ok || !workspace.ValidName(opt.workspace) {
		return fmt.Errorf("unknown workspace %q", opt.workspace)
	}
	project, base := row.Project, opt.workspace
	if opt.project != "" {
		p, ok := row.Projects[opt.project]
		if !ok || !workspace.ValidName(opt.project) {
			return fmt.Errorf("unknown project %q", opt.project)
		}
		project = p
		if opt.workspace == workspace.Default {
			base = opt.project
		} else {
			base += "-" + opt.project
		}
	}
	if opt.session != "" && !workspace.ValidName(opt.session) {
		return fmt.Errorf("invalid session name %q", opt.session)
	}
	if opt.session != "" {
		if _, err := tmuxOutput("has-session", "-t", "="+opt.session); err == nil {
			label, err := tmuxOutput("display-message", "-p", "-t", "="+opt.session, "#{@hev-workspace}:#{@hev-project}")
			if err != nil {
				return err
			}
			if label != opt.workspace+":"+opt.project {
				return fmt.Errorf("session %q belongs to another context", opt.session)
			}
			if opt.detach {
				return nil
			}
			return attachPickerSession("=" + opt.session)
		}
	}
	if row.Archived {
		row.Login = ""
	}
	launch, inbox, err := fohLaunch(cfg.Pick, opt.workspace, opt.harness, row.Login)
	if err != nil {
		return err
	}
	traceID := ""
	if opt.harness == "claude" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		id[6] = (id[6] & 0x0f) | 0x40
		id[8] = (id[8] & 0x3f) | 0x80
		traceID = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
		launch = append(launch, "--session-id", traceID)
	}
	dir := workspace.Expand(project.Dir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("workspace directory is unavailable: %s", dir)
	}
	name := opt.session
	names, _ := tmuxOutput("list-sessions", "-F", "#{session_name}")
	const placeholder = "sleep 60"
	for n := 1; ; n++ {
		if opt.session == "" {
			name = "foh-" + base
			if n > 1 {
				name += "-" + strconv.Itoa(n)
			}
			if strings.Contains("\n"+names+"\n", "\n"+name+"\n") {
				continue
			}
		}
		id, err := tmuxOutput("new-session", "-d", "-s", name, "-c", dir, "-P", "-F", "#{session_id}", placeholder)
		if err != nil {
			if opt.session == "" && strings.Contains(err.Error(), "duplicate session") {
				continue
			}
			return err
		}
		options := [][2]string{{"@peers-role", "foh"}, {"@hev-workspace", opt.workspace}, {"@hev-project", opt.project}, {"@foh-theme", base}, {"@hev-harness", opt.harness}}
		if traceID != "" {
			options = append(options, [2]string{"@hev-trace", traceID})
		}
		if inbox {
			options = append(options, [2]string{"@peers-inbox", "on"})
		}
		for _, option := range options {
			if _, err = tmuxOutput("set-option", "-t", id, option[0], option[1]); err != nil {
				break
			}
		}
		if err == nil {
			_, err = tmuxOutput("respawn-pane", "-k", "-t", id, "-c", dir, pickerShellCommand(launch))
		}
		if err != nil {
			_, _ = tmuxOutput("kill-session", "-t", id)
			return err
		}
		if opt.detach {
			return nil
		}
		return attachPickerSession(id)
	}
}

func fohLaunch(cfg workspace.Picker, name, harness, login string) ([]string, bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, false, err
	}
	if _, err := exec.LookPath(harness); err != nil {
		return nil, false, err
	}
	args := []string{executable, "peers", "runtime", harness}
	if login != "" && login != "own" {
		return nil, false, fmt.Errorf("invalid workspace login mode %q", login)
	}
	if login == "own" && harness != "claude" {
		return nil, false, fmt.Errorf("workspace %q requires its own login, which %s does not support in hev foh", name, harness)
	}
	inbox := true
	switch harness {
	case "claude":
		args = append(args, "--append-system-prompt", fohNote, "--dangerously-skip-permissions")
		plugin := pickerAsset(cfg.ClaudePlugin, "mod/.claude-plugin/plugin.json")
		if plugin != "" {
			args = append(args, "--plugin-dir", filepath.Dir(filepath.Dir(plugin)))
		}
		if login == "own" {
			profile, err := claudeWorkspaceProfile(name)
			if err != nil {
				return nil, false, err
			}
			args = append([]string{"env", "-u", "CLAUDE_CODE_OAUTH_TOKEN", "-u", "ANTHROPIC_API_KEY", "-u", "ANTHROPIC_AUTH_TOKEN", "-u", "CLAUDE_SECURESTORAGE_CONFIG_DIR", "-u", "OP_SERVICE_ACCOUNT_TOKEN", "-u", "CLAUDE_CODE_USE_BEDROCK", "-u", "CLAUDE_CODE_USE_VERTEX", "-u", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CONFIG_DIR=" + profile, "HEV_CLIENT=" + name}, args...)
		}
	case "codex":
		encoded, _ := json.Marshal(fohNote)
		args = append(args, "-c", "developer_instructions="+string(encoded))
	case "pi":
		extension := pickerAsset(cfg.PiExtension, "pi/extension.ts")
		if extension == "" {
			return nil, false, fmt.Errorf("pi inbox extension missing; configure pick.pi_extension")
		}
		args = append(args, "--extension", extension, "--append-system-prompt", fohNote)
	case "agy", "opencode":
		args, inbox = []string{harness}, false
	default:
		return nil, false, fmt.Errorf("unknown harness %q", harness)
	}
	return args, inbox, nil
}

func pickerAsset(configured, relative string) string {
	if configured != "" {
		return workspace.Expand(configured)
	}
	for _, base := range []string{"~/.agents/skills/peers", "~/.claude/skills/peers"} {
		file := filepath.Join(workspace.Expand(base), relative)
		if _, err := os.Stat(file); err == nil {
			return file
		}
	}
	return ""
}

func pickerShellCommand(args []string) string {
	// tmux executes the single start command through its configured shell.
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func claudeWorkspaceProfile(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	profile := filepath.Join(xdgPath("XDG_DATA_HOME", "~/.local/share"), "client-agent", name, "claude")
	if err := os.MkdirAll(profile, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(profile, 0700); err != nil {
		return "", err
	}
	for _, file := range []string{"settings.json", "settings.local.json", "CLAUDE.md", "commands", "skills", "agents", "hooks", "plugins", "projects"} {
		source := filepath.Join(home, ".claude", file)
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", err
		}
		err := os.Symlink(source, filepath.Join(profile, file))
		if err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	settings := filepath.Join(profile, ".claude.json")
	if _, err := os.Lstat(settings); err == nil {
		return profile, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var main map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &main); err != nil {
			return "", err
		}
	}
	initial := make(map[string]json.RawMessage)
	for _, key := range []string{"hasCompletedOnboarding", "lastOnboardingVersion", "theme", "bypassPermissionsModeAccepted", "autoUpdates", "installMethod", "mcpServers"} {
		if value := main[key]; value != nil {
			initial[key] = value
		}
	}
	if raw := main["projects"]; raw != nil {
		var projects map[string]struct {
			Trusted *bool `json:"hasTrustDialogAccepted,omitempty"`
		}
		if err := json.Unmarshal(raw, &projects); err != nil {
			return "", err
		}
		initial["projects"], err = json.Marshal(projects)
		if err != nil {
			return "", err
		}
	}
	file, err := os.CreateTemp(profile, ".claude-profile-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	encodeErr := json.NewEncoder(file).Encode(initial)
	closeErr := file.Close()
	if encodeErr != nil {
		return "", encodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Link(file.Name(), settings); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	return profile, nil
}
