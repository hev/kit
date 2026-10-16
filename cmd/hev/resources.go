package main

// The existing hev binary owns the customer command surface. Factory remains
// the resource backend and owns transport/identity/storage; this bridge never
// reads its files or starts a daemon. No edition-specific code is required.
import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var legacyWatchBrowser = runTUI

type resourceBackendExit struct{ code int }

func (e *resourceBackendExit) Error() string {
	return fmt.Sprintf("resource backend exited %d", e.code)
}
func resourceBackendCommand(verb string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   verb + " [resource[/name]] [flags]",
		Short: "Manage versioned resources through the configured resource backend",
		Long: "hev " + verb + ` uses the resource backend (factory on PATH by default).
Use hev controller status --store /absolute/existing/journal for opt-in controller
status (no default authority or activation).
Use hev api-resources -o json to discover actual capabilities. Current Order
resources are staged declarations; apply does not activate executions. Native inventory uses an explicit backend-owned catalog; a listed node does not
imply Kubernetes membership or executor readiness. No Host or Line resource is
implied. Backend transport retains the owner's identity.

Examples:
  hev apply -f resource.yaml --dry-run -o json
  hev get orders --selector team=platform --limit 100 -o yaml
  hev describe order/example
  hev api-resources --catalog /absolute/owner/catalog.json -o json
  hev get nodes --catalog /absolute/owner/catalog.json -o yaml
  hev describe node NAME --catalog /absolute/owner/catalog.json

Inventory requires an explicitly selected canonical authority. Discovery advertises
only configured callable resource providers; unreachable observations remain Unknown.
Source adapters grant no credentials or permissions. Other resource kinds, queue
views and maintenance verbs require matching implemented backend capabilities.

logs/watch/wait require implemented durable backend capabilities; unsupported
operations fail explicitly. Trace commands remain hev query/trace/ls/index. Bare hev watch retains the
interactive trace browser; hev watch RESOURCE[/NAME] selects resource observation.`,
		DisableFlagParsing: true, SilenceErrors: true, SilenceUsage: true,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if verb == "watch" && len(args) == 0 {
			return legacyWatchBrowser(cmd, args)
		}
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			return cmd.Help()
		}
		return runResourceBackend(cmd, verb, args)
	}
	return cmd
}
func init() {
	for _, verb := range []string{"apply", "get", "describe", "logs", "watch", "wait", "api-resources", "controller"} {
		rootCmd.AddCommand(resourceBackendCommand(verb))
	}
}
func runResourceBackend(cmd *cobra.Command, verb string, args []string) error {
	backend := os.Getenv("HEV_RESOURCE_BACKEND")
	if backend == "" {
		backend = "factory"
	}
	child := exec.CommandContext(cmd.Context(), backend, append([]string{verb}, args...)...)
	child.Stdin = cmd.InOrStdin()
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()
	err := child.Run()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if code < 1 {
			code = 1
		}
		return &resourceBackendExit{code: code}
	}
	raw, _ := json.Marshal(map[string]string{"reason": "ResourceBackendUnavailable", "message": "cannot start resource backend; install a matching backend or set HEV_RESOURCE_BACKEND to an executable path: " + err.Error()})
	return errors.New(string(raw))
}
