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

// ownPeersCommand reports whether args name a command only this
// implementation provides, which never forwards to HEV_PEERS_BACKEND or the
// native engine: a backend would read `inbox --wait` as a consuming `inbox`.
func ownPeersCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "inbox" {
		for _, arg := range args {
			if arg == "--wait" {
				return true
			}
		}
	}
	return args[0] == "inbox-hook" || args[0] == "runtime"
}

// pinnedPeersArgs returns the peers argv when hev runs as a peers helper
// alias or as the hidden `hev peers-native` re-entry.
func pinnedPeersArgs(argv []string) ([]string, bool) {
	switch name := strings.TrimSuffix(filepath.Base(argv[0]), ".exe"); name {
	case "peers":
		return argv[1:], true
	case "peers-inbox-hook", "peers-runtime":
		return append([]string{strings.TrimPrefix(name, "peers-")}, argv[1:]...), true
	}
	if len(argv) > 1 && argv[1] == compat.NativeCommand {
		return argv[2:], true
	}
	return nil, false
}

// newPeersCommand builds `hev peers`. pinned callers (the helper aliases and
// their re-entrant calls) never forward to HEV_PEERS_BACKEND, and neither do
// inbox-hook, runtime and inbox --wait, which only this implementation provides.
func newPeersCommand(pinned bool) *cobra.Command {
	c := &cobra.Command{Use: "peers [ls|read|send|wait|chat|role|inbox|managed|beacon|inbox-hook|runtime]", Short: "List, read and message local or SSH agent peers", DisableFlagParsing: true, SilenceUsage: true, SilenceErrors: true, Long: compat.Usage}
	c.RunE = func(c *cobra.Command, args []string) error {
		// Help never executes a backend; `peers chat --help` is the chat usage.
		for _, arg := range args {
			if (arg == "-h" || arg == "--help") && !(len(args) == 2 && args[0] == "chat") {
				return c.Help()
			}
		}
		if backend := os.Getenv("HEV_PEERS_BACKEND"); backend != "" && !pinned && !ownPeersCommand(args) {
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
		if nativePeersEngine(args) && !ownPeersCommand(args) {
			return peers.New().Run(c.Context(), args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr())
		}
		if len(args) > 0 && (args[0] == "list" || args[0] == "show") {
			args = append([]string{map[string]string{"list": "ls", "show": "read"}[args[0]]}, args[1:]...)
		}
		if code := compat.Main(args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()); code != 0 {
			return &commandExit{code}
		}
		return nil
	}
	return c
}
