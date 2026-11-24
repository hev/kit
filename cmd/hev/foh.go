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
	Use:     "kit",
	Aliases: []string{"foh"},
	Short:   "Optional native agent harness (OpenRouter or Codex subscription)",
	Long: `hev kit's optional native agent harness: a lean Go agent loop and
Bubble Tea TUI routed through OpenRouter or an OpenAI/Codex ChatGPT
subscription (codex/... models), with an in-harness model picker, a composable
system prompt and first-class hev tools (dispatch, peers, board, traces,
status).

hev kit runs without permissions. It's your responsibility to secure your
compute.

Set OPENROUTER_API_KEY (or HEV_API_KEY, or [foh].api_key_ref in
~/.hev/config.toml), or sign in to Codex with a ChatGPT plan (codex login).
Claude subscription logins are never used. See https://hevkit.com/docs/foh-harness.`,
	Args: cobra.NoArgs,
	RunE: runFOH,
}

func init() {
	f := fohCmd.Flags()
	f.String("model", "", "Model ID: an OpenRouter slug or codex/NAME (default [foh].model, HEV_FOH_MODEL or the first favorite)")
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
	cmd.SilenceUsage = true
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

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	router, secrets, err := fohProviders(ctx, cfg, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	fallback := ""
	if codex := router.Provider(foh.ProviderCodexSub); codex != nil {
		if models, _, _ := codex.Models(ctx, false); len(models) > 0 {
			fallback = models[0].ID
		}
	}
	startupFavorites := favorites.IDs
	if len(cfg.KeyScopes) > 0 {
		startupFavorites = nil
	}
	model := cfg.ResolveModel(modelFlag, startupFavorites, func(id string) bool { return router.Has(foh.ProviderOf(id)) }, fallback)
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
		return fmt.Sprintf("host: %s\ncwd: %s\nworkspace: %s\nmodel: %s\nkit: %s\ndocs: %s\nllms_txt: %s\ntime: %s",
			host, cwd, ws, m, version.String(), cfg.ResolvedDocsURL(), cfg.DocsLLMsURL(), time.Now().UTC().Format(time.RFC3339))
	}}
	system, err := composer.Compose()
	if err != nil {
		return err
	}
	session, err := foh.OpenSession("", secrets...)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "hev foh: session trace disabled: could not create the sessions directory")
		session = nil
	}
	defer session.Close()
	tools := foh.NewToolset(cwd, nil, secrets...)
	tools.DocsURL = cfg.ResolvedDocsURL()
	agent = foh.NewAgent(router, tools, model, system)
	agent.Session = session
	session.Write(foh.Record{Type: "meta", Model: model, Provider: agent.ProviderFor(model), Cwd: cwd,
		Content: "hev foh " + version.String() + " providers: " + strings.Join(router.Providers(), ",")})

	if strings.TrimSpace(once) != "" {
		return runFOHOnce(ctx, agent, once, out, cmd.ErrOrStderr())
	}
	tui := foh.NewTUI(foh.TUIOptions{
		Agent: agent, Favorites: favorites, Composer: composer, Version: version.String(),
		Cwd: cwd, Workspace: workspace, SummaryModel: cfg.SummaryModel, Color: color,
		CatalogContext: ctx,
		Catalog: func(ctx context.Context) ([]foh.Model, foh.CatalogSource, error) {
			return router.Models(ctx, false)
		},
	})
	closePeers, peerErr := tui.RegisterNativePeer(ctx)
	if peerErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "hev kit: native peers unavailable")
	}
	defer closePeers()
	_, err = tea.NewProgram(tui, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)).Run()
	if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// fohProviders builds the provider router: OpenRouter/Club Kit when a key
// resolves, and codex-sub when a Codex ChatGPT login is found (or
// [foh.codex].enabled forces it). It never reads Anthropic/Claude subscription
// credentials. A stale legacy hk_ key in implicit [kit].key is skipped with a
// warning when another provider resolves, and surfaced as ErrLegacyKitKey when
// no provider resolves. The returned secrets seed the session and tool
// scrubbers.
func fohProviders(ctx context.Context, cfg foh.Config, errOut io.Writer) (*foh.Router, []string, error) {
	if len(cfg.KeyScopes) > 0 {
		provider, err := cfg.ScopedInference()
		if err != nil {
			return nil, nil, err
		}
		return foh.NewRouter(provider), nil, nil
	}
	var providers []foh.Provider
	var secrets []string
	var implicitLegacyErr error
	ep, epErr := cfg.ResolveEndpoint(ctx)
	if epErr == nil {
		providers = append(providers, foh.NewClient(ep.BaseURL, ep.APIKey))
		secrets = append(secrets, ep.APIKey)
	} else {
		var implicitLegacy foh.ImplicitLegacyKitKeyError
		switch {
		case errors.Is(epErr, foh.ErrNoAPIKey):
			// No OpenRouter / Club Kit key configured; continue to other providers.
		case errors.As(epErr, &implicitLegacy):
			// Do not block fallback providers (such as codex-sub) on a stale [kit].key.
			implicitLegacyErr = epErr
			if k := strings.TrimSpace(cfg.Kit.Key); k != "" {
				secrets = append(secrets, k)
			}
		default:
			return nil, nil, epErr
		}
	}
	if codex, err := foh.NewCodexSub(cfg.Codex); err == nil && codex.Active() {
		providers = append(providers, codex)
		secrets = append(secrets, codex.Secrets()...)
	} else if errors.Is(err, foh.ErrAnthropicCredentials) && cfg.Codex.Enabled != nil && *cfg.Codex.Enabled {
		return nil, nil, err
	}
	if len(providers) == 0 {
		if implicitLegacyErr != nil {
			return nil, nil, implicitLegacyErr
		}
		return nil, nil, foh.ErrNoProviders
	}
	if implicitLegacyErr != nil && errOut != nil {
		fmt.Fprintln(errOut, "hev foh: skipping legacy hk_ key in [kit].key (run `hev login` to obtain a hev_ key)")
	}
	return foh.NewRouter(providers...), secrets, nil
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
