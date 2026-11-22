package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

//go:embed templates/swarm.toml
var swarmTemplate string

//go:embed templates/climb.toml
var climbTemplate string

//go:embed templates/swarm-climb.toml
var swarmClimbTemplate string

func newOrchestrateTemplateCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "template swarm|climb|swarm-climb",
		Short:     "Print an editable TOML orchestrator template",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"swarm", "climb", "swarm-climb"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var template string
			switch args[0] {
			case "swarm":
				template = swarmTemplate
			case "climb":
				template = climbTemplate
			case "swarm-climb":
				template = swarmClimbTemplate
			default:
				return fmt.Errorf("unknown orchestrator template %q", args[0])
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), template)
			return err
		},
	}
}

func applySwarmTemplate(cmd *cobra.Command, text string, dry bool) error {
	template, err := dispatch.ParseOrchestratorTemplate(text)
	if err != nil {
		return err
	}
	markdown, err := template.SwarmMarkdown()
	if err != nil {
		return err
	}
	selected, err := currentWorkspace()
	if err != nil {
		return err
	}
	// The existing swarm supervisor stores and launches in the default workspace.
	if scopedOrchestratorName(selected) != "" {
		return fmt.Errorf("swarm templates currently require the unscoped default workspace")
	}
	if existing, err := dispatch.LoadOrchestrator(template.Name); err == nil {
		if !orchestratorWorkspaceMatches(selected, existing.WorkspaceName) {
			return fmt.Errorf("workspace boundary: orchestrator belongs to workspace %q", existing.WorkspaceName)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return applySwarm(cmd, markdown, dry)
}

func applySwarm(cmd *cobra.Command, text string, dry bool) error {
	plan, err := dispatch.SwarmUp(text, dry)
	if err != nil {
		return err
	}
	dispatch.WriteSwarmPlan(cmd.OutOrStdout(), plan, dry)
	if !dry {
		fmt.Fprintf(cmd.OutOrStdout(), "applied o-%s; the reconcile tick launches units\n", plan.Spec.Name)
	}
	return nil
}
