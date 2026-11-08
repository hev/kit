package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
	"github.com/spf13/cobra"
)

func newNodesCommand() *cobra.Command {
	var address, user, keyfile, config string
	var ttl time.Duration
	var administrators, asJSON bool
	var allow []string
	root := &cobra.Command{Use: "nodes", Short: "Invite teammates and manage SSH dispatch access"}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	invite := &cobra.Command{Use: "invite NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		b, e := os.ReadFile(keyfile)
		if e != nil {
			return e
		}
		code, e := nodes.Default().Invite(nodes.Invite{Name: args[0], Address: address, User: user, HostKey: string(b)}, ttl)
		if e == nil {
			fmt.Fprintln(cmd.OutOrStdout(), code)
		}
		return e
	}}
	invite.Flags().StringVar(&address, "address", "", "Host's reachable SSH address (required)")
	invite.Flags().StringVar(&user, "user", "", "SSH account (required)")
	defaultKey := "/etc/ssh/ssh_host_ed25519_key.pub"
	if runtime.GOOS == "windows" {
		defaultKey = filepath.Join(os.Getenv("ProgramData"), "ssh", "ssh_host_ed25519_key.pub")
	}
	invite.Flags().StringVar(&keyfile, "host-key", defaultKey, "Host public key file")
	invite.Flags().DurationVar(&ttl, "expires", time.Hour, "Invitation lifetime (maximum 24h)")
	root.AddCommand(invite)
	join := &cobra.Command{Use: "join CODE", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		request, e := nodes.Default().Join(args[0], home, dispatch.ConfigPath(config))
		if e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), request)
		fmt.Fprintln(cmd.ErrOrStderr(), "Send this request to the owner for: hev nodes accept REQUEST")
		// Join never edits harness homes: setup is a separate, consenting step.
		fmt.Fprintln(cmd.ErrOrStderr(), "Then teach your agents to dispatch here: hev skills setup")
		return nil
	}}
	join.Flags().StringVar(&config, "config", "", "Dispatch TOML configuration")
	root.AddCommand(join)
	accept := &cobra.Command{Use: "accept REQUEST", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if administrators && runtime.GOOS != "windows" {
			return fmt.Errorf("--administrators requires Windows")
		}
		h, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		a, e := nodes.Default().AcceptWith(args[0], nodes.AuthorizedPath(runtime.GOOS, h, os.Getenv("ProgramData"), administrators), exe, administrators, allow)
		if e != nil {
			return e
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Accepted %s\n", a.Name)
		return nil
	}}
	accept.Flags().BoolVar(&administrators, "administrators", false, "Use Windows administrator authorized_keys and ACLs")
	accept.Flags().StringSliceVar(&allow, "allow", nil, "Extra capability for this teammate: repos (add and remove repositories under [admission] add_from)")
	root.AddCommand(accept)
	for _, on := range []bool{true, false} {
		on := on
		use, short := "allow", "Grant a teammate a capability (repos)"
		if !on {
			use, short = "deny", "Withdraw a teammate capability (repos)"
		}
		root.AddCommand(&cobra.Command{Use: use + " NAME CAPABILITY", Short: short, Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			a, e := nodes.Default().SetAllow(args[0], args[1], on)
			if e != nil {
				return e
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s allow=%s\n", a.Name, allowed(a))
			return nil
		}})
	}
	ls := &cobra.Command{Use: "ls", Aliases: []string{"list"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		access, e := nodes.Default().Accesses()
		if e != nil {
			return e
		}
		joined, e := nodes.Default().Joined()
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Access []nodes.Access `json:"access"`
				Joined []nodes.Joined `json:"joined"`
			}{access, joined})
		}
		for _, a := range access {
			last := "never"
			if !a.LastUsed.IsZero() {
				last = a.LastUsed.Format(time.RFC3339)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s added=%s last-used=%s allow=%s\n", a.Name, a.Added.Format(time.RFC3339), last, allowed(a))
		}
		for _, j := range joined {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", j.Name, j.Address, j.Fingerprint)
		}
		return nil
	}}
	root.AddCommand(ls, &cobra.Command{Use: "remove NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error { return nodes.Default().Remove(args[0]) }})
	return root
}

func allowed(a nodes.Access) string {
	if len(a.Allow) == 0 {
		return "-"
	}
	return strings.Join(a.Allow, ",")
}
