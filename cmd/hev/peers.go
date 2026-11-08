package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/peers/compat"
	"github.com/spf13/cobra"
)

// nativePeersEngine reports whether a call needs the native notification
// engine rather than the reference-compatible CLI: dispatch notifications set
// HEV_PEERS_ENGINE=native, and --inbox-only exists only there.
func nativePeersEngine(args []string) bool {
	if os.Getenv("HEV_PEERS_ENGINE") == "native" {
		return true
	}
	for _, arg := range args {
		if arg == "--inbox-only" {
			return true
		}
	}
	return false
}

func newPeersCommand() *cobra.Command {
	c := &cobra.Command{Use: "peers [ls|read|send|wait|chat|role|inbox|managed|beacon|inbox-hook|runtime]", Short: "List, read and message local or SSH agent peers", DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true, Long: compat.Usage}
	c.RunE = func(c *cobra.Command, args []string) error {
		// Help never executes a backend; `peers chat --help` is the chat usage.
		for _, arg := range args {
			if (arg == "-h" || arg == "--help") && !(len(args) == 2 && args[0] == "chat") {
				return c.Help()
			}
		}
		if backend := os.Getenv("HEV_PEERS_BACKEND"); backend != "" && strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") != "peers" {
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
		if nativePeersEngine(args) {
			return peers.New().Run(c.Context(), args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr())
		}
		if len(args) > 0 && (args[0] == "list" || args[0] == "show") {
			args = append([]string{map[string]string{"list": "ls", "show": "read"}[args[0]]}, args[1:]...)
		}
		if code := compat.Main(args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()); code != 0 {
			return &resourceBackendExit{code}
		}
		return nil
	}
	return c
}
