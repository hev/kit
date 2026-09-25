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
	Short: "Show the gateway's edition and what hev layer pro adds",
	Long: `Show which edition of the hev layer gateway kit is talking to, and what the
licensed edition adds. The answer is the gateway's own (GET /v2/license),
which it computes offline from its key.

  hev pro         the edition, and what pro adds
  hev pro trial   open the trial signup in your browser`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runPro(cmd.OutOrStdout())
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
	proCmd.AddCommand(proTrialCmd)
	rootCmd.AddCommand(proCmd)
}

// proFeatures is what the licensed gateway adds, in the words a kit user
// would want them in. Each row is a license feature the gateway gates
// (hevlayer.com/docs/licensing).
var proFeatures = []hevdLine{
	{"scoped keys", "a key per machine or teammate, each revocable, instead of"},
	{"", "one Turbopuffer key copied onto every machine that runs hevd"},
	{"functions", "pipelines and UDFs on write, such as your own redaction rules"},
	{"history", "search history, clickstream, checkpoints and restore"},
	{"cost", "cost and fin-ops APIs and dashboard panels"},
	{"agents", "agents managed and invoked on the gateway (Team and above)"},
}

func runPro(out io.Writer) error {
	cl, err := client("")
	if err != nil {
		return err
	}
	lic, err := cl.License()
	if err != nil {
		fmt.Fprintf(out, "edition     unknown: %s did not answer (%v)\n", cl.Endpoint, err)
	} else {
		fmt.Fprintf(out, "edition     %s on %s\n", lic.Edition(), cl.Endpoint)
		if w := lic.Warning(); w != "" {
			fmt.Fprintf(out, "  ! %s\n", w)
		}
		if lic.Valid {
			if len(lic.Features) > 0 {
				fmt.Fprintf(out, "features    %s\n", strings.Join(lic.Features, ", "))
			}
			return nil
		}
	}

	fmt.Fprintln(out, "\nhev layer pro is the licensed edition of the gateway kit runs on. It adds:")
	for _, f := range proFeatures {
		fmt.Fprintf(out, "  %-13s%s\n", f.label, f.value)
	}
	fmt.Fprintf(out, "\nTrial, key by email:  hev pro trial\nPricing:              %s\n", pricingURL)
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
// Turbopuffer key, which is the problem scoped keys solve.
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
	showHint(errOut, id, fmt.Sprintf("%d machines write to this archive with one Turbopuffer key. hev layer pro gives each its own revocable key: `hev pro`", len(hosts)), now)
}
