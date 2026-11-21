package main

import (
	"fmt"
	"os/exec"

	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/workspace"
	"github.com/spf13/cobra"
)

func newPickCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "pick [picker options]", Short: "Open the workspace session picker",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			binary, err := exec.LookPath("hev-pick")
			if err != nil {
				return fmt.Errorf("hev-pick is required: brew install hev/tap/hev-pick")
			}
			code, err := runPlugin(binary, args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err == nil && code != 0 {
				return &commandExit{code: code}
			}
			return err
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use: "refresh", Short: "Refresh the workspace picker cache", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := workspace.Read(layerenv.ConfigPath())
			if err != nil {
				return err
			}
			return refreshPicker(cfg)
		},
	})
	return cmd
}
