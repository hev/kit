package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hev/kit/internal/foh"
	"github.com/hev/kit/internal/version"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var fohCmd = &cobra.Command{
	Use:   "foh",
	Short: "Optional native front-of-house harness (OpenRouter, YOLO by default)",
	Long: `hev foh is kit's optional native front-of-house harness: a lean Go agent
loop and Bubble Tea TUI routed through OpenRouter, with an in-harness model
picker, a composable system prompt and first-class hev tools (dispatch, peers,
board, traces, status). Tools run without approval prompts; isolation belongs
to the host, workspace, admission and gates.

Back-of-house work stays on claude, codex and agy through hev dispatch.

Set OPENROUTER_API_KEY (or HEV_FOH_API_KEY, or [foh].api_key_ref in
~/.hev/config.toml). See docs/foh-harness.md.`,
	Args: cobra.NoArgs,
	RunE: runFOH,
}

func init() {
	f := fohCmd.Flags()
	f.String("model", "", "OpenRouter model slug (default [foh].model, HEV_FOH_MODEL or the first favorite)")
	f.String("prompt", "", "Extra system prompt file appended to the composed prompt")
	f.String("once", "", "Run one non-interactive turn with this message and exit")
	f.Bool("show-mascot", false, "Print the mascot and exit")
	f.Bool("no-color", false, "Force the monochrome ASCII rendering")
	rootCmd.AddCommand(fohCmd)
}

// fohColor reports whether to render color: not --no-color, not NO_COLOR,
// and not TERM=dumb.
func fohColor(cmd *cobra.Command) bool {
	if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
		return false
	}
	// Piped output keeps color so --show-mascot can be captured.
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func terminalWidth(out io.Writer) int {
	if f, ok := out.(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			return w
		}
	}
	return 0
}

func runFOH(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	color := fohColor(cmd)
	if show, _ := cmd.Flags().GetBool("show-mascot"); show {
		fmt.Fprintln(out, foh.RenderMascot(terminalWidth(out), color))
		return nil
	}
	if !color {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	cfg, err := foh.LoadConfig("")
	if err != nil {
		return err
	}
	modelFlag, _ := cmd.Flags().GetString("model")
	promptFlag, _ := cmd.Flags().GetString("prompt")
	once, _ := cmd.Flags().GetString("once")
	favorites := foh.LoadFavorites("")
	model := cfg.ResolveModel(modelFlag, favorites.IDs)

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	key, err := cfg.ResolveAPIKey(ctx)
	if err != nil {
		return err
	}
	client := foh.NewClient(cfg.ResolvedBaseURL(), key)
	cwd, _ := os.Getwd()
	workspace := foh.WorkspaceName()
	host, _ := os.Hostname()
	var agent *foh.Agent
	composer := foh.PromptComposer{Cwd: cwd, ConfigPrompt: cfg.SystemPrompt, ExtraPath: promptFlag, Header: func() string {
		m := model
		if agent != nil {
			m = agent.Model()
		}
		ws := workspace
		if ws == "" {
			ws = "default"
		}
		return fmt.Sprintf("host: %s\ncwd: %s\nworkspace: %s\nmodel: %s\nkit: %s\ntime: %s",
			host, cwd, ws, m, version.String(), time.Now().UTC().Format(time.RFC3339))
	}}
	system, err := composer.Compose()
	if err != nil {
		return err
	}
	session, err := foh.OpenSession("", key)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "hev foh: session trace disabled: could not create the sessions directory")
		session = nil
	}
	defer session.Close()
	session.Write(foh.Record{Type: "meta", Model: model, Cwd: cwd, Content: "hev foh " + version.String()})
	tools := foh.NewToolset(cwd, nil, key)
	agent = foh.NewAgent(client, tools, model, system)
	agent.Session = session

	if strings.TrimSpace(once) != "" {
		return runFOHOnce(ctx, agent, once, out, cmd.ErrOrStderr())
	}
	catalog := &foh.Catalog{Client: client}
	tui := foh.NewTUI(foh.TUIOptions{
		Agent: agent, Favorites: favorites, Composer: composer, Version: version.String(),
		Cwd: cwd, Workspace: workspace, Color: color,
		Catalog: func(ctx context.Context) ([]foh.Model, foh.CatalogSource, error) {
			return catalog.Models(ctx, false)
		},
	})
	_, err = tea.NewProgram(tui, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// runFOHOnce streams one turn: text to stdout, tool activity to stderr.
func runFOHOnce(ctx context.Context, agent *foh.Agent, message string, out, errOut io.Writer) error {
	wroteText := false
	err := agent.Run(ctx, message, func(ev foh.Event) {
		switch ev.Kind {
		case foh.EventText:
			fmt.Fprint(out, ev.Text)
			wroteText = true
		case foh.EventToolStart:
			fmt.Fprintf(errOut, "· %s %s\n", ev.Call.Function.Name, ev.Call.Function.Arguments)
		case foh.EventToolResult:
			if ev.Result.Card != nil {
				fmt.Fprintf(errOut, "◆ %s %s: %s\n", ev.Result.Card.Tool, ev.Result.Card.Action, ev.Result.Card.Summary)
			} else if ev.Result.IsError {
				fmt.Fprintf(errOut, "  ✗ %s\n", firstLine(ev.Result.Content))
			}
		}
	})
	if wroteText {
		fmt.Fprintln(out)
	}
	return err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
