package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/platformops"
	"github.com/spf13/cobra"
)

const earnedDispatches = 5
const earnedTimeout = 2 * time.Second

type earnedCode struct {
	Code       string    `json:"code"`
	CreditText string    `json:"credit_text"`
	Expires    time.Time `json:"expires"`
}
type signupState struct {
	InstallID  string            `json:"install_id"`
	Dispatches int               `json:"dispatches"`
	Days       map[string]string `json:"days,omitempty"`
	Attempt    time.Time         `json:"attempt,omitempty"`
	Finished   bool              `json:"finished,omitempty"`
	Earned     *earnedCode       `json:"earned,omitempty"`
}

func signupPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "kit", "signup.json")
}

// All state changes use the same short file lock, including concurrent launches.
// Network I/O never holds this lock. A lease recovers a killed request helper.
func updateSignup(fn func(*signupState)) bool {
	path := signupPath()
	if platformops.MkdirAll(filepath.Dir(path), 0700) != nil {
		return false
	}
	release, err := platformops.LockPath(path+".lock", false)
	if err != nil {
		return false
	}
	defer release()
	var s signupState
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false
	}
	if len(raw) > 0 && json.Unmarshal(raw, &s) != nil {
		return false
	}
	fn(&s)
	raw, err = json.Marshal(s)
	return err == nil && platformops.WriteFileAtomic(path, raw, 0600) == nil
}

func signupConfig() (map[string]any, bool) {
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return nil, false
	}
	if os.Getenv("HEV_NO_HINTS") == "1" || doc["hints"] == false {
		return doc, false
	}
	kit, _ := doc["kit"].(map[string]any)
	key, _ := kit["key"].(string)
	return doc, key == ""
}
func signupHuman(cmd *cobra.Command) bool {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	// Human-formatted non-TTY output is also consumed by agents. These selected
	// command families deliberately allow it; JSON never carries marketing text.
	return !jsonOutput
}
func signupHint(cmd *cobra.Command, family string) {
	if _, ok := signupConfig(); !ok || !signupHuman(cmd) {
		return
	}
	show := false
	if !updateSignup(func(s *signupState) {
		today := time.Now().UTC().Format("2006-01-02")
		if s.Days == nil {
			s.Days = map[string]string{}
		}
		if s.Days[family] != today {
			s.Days[family] = today
			show = true
		}
	}) {
		return
	}
	if show {
		fmt.Fprintln(cmd.OutOrStdout(), "Share traces and the board across machines and your team: hev login")
	}
}

// Called only after the host accepted a start RPC. Queues, rework and failed
// admissions do not pass this point. Count even when display is disabled.
func signupDispatch(cmd *cobra.Command, remote bool, launch func()) {
	eligible := false
	_, enabled := signupConfig()
	if !updateSignup(func(s *signupState) {
		s.Dispatches++
		eligible = enabled && signupHuman(cmd) && s.Dispatches >= earnedDispatches && !s.Finished
	}) {
		return
	}
	if eligible {
		launch()
	}
	if remote {
		signupHint(cmd, "dispatch")
	}
}
func launchEarnedHelper(cmd *cobra.Command) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	child := exec.Command(exe, "signup-earned")
	child.Stdout = cmd.OutOrStdout()
	// No stderr: campaign failures must never turn a successful launch into an error.
	if child.Start() == nil {
		go func() { _ = child.Wait() }()
	}
}

func fetchEarned(ctx context.Context, out io.Writer) {
	doc, enabled := signupConfig()
	if !enabled {
		return
	}
	service := strings.TrimRight(resolveKitService("", doc), "/")
	if !validKitURL(service) {
		return
	}
	var id string
	var count int
	now := time.Now().UTC()
	claimed := false
	if !updateSignup(func(s *signupState) {
		if s.Dispatches < earnedDispatches || s.Finished || (!s.Attempt.IsZero() && now.Sub(s.Attempt) < 10*time.Second) {
			return
		}
		if s.InstallID == "" {
			s.InstallID = loginRandom()
		}
		id, count, s.Attempt, claimed = s.InstallID, s.Dispatches, now, true
	}) || !claimed {
		return
	}
	body, _ := json.Marshal(struct {
		InstallID  string `json:"install_id"`
		Dispatches int    `json:"dispatches"`
	}{id, count})
	ctx, cancel := context.WithTimeout(ctx, earnedTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", service+"/v1/codes/earned", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: earnedTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	var code *earnedCode
	finished := false
	if err == nil {
		defer resp.Body.Close()
		finished = resp.StatusCode == 204 || resp.StatusCode >= 400 && resp.StatusCode < 500
		if resp.StatusCode == 200 {
			var c earnedCode
			if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&c) == nil && validEarned(c) && c.Expires.After(time.Now()) {
				code = &c
				finished = true
			}
		}
	}
	_, stillEnabled := signupConfig()
	if !stillEnabled {
		code = nil
	}
	saved := updateSignup(func(s *signupState) { s.Attempt = time.Time{}; s.Finished = finished; s.Earned = code })
	if saved && code != nil {
		fmt.Fprintf(out, "You've dispatched %d runs. Here's a code for Field Kit: %s (%s).\nSign up: hev login --code %s\n", count, code.Code, code.CreditText, code.Code)
	}
}
func validEarned(c earnedCode) bool {
	if c.Code == "" || len(c.Code) > 256 || len(c.CreditText) > 512 {
		return false
	}
	for _, v := range c.Code {
		if !(v >= 'a' && v <= 'z' || v >= 'A' && v <= 'Z' || v >= '0' && v <= '9' || v == '-' || v == '_') {
			return false
		}
	}
	for _, v := range c.CreditText {
		if v < 32 || v == 127 {
			return false
		}
	}
	return true
}
func showEarnedCode(cmd *cobra.Command) {
	if _, ok := signupConfig(); !ok || !signupHuman(cmd) {
		return
	}
	var s signupState
	raw, err := os.ReadFile(signupPath())
	if err == nil && json.Unmarshal(raw, &s) == nil && s.Earned != nil && validEarned(*s.Earned) && s.Earned.Expires.After(time.Now()) {
		fmt.Fprintf(cmd.OutOrStdout(), "Field Kit code: %s (%s). Sign up: hev login --code %s\n", s.Earned.Code, s.Earned.CreditText, s.Earned.Code)
	}
}
func init() {
	rootCmd.AddCommand(&cobra.Command{Use: "status", Short: "Show machine status", Args: cobra.NoArgs, RunE: daemonStatusCmd.RunE})
	rootCmd.AddCommand(&cobra.Command{Use: "signup-earned", Hidden: true, Args: cobra.NoArgs, Run: func(cmd *cobra.Command, _ []string) { fetchEarned(cmd.Context(), cmd.OutOrStdout()) }})
	rootCmd.PersistentPostRun = signupPostRun
}

func signupPostRun(cmd *cobra.Command, args []string) {
	names := strings.Fields(cmd.CommandPath())
	if len(names) >= 2 && names[1] == "board" && boardHintOutput(args) {
		signupHint(cmd, "board")
	}
	if len(names) >= 3 && names[1] == "traces" && names[2] == "search" {
		signupHint(cmd, "traces")
	}
	if cmd.CommandPath() == "hev status" || cmd.CommandPath() == "hev daemon status" || cmd.Name() == "s" {
		showEarnedCode(cmd)
	}
}

func boardHintOutput(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return false
		}
	}
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "ls", "list", "boards", "read", "show", "search", "find", "post", "new", "reply", "whoami":
		return true
	}
	return strings.HasPrefix(args[0], "-") && args[0] != "--help" && args[0] != "-h"
}
