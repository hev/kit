package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/index"
	"github.com/spf13/cobra"
)

// Replaceable by fake servers in tests; production always uses hevkit.com.
var inviteBaseURL = "https://hevkit.com"
var inviteHTTP = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

var joinCmd = &cobra.Command{Use: "join <code>", Aliases: []string{"redeem"}, Short: "Join an invited hosted archive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	return runJoin(cmd.Context(), cmd.OutOrStdout(), args[0])
}}
var leaveCmd = &cobra.Command{Use: "leave", Short: "Revoke all your invite keys and delete your hosted archive", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return runLeave(cmd.Context(), cmd.OutOrStdout()) }}
var openCmd = &cobra.Command{Use: "open", Short: "Open your hosted archive in the browser", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return runOpen(cmd.Context(), cmd.OutOrStdout()) }}

func init() {
	for _, c := range []*cobra.Command{joinCmd, leaveCmd, openCmd} {
		c.SilenceErrors = true
		c.SilenceUsage = true
		rootCmd.AddCommand(c)
	}
}

type inviteResponse struct {
	Endpoint  string `json:"endpoint"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	daemon.InviteConfig
}

func invitePost(ctx context.Context, route, key string, body any, result any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inviteBaseURL+"/join/"+route, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := inviteHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("invite service did not answer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 404:
			return fmt.Errorf("unknown invite code: check the code you were sent")
		case 409:
			return fmt.Errorf("invite machine limit reached: ask your inviter for another machine")
		case 410:
			return fmt.Errorf("invite expired or revoked: ask your inviter for a new code")
		case 429:
			return fmt.Errorf("invite rate limit reached: wait and try again")
		case 401, 403:
			return fmt.Errorf("invite key rejected: run `hev join <code>` again")
		}
		return fmt.Errorf("invite service returned HTTP %d; try again later", resp.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(result); err != nil {
			return fmt.Errorf("invalid invite service response")
		}
	}
	return nil
}

func runJoin(ctx context.Context, out io.Writer, code string) error {
	if !hasLaunchd() {
		return fmt.Errorf("hev join needs macOS launchd to capture sessions")
	}
	// Reject shell overrides before redeeming a code: they would send CLI writes
	// to a different archive than the new launchd job.
	for _, name := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_STORE"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("unset %s before `hev join` so commands use your invited archive", name)
		}
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	var r inviteResponse
	if err := invitePost(ctx, "redeem", "", map[string]string{"code": code, "hostname": hostname}, &r); err != nil {
		return err
	}
	u, err := url.Parse(r.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || r.Namespace == "" || strings.ContainsAny(r.Namespace, "/ ?#") || r.Key == "" || r.KeyID == "" || r.Machine < 1 || r.MachinesLeft < 0 || r.For == "" || inviteDate(r.Until) == "" {
		return fmt.Errorf("invalid invite service response: missing or invalid archive or invite metadata")
	}
	r.Endpoint = strings.TrimRight(r.Endpoint, "/")
	if err := checkKey(ctx, r.Endpoint, r.Key, r.Namespace); err != nil {
		return fmt.Errorf("hosted gateway key check failed; run `hev join <code>` again: %w", err)
	}
	fmt.Fprintf(out, "✓ invite accepted · %s · %d machines · until %s\n", r.For, r.Machine+r.MachinesLeft, inviteDate(r.Until))
	fmt.Fprintf(out, "✓ key minted for %s · namespace %s\n", hostname, r.Namespace)
	// bootout precedes BOTH the target write and state reset. The file lock
	// waits for a launchd process's last writes, and refuses a manual daemon.
	unlock, err := stopInviteWriter(ctx, home)
	if err != nil {
		return err
	}
	defer unlock()
	if doc["local"] != nil && doc["invite"] == nil {
		fmt.Fprintln(out, "  local archive preserved in its Docker volume; local capture stopped")
	}
	if err := daemon.WriteInviteConfig(doc, r.Endpoint, r.Namespace, r.Key, r.InviteConfig); err != nil {
		return err
	}
	if err := index.ResetState(); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ archive   hev layer pro (hosted)")
	path, err := filepath.Abs(daemon.DefaultConfigPath())
	if err != nil {
		return err
	}
	bin, _, err := installSelf(home)
	if err != nil {
		return err
	}
	unlock()
	if _, err := (daemonJob(bin, home, path)).ensure(home, true); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ hevd      capturing under launchd, redaction on")
	for _, h := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, h), 0755); err != nil {
			return err
		}
	}
	if _, err := installSkills(home); err != nil {
		return fmt.Errorf("install agent skills: %w", err)
	}
	fmt.Fprintln(out, "✓ skills    installed for Claude Code and Codex")
	fmt.Fprintln(out, `  search    hev query "why did the preflight fail"`)
	fmt.Fprintf(out, "  next      hev join %s on your other machine (%d left)\n", code, r.MachinesLeft)
	fmt.Fprintln(out, "  leave     hev leave   (revokes your keys, deletes your archive)")
	return nil
}

func inviteDate(until string) string {
	for _, format := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(format, until); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// Take the same writer lock as hevd. Never signal unrelated daemon processes.
func stopInviteWriter(ctx context.Context, home string) (func(), error) {
	if hasLaunchd() {
		if _, err := (launchdJob{Label: launchdLabel()}).bootout(); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Join(home, ".hev"), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(home, ".hev", "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("capture is still writing: stop your manual `hev d` process and try again")
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	closed := false
	return func() {
		if !closed {
			closed = true
			syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			f.Close()
		}
	}, nil
}

func invitedTarget() (map[string]any, string, error) {
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return nil, "", err
	}
	target, _ := doc["layer"].(map[string]any)
	key, _ := target["api_key"].(string)
	if doc["invite"] == nil || key == "" {
		return nil, "", fmt.Errorf("no invited archive configured: run `hev join <code>` first")
	}
	return doc, key, nil
}

func runLeave(ctx context.Context, out io.Writer) error {
	doc, key, err := invitedTarget()
	if err != nil {
		return err
	}
	if err := invitePost(ctx, "leave", key, nil, nil); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unlock, err := stopInviteWriter(ctx, home)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.Remove((launchdJob{Label: launchdLabel()}).plistPath(home)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := daemon.ClearInviteConfig(doc); err != nil {
		return err
	}
	if err := index.ResetState(); err != nil {
		return err
	}
	fmt.Fprintln(out, "✓ invite left · all invite keys revoked, hosted archive deleted\n✓ hevd unloaded · hosted target cleared")
	return nil
}

var inviteOpenURL = openURL

func runOpen(ctx context.Context, out io.Writer) error {
	_, key, err := invitedTarget()
	if err != nil {
		return err
	}
	var session struct {
		URL string `json:"url"`
	}
	if err := invitePost(ctx, "session", key, nil, &session); err != nil {
		return err
	}
	u, err := url.Parse(session.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("invalid invite session URL")
	}
	return inviteOpenURL(out, session.URL)
}

func printInvite(out io.Writer) {
	cfg, err := daemon.LoadConfig()
	if err != nil || cfg.Invite == nil {
		return
	}
	i := cfg.Invite
	fmt.Fprintf(out, "Invite: %s · %d/%d machines used at last join · until %s\n", i.For, i.Machine, i.Machine+i.MachinesLeft, inviteDate(i.Until))
}
