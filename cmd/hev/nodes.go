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
	"golang.org/x/crypto/ssh"
)

func newNodesCommand() *cobra.Command {
	var address, user, keyfile, config string
	var ttl time.Duration
	var administrators, asJSON bool
	var allow []string
	root := &cobra.Command{Use: "nodes", Short: "Invite teammates and manage SSH dispatch access"}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	var long bool
	authorized := func() (string, string, error) {
		if administrators && runtime.GOOS != "windows" {
			return "", "", fmt.Errorf("--administrators requires Windows")
		}
		h, e := os.UserHomeDir()
		if e != nil {
			return "", "", e
		}
		exe, e := os.Executable()
		if e != nil {
			return "", "", e
		}
		return nodes.AuthorizedPath(runtime.GOOS, h, os.Getenv("ProgramData"), administrators), exe, nil
	}
	invite := &cobra.Command{Use: "invite NAME", Short: "Invite a teammate with a short code (or --long for a pasted code)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		b, e := os.ReadFile(keyfile)
		if e != nil {
			return e
		}
		i := nodes.Invite{Name: args[0], Address: address, User: user, HostKey: string(b)}
		if long {
			if len(allow) > 0 {
				return fmt.Errorf("--allow applies at accept for --long invitations")
			}
			code, e := nodes.Default().Invite(i, ttl)
			if e == nil {
				fmt.Fprintln(cmd.OutOrStdout(), code)
			}
			return e
		}
		path, exe, e := authorized()
		if e != nil {
			return e
		}
		r, e := nodes.Default().ShortInvite(i, ttl, allow, nodes.EnrollKey{Path: path, Exe: exe, Admin: administrators})
		if e != nil {
			return e
		}
		join := "hev nodes join " + r.User + "@" + r.Address + " " + r.Code()
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"name": r.Name, "code": r.Code(), "join": join, "expires": r.Expires})
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Invite code for %s: %s (single use, expires %s, locks after %d wrong tries)\n", r.Name, r.Code(), r.Expires.Local().Format("2006-01-02 15:04 MST"), nodes.MaxAttempts)
		fmt.Fprintf(out, "On %s's machine: %s\n", r.Name, join)
		fmt.Fprintf(out, "Then check the fingerprint with them and approve: hev nodes accept %s\n", r.Name)
		return nil
	}}
	invite.Flags().StringVar(&address, "address", "", "Host's reachable SSH address (required)")
	invite.Flags().StringVar(&user, "user", "", "SSH account (required)")
	defaultKey := "/etc/ssh/ssh_host_ed25519_key.pub"
	if runtime.GOOS == "windows" {
		defaultKey = filepath.Join(os.Getenv("ProgramData"), "ssh", "ssh_host_ed25519_key.pub")
	}
	invite.Flags().StringVar(&keyfile, "host-key", defaultKey, "Host public key file")
	invite.Flags().DurationVar(&ttl, "expires", time.Hour, "Invitation lifetime (maximum 24h)")
	invite.Flags().StringSliceVar(&allow, "allow", nil, "Extra capability granted on accept: repos")
	invite.Flags().BoolVar(&administrators, "administrators", false, "Use Windows administrator authorized_keys for the enrollment key")
	invite.Flags().BoolVar(&long, "long", false, "Print a pasted base64 invitation instead of a short code (older flow)")
	root.AddCommand(invite)
	join := &cobra.Command{Use: "join [USER@]HOST CODE | join LONG-CODE", Short: "Join a host with its short invite code", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		if len(args) == 1 {
			request, e := nodes.Default().Join(args[0], home, dispatch.ConfigPath(config))
			if e != nil {
				return e
			}
			fmt.Fprintln(cmd.OutOrStdout(), request)
			fmt.Fprintln(cmd.ErrOrStderr(), "Send this request to the owner for: hev nodes accept REQUEST")
			// Join never edits harness homes: setup is a separate, consenting step.
			fmt.Fprintln(cmd.ErrOrStderr(), "Then teach your agents to dispatch here: hev skills setup")
			return nil
		}
		j, e := nodes.Default().JoinShort(args[0], args[1], home, dispatch.ConfigPath(config), nodes.SSHDial(args[0]))
		if e != nil {
			return e
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Joined %s as %s (host key %s). Waiting for the owner to approve.\n", j.Address, j.Name, j.Fingerprint)
		fmt.Fprintf(cmd.OutOrStdout(), "Your key: %s\n", keyFingerprint(home, j.Name))
		fmt.Fprintf(cmd.OutOrStdout(), "Read your key fingerprint to the owner; once they run hev nodes accept, dispatch with --host %s\n", j.Name)
		// Join never edits harness homes: setup is a separate, consenting step.
		fmt.Fprintln(cmd.ErrOrStderr(), "Then teach your agents to dispatch here: hev skills setup")
		return nil
	}}
	join.Flags().StringVar(&config, "config", "", "Dispatch TOML configuration")
	root.AddCommand(join)
	root.AddCommand(&cobra.Command{Use: "enroll", Hidden: true, Short: "Short-code enrollment (forced command of the enrollment key)", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		// An idle or stalled exchange must not hold the session open.
		timer := time.AfterFunc(2*time.Minute, func() { os.Exit(1) })
		defer timer.Stop()
		return nodes.Default().Enroll(cmd.InOrStdin(), cmd.OutOrStdout())
	}})
	accept := &cobra.Command{Use: "accept NAME | accept REQUEST", Short: "Approve a pending teammate by name (or a pasted --long request)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path, exe, e := authorized()
		if e != nil {
			return e
		}
		var a nodes.Access
		if len(args[0]) <= 64 && nodes.ValidName(args[0]) {
			a, e = nodes.Default().AcceptPending(args[0], path, exe, administrators, allow)
		} else {
			a, e = nodes.Default().AcceptWith(args[0], path, exe, administrators, allow)
		}
		if e != nil {
			return e
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Accepted %s\n", a.Name)
		return nil
	}}
	accept.Flags().BoolVar(&administrators, "administrators", false, "Use Windows administrator authorized_keys and ACLs")
	accept.Flags().StringSliceVar(&allow, "allow", nil, "Extra capability for this teammate: repos (add and remove repositories under [admission] add_from)")
	root.AddCommand(accept)
	pending := &cobra.Command{Use: "pending", Short: "List open, locked and pending short invitations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		invites, e := invitations()
		if e != nil || asJSON {
			if e == nil {
				e = json.NewEncoder(cmd.OutOrStdout()).Encode(invites)
			}
			return e
		}
		printInvitations(cmd, invites)
		return nil
	}}
	root.AddCommand(pending)
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
		invites, e := invitations()
		if e != nil {
			return e
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Access  []nodes.Access    `json:"access"`
				Joined  []nodes.Joined    `json:"joined"`
				Invites []invitationEntry `json:"invites"`
			}{access, joined, invites})
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
		printInvitations(cmd, invites)
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

type invitationEntry struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Expires     time.Time `json:"expires"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Allow       []string  `json:"allow,omitempty"`
}

// invitations tidies expired invitations when it can (a busy lock only
// delays that) and lists the rest without their secrets.
func invitations() ([]invitationEntry, error) {
	_ = nodes.Default().Tidy()
	all, e := nodes.Default().Invitations()
	if e != nil {
		return nil, e
	}
	out := []invitationEntry{}
	now := time.Now()
	for _, r := range all {
		x := invitationEntry{Name: r.Name, State: r.State(now), Expires: r.Expires, Allow: r.Allow}
		if r.Pending != nil {
			x.Fingerprint = r.Pending.Fingerprint
		}
		out = append(out, x)
	}
	return out, nil
}

func printInvitations(cmd *cobra.Command, invites []invitationEntry) {
	for _, x := range invites {
		line := fmt.Sprintf("%s invite=%s expires=%s", x.Name, x.State, x.Expires.Format(time.RFC3339))
		if x.Fingerprint != "" {
			line += " key=" + x.Fingerprint + " (accept: hev nodes accept " + x.Name + ")"
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
}

// keyFingerprint reads the public key join wrote for alias.
func keyFingerprint(home, alias string) string {
	b, e := os.ReadFile(filepath.Join(home, ".ssh", alias+".pub"))
	if e != nil {
		return "unknown"
	}
	k, _, _, _, e := ssh.ParseAuthorizedKey(b)
	if e != nil {
		return "unknown"
	}
	return ssh.FingerprintSHA256(k)
}
