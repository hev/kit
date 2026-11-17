package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/steer"
	"github.com/spf13/cobra"
)

const steerLong = `Steer a peers session, a dispatch run or an orchestrator (o-NAME).

Levels:
  note      next turn (default; dispatch say, orchestrate say and peers send are notes)
  urgent    injected at the agent's next tool call; file writes, git commit/push,
            hev dispatch and gh pr writes are blocked until it runs hev steer ack ID
  halt      every tool call is blocked with "end your turn now"; the run shows halted
  redirect  halt, commit the worktree as a checkpoint, and rework the same run with
            MESSAGE appended (branch and PR kept); dispatch runs only

Every message gets an ID and a receipt: queued -> injected (time, turn, tool
call) -> acked. hev steer says which delivery the target gets: tool-boundary
when kit's hooks run in it, else the turn-boundary fallback. Nothing is killed
to deliver; a halt that has not ended its turn after --halt-timeout (default
10m, [hooks] halt_timeout) becomes a hard stop of a dispatch run.

  hev steer fix-ci 'Stop: the scope changed, see #292' --level urgent
  hev steer ack st-01...           (run by the agent)
  hev steer ls fix-ci
  hev steer fix-ci 'Use the new candidate instead' --level redirect`

func init() { rootCmd.AddCommand(newSteerCommand()) }

func newSteerCommand() *cobra.Command {
	var level, host, configPath, haltTimeout string
	var asJSON bool
	c := &cobra.Command{
		Use:   "steer TARGET MESSAGE...",
		Short: "Steer a busy agent: note, urgent, halt or redirect, with receipts",
		Long:  steerLong,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.Join(args[1:], " ")
			if text == "-" {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				text = string(b)
			}
			if !steer.ValidLevel(level) {
				return fmt.Errorf("unknown --level %q (want %s)", level, strings.Join(steer.Levels, ", "))
			}
			if haltTimeout != "" {
				if d, err := time.ParseDuration(haltTimeout); err != nil || d <= 0 {
					return fmt.Errorf("--halt-timeout %q is not a positive duration", haltTimeout)
				}
			}
			r, err := sendSteer(cmd.Context(), args[0], text, level, host, configPath, haltTimeout)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			fmt.Fprintln(cmd.OutOrStdout(), r.Message)
			return nil
		},
	}
	c.Flags().StringVar(&level, "level", steer.Note, "note, urgent, halt or redirect")
	c.Flags().StringVar(&host, "host", "", "Dispatch host of the run, when its name is ambiguous")
	c.Flags().StringVar(&configPath, "config", "", "Dispatch config path")
	c.Flags().StringVar(&haltTimeout, "halt-timeout", "", "Hard-stop a halted dispatch run that has not ended its turn after this long (default [hooks] halt_timeout, 10m)")
	c.Flags().BoolVar(&asJSON, "json", false, "Print the receipt response as JSON")

	c.AddCommand(&cobra.Command{Use: "ack ID...", Short: "Acknowledge steering messages you have taken into account", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		var first *steer.Store
		cwd, _ := os.Getwd()
		if s, ok := steer.Resolve(os.Getenv, cwd, home, dispatch.Root(), tmuxSessionName); ok {
			first = &s
		}
		by := os.Getenv(steer.EnvTarget)
		if by == "" {
			by = dispatch.DetectCaller().Sender
		}
		failed := false
		for _, id := range args {
			if !steer.ValidID(id) {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: not a steer ID\n", id)
				failed = true
				continue
			}
			store, _, err := steer.Find(id, first, home, dispatch.Root())
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: not found on this host\n", id)
				failed = true
				continue
			}
			m, err := store.Ack(id, by)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", id, err)
				failed = true
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "acked %s (%s for %s); mutating tool calls it held may proceed\n", id, m.Level, m.Target)
		}
		if failed {
			return &commandExit{code: 1}
		}
		return nil
	}})

	var lsJSON bool
	ls := &cobra.Command{Use: "ls [TARGET]", Short: "List steering receipts on this host", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		stores := localSteerStores(home)
		if len(args) == 1 {
			s, ok := localSteerStore(home, args[0])
			if !ok {
				return fmt.Errorf("no local steering target %s (for a remote run use hev dispatch show)", args[0])
			}
			stores = []steer.Store{s}
		}
		var all []steer.Message
		for _, s := range stores {
			ms, _ := s.List()
			all = append(all, ms...)
		}
		if lsJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(all)
		}
		b, _ := json.Marshal(all)
		fmt.Fprint(cmd.OutOrStdout(), steerReceiptTable(string(b)))
		return nil
	}}
	ls.Flags().BoolVar(&lsJSON, "json", false, "Print receipts as JSON")
	c.AddCommand(ls)

	c.AddCommand(&cobra.Command{Use: "show ID", Short: "Print one steering message and its receipt", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		_, m, err := steer.Find(args[0], nil, home, dispatch.Root())
		if err != nil {
			return fmt.Errorf("%s: not found on this host", args[0])
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(m)
	}})
	return c
}

// steerReceiptTable renders receipts JSON as a table, with delivery latency.
func steerReceiptTable(raw string) string {
	var ms []steer.Message
	if json.Unmarshal([]byte(raw), &ms) != nil || len(ms) == 0 {
		return "no steering messages\n"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTARGET\tLEVEL\tSTATE\tDELIVERY\tQUEUED\tINJECTED\tLATENCY\tACKED\tBLOCKED\tMESSAGE")
	for _, m := range ms {
		injected, latency, acked := "-", "-", "-"
		if m.Injected != nil {
			injected = fmt.Sprintf("%s turn %d call %d (%s)", m.Injected.At.Format("15:04:05"), m.Injected.Turn, m.Injected.ToolCall, m.Injected.Event)
			latency = m.Latency().Round(100 * time.Millisecond).String()
		}
		if m.Acked != nil {
			acked = m.Acked.At.Format("15:04:05")
			if m.Acked.By != "" {
				acked += " (" + m.Acked.By + ")"
			}
		}
		text := strings.Join(strings.Fields(m.Text), " ")
		if len(text) > 60 {
			text = text[:57] + "..."
		}
		state := m.State
		if m.Escalated != nil {
			state += " (hard stop)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", m.ID, m.Target, m.Level, state, m.Delivery, m.QueuedAt.Format("15:04:05"), injected, latency, acked, m.Blocked, text)
	}
	w.Flush()
	return b.String()
}

// localSteerStore resolves a target name to a store on this host.
func localSteerStore(home, target string) (steer.Store, bool) {
	root := dispatch.Root()
	if name, ok := strings.CutPrefix(target, "o-"); ok {
		return steer.OrchestratorStore(root, name), true
	}
	if dispatch.ValidName(target) {
		if _, err := os.Stat(filepath.Join(root, target, "meta.json")); err == nil {
			return steer.RunStore(root, target), true
		}
	}
	return steer.PeersStore(home, target)
}

func localSteerStores(home string) []steer.Store {
	var out []steer.Store
	root := dispatch.Root()
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, e.Name(), "steer")); err != nil {
				continue
			}
			if name, ok := strings.CutPrefix(e.Name(), "orch-"); ok {
				out = append(out, steer.OrchestratorStore(root, name))
			} else {
				out = append(out, steer.RunStore(root, e.Name()))
			}
		}
	}
	if entries, err := os.ReadDir(filepath.Join(home, ".hev", "steer", "peers")); err == nil {
		for _, e := range entries {
			if s, ok := steer.PeersStore(home, e.Name()); ok && e.IsDir() {
				out = append(out, s)
			}
		}
	}
	return out
}

// sendSteer routes a steer to its target: a local dispatch run or
// orchestrator in process, a local peers session through its store, else a
// dispatch run on another host through the dispatch RPC.
func sendSteer(ctx context.Context, target, text, level, host, configPath, haltTimeout string) (dispatch.Response, error) {
	caller := dispatch.DetectCaller()
	q := dispatch.Request{Action: "steer", Name: target, Note: text, Level: level, HaltTimeout: haltTimeout, Caller: &caller}
	root := dispatch.Root()
	if host == "" {
		local := false
		if name, ok := strings.CutPrefix(target, "o-"); ok {
			_, err := os.Stat(dispatch.OrchestratorDir(name))
			local = err == nil
		} else if dispatch.ValidName(target) {
			_, err := os.Stat(filepath.Join(root, target, "meta.json"))
			local = err == nil
		}
		if local {
			return dispatch.Handle(q)
		}
		if !strings.Contains(target, "/") && tmuxHasSession(target) {
			return steerPeers(target, text, level, caller)
		}
	}
	if strings.Contains(target, "/") {
		if level != steer.Note {
			return dispatch.Response{}, fmt.Errorf("%s is a remote peers session: only a note reaches it (peers send); urgent, halt and redirect need the session's own host", target)
		}
		return peersSendNote(target, text)
	}
	cfg, err := dispatch.Load(dispatch.ConfigPath(configPath))
	if err != nil {
		return dispatch.Response{}, err
	}
	inv := dispatch.Inventory(cfg, host)
	var matches []string
	for n, r := range inv {
		for _, row := range r.Rows {
			if row.Name == target {
				matches = append(matches, n)
			}
		}
	}
	if len(matches) != 1 {
		return dispatch.Response{}, fmt.Errorf("no dispatch run, orchestrator or local peers session named %s (or ambiguous; select --host)", target)
	}
	h := cfg.Hosts[matches[0]]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return dispatch.RPC(ctx, h, q)
}

func tmuxHasSession(name string) bool {
	if !regexpSession(name) {
		return false
	}
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}

func regexpSession(name string) bool {
	_, ok := steer.PeersStore("/", name)
	return ok
}

func peersSendNote(target, text string) (dispatch.Response, error) {
	exe, err := os.Executable()
	if err != nil {
		return dispatch.Response{}, err
	}
	out, err := exec.Command(exe, "peers", "send", target, text).CombinedOutput()
	if err != nil {
		return dispatch.Response{}, fmt.Errorf("peers send: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return dispatch.Response{Delivery: steer.TurnBoundary, Message: "note via peers send (its inbox receipt applies): " + strings.TrimSpace(string(out))}, nil
}

// steerPeers steers a local peers session. A session whose hooks have fired
// since it started gets tool-boundary delivery; otherwise urgent falls back to
// peers send and halt to the harness's own interrupt (Escape).
func steerPeers(session, text, level string, caller dispatch.Caller) (dispatch.Response, error) {
	if level == steer.Note {
		return peersSendNote(session, text)
	}
	if level == steer.Redirect {
		return dispatch.Response{}, fmt.Errorf("redirect needs a dispatch run; for a peers session use --level halt, then send the new instruction")
	}
	home, _ := os.UserHomeDir()
	store, ok := steer.PeersStore(home, session)
	if !ok {
		return dispatch.Response{}, fmt.Errorf("invalid session name %q", session)
	}
	hooked := false
	if st := store.LoadState(); !st.LastEventAt.IsZero() {
		hooked = !st.LastEventAt.Before(tmuxSessionCreated(session))
	}
	delivery := steer.TurnBoundary
	if hooked {
		delivery = steer.ToolBoundary
	}
	from := strings.TrimSpace(caller.Sender + " " + caller.Session)
	cfg, _ := hooks.LoadConfig(layerenv.ConfigPath())
	m, err := store.Enqueue(level, text, from, delivery, cfg.HaltTimeout)
	if err != nil {
		return dispatch.Response{}, err
	}
	r := dispatch.Response{SteerID: m.ID, Delivery: delivery}
	switch {
	case hooked && level == steer.Urgent:
		r.Message = fmt.Sprintf("%s urgent queued for %s. Delivery: tool-boundary (kit hooks active in this session): injected at its next tool call; mutating calls blocked until it runs hev steer ack %s.", m.ID, session, m.ID)
	case hooked:
		r.Message = fmt.Sprintf("%s halt queued for %s. Delivery: tool-boundary: every tool call is blocked with \"end your turn now\". A peers session has no supervisor, so there is no hard stop; check it with hev peers read %s.", m.ID, session, session)
	case level == steer.Urgent:
		sent, err := peersSendNote(session, "URGENT ("+m.ID+"): "+text)
		if err != nil {
			return r, err
		}
		r.Message = fmt.Sprintf("%s urgent for %s. Delivery: turn-boundary fallback (no kit hook has fired in this session; hev hooks install wires new sessions): %s", m.ID, session, sent.Message)
	default:
		status := "sent the harness's own interrupt (Escape)"
		if paneCommand(session) == "pi" {
			// peers never types into pi, and kit has no verified pi hook.
			status = "not delivered: pi has no kit hooks and kit does not type into a pi pane; the halt stays queued"
		} else if err := exec.Command("tmux", "send-keys", "-t", "="+session+":", "Escape").Run(); err != nil {
			status = "interrupt failed: " + err.Error()
		}
		r.Message = fmt.Sprintf("%s halt for %s. Delivery: turn-boundary fallback (no kit hook has fired in this session): %s.", m.ID, session, status)
	}
	return r, nil
}

// paneCommand is the foreground command of the session's active pane.
func paneCommand(session string) string {
	b, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session+":", "#{pane_current_command}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func tmuxSessionCreated(session string) time.Time {
	b, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session+":", "#{session_created}").Output()
	if err != nil {
		return time.Time{}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
