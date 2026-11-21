package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/redact"
	"github.com/spf13/cobra"
)

// proURL is the trial signup, tagged with where in kit the reader came from so
// that PostHog can tell a kit signup from any other.
func proURL(content string) string {
	q := url.Values{"utm_source": {"kit"}, "utm_medium": {"cli"}, "utm_campaign": {"pro"}, "utm_content": {content}}
	return "https://hevlayer.com/?" + q.Encode() + "#start-trial"
}

const pricingURL = "https://hevlayer.com/pricing"

var proCmd = &cobra.Command{
	Use:   "pro",
	Short: "Show Kit Pro license status and the configured Layer gateway edition",
	Long:  "Inspect Kit's offline license and, when configured, the Layer gateway's own edition.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		machine, _ := cmd.Flags().GetBool("json")
		return runPro(cmd, machine)
	},
}

var proTrialCmd = &cobra.Command{
	Use:   "trial",
	Short: "Open the hev layer pro trial signup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return openURL(cmd.OutOrStdout(), proURL("pro-trial"))
	},
}

func init() {
	proCmd.Flags().Bool("json", false, "Print edition and license status as JSON")
	proCmd.AddCommand(proTrialCmd)
	rootCmd.AddCommand(proCmd)
}

// proFeatures is what the licensed gateway adds, in the words a kit user
// would want them in. Each row is a license feature the gateway gates
// (hevlayer.com/docs/licensing).
var proFeatures = []hevdLine{
	{"scoped keys", "a key per machine or teammate, each revocable, instead of"},
	{"", "one hev layer key copied onto every machine that runs hevd"},
	{"functions", "pipelines and UDFs on write, such as your own redaction rules"},
	{"history", "search history, clickstream, checkpoints and restore"},
	{"cost", "cost and fin-ops APIs and dashboard panels"},
	{"agents", "agents managed and invoked on the gateway (Team and above)"},
}

func runPro(cmd *cobra.Command, machine bool) error {
	out := cmd.OutOrStdout()
	status := license.Resolve(cmd.Context())
	result := struct {
		Edition      string         `json:"edition"`
		License      license.Status `json:"license"`
		LayerEdition string         `json:"layer_edition,omitempty"`
	}{Edition: kitEdition(status), License: status}
	if !machine {
		writeKitLicense(out, status)
	}

	cl, err := client("")
	if err == nil {
		lic, err := cl.License()
		if err != nil {
			result.LayerEdition = "unknown (gateway license unavailable)"
		} else {
			result.LayerEdition = redact.PublicTokens(lic.Edition())
		}
		if !machine {
			fmt.Fprintf(out, "\nLayer gateway  %s\n", result.LayerEdition)
			if warning := lic.Warning(); warning != "" {
				fmt.Fprintf(out, "  ! %s\n", redact.PublicTokens(warning))
			}
			if lic.Valid && !lic.Community() && len(lic.Features) > 0 {
				fmt.Fprintf(out, "Layer features %s\n", redact.PublicTokens(strings.Join(lic.Features, ", ")))
			}
			if !lic.Valid || lic.Community() {
				fmt.Fprintln(out, "\nhev layer pro adds:")
				for _, f := range proFeatures {
					fmt.Fprintf(out, "  %-13s%s\n", f.label, f.value)
				}
				fmt.Fprintf(out, "\nLayer trial:  hev pro trial\nPricing:      %s\n", pricingURL)
			}
		}
	}
	if machine {
		return json.NewEncoder(out).Encode(result)
	}
	return nil
}

func openURL(out io.Writer, u string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", u)
	case "linux":
		c = exec.Command("xdg-open", u)
	}
	if c == nil || !isTerminal(out) || c.Run() != nil {
		fmt.Fprintf(out, "Open this URL in your browser:\n  %s\n", u)
	}
	return nil
}

// licenseRequiredMessage rewrites a gateway's 402 as the one sentence a
// person needs, or returns "" for any other error.
func licenseRequiredMessage(err error) string {
	feature, ok := layer.LicenseRequired(err)
	if !ok {
		return ""
	}
	if feature == "" {
		feature = "this feature"
	}
	return fmt.Sprintf("the gateway refused this: %s needs a hev layer pro license. See `hev pro`, or start a trial at %s", feature, proURL("402-"+feature))
}

// Hints are the only unasked-for words kit prints about pro, so they are
// held to the least intrusive form there is: stderr, a person at a terminal
// on both streams, not inside a coding agent or CI, each hint at most once a
// week, and none at all with HEV_NO_HINTS set. A hint never changes what a
// command does or returns.

const hintEvery = 7 * 24 * time.Hour

// agentEnv are variables the agent harnesses and CI set. Agents read kit's
// output as data: a hint there is noise in someone's context window.
var agentEnv = []string{"CLAUDECODE", "CODEX_SANDBOX", "CODEX_SANDBOX_NETWORK_DISABLED", "CI"}

func hintsAllowed() bool {
	if os.Getenv("HEV_NO_HINTS") != "" {
		return false
	}
	for _, v := range agentEnv {
		if os.Getenv(v) != "" {
			return false
		}
	}
	return isTerminal(os.Stdout) && isTerminal(os.Stderr)
}

func hintsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hev", "hints.json")
}

func readHints(path string) map[string]time.Time {
	shown := map[string]time.Time{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &shown)
	}
	return shown
}

// hintDue reports whether hint id may be shown now. It is asked before any
// work the hint would need, so a hint that is not due costs nothing.
func hintDue(id string, now time.Time) bool {
	if !hintsAllowed() {
		return false
	}
	last, ok := readHints(hintsPath())[id]
	return !ok || now.Sub(last) >= hintEvery
}

// showHint prints a hint and records when, so it waits a week to recur.
func showHint(errOut io.Writer, id, msg string, now time.Time) {
	fmt.Fprintf(errOut, "\n  ◆ %s\n    (HEV_NO_HINTS=1 silences these)\n", msg)
	path := hintsPath()
	if path == "" {
		return
	}
	shown := readHints(path)
	shown[id] = now
	if raw, err := json.Marshal(shown); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, raw, 0o644)
	}
}

// hintSharedKey fires when the archive holds sessions from more than one
// machine on a community gateway: every one of those machines holds the same
// hev layer key, which is the problem scoped keys solve.
func hintSharedKey(errOut io.Writer, cl *layer.Client, hosts map[string]bool) {
	const id = "shared-key"
	now := time.Now()
	if len(hosts) < 2 || !hintDue(id, now) {
		return
	}
	lic, err := cl.License()
	if err != nil || !lic.Community() {
		return
	}
	showHint(errOut, id, fmt.Sprintf("%d machines write to this archive with one hev layer key. hev layer pro gives each its own revocable key: `hev pro`", len(hosts)), now)
}
