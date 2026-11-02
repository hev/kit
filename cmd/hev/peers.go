package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/hev/kit/internal/peers"
	"github.com/spf13/cobra"
)

func newPeersCommand() *cobra.Command {
	c := &cobra.Command{Use: "peers [ls|read|send|inbox]", Short: "List, read and message local or SSH agent peers", DisableFlagParsing: true}
	c.RunE = func(c *cobra.Command, args []string) error {
		if backend := os.Getenv("HEV_PEERS_BACKEND"); backend != "" {
			if len(args) == 0 || strings.HasPrefix(args[0], "-") {
				args = append([]string{"ls"}, args...)
			} else if args[0] == "list" {
				args[0] = "ls"
			} else if args[0] == "show" {
				args[0] = "read"
			}
			child := exec.CommandContext(c.Context(), backend, args...)
			child.Stdin, child.Stdout, child.Stderr = c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()
			return child.Run()
		}
		return peers.New().Run(c.Context(), args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr())
	}
	return c
}
