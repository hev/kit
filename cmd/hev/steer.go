package main

import (
	"context"
	"encoding/json"
	"errors"
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

Every message gets an ID and a receipt: queued -> delivered or injected -> acked.
For dispatch inbox delivery, the injected event is "inbox". Run the ack
command in the inbox entry after handling it. hev steer reports tool-boundary
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
				fmt.Fprintln(cmd.ErrOrStderr(), "invalid steer ID")
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
				if errors.Is(err, steer.ErrNotInjected) {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", id, err)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: could not record acknowledgement\n", id)
				}
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
	var lsHost, lsConfig string
	ls := &cobra.Command{Use: "ls [TARGET]", Short: "List local steering receipts or look up a dispatch target", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var all []steer.Message
		if len(args) == 1 {
			var err error
			all, err = lookupSteer(cmd.Context(), args[0], lsHost, lsConfig)
			if err != nil {
				return err
			}
		} else {
			if lsHost != "" || lsConfig != "" {
				return fmt.Errorf("a target is required with --host or --config")
			}
			home, _ := os.UserHomeDir()
			for _, s := range localSteerStores(home) {
				ms, _ := s.List()
				all = append(all, ms...)
			}
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
	ls.Flags().StringVar(&lsHost, "host", "", "Dispatch host of the target")
	ls.Flags().StringVar(&lsConfig, "config", "", "Dispatch config path")
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
			if m.Injected.Event == steer.InboxEvent {
				injected = m.Injected.At.Format("15:04:05") + " (inbox)"
			} else {
				injected = fmt.Sprintf("%s turn %d call %d (%s)", m.Injected.At.Format("15:04:05"), m.Injected.Turn, m.Injected.ToolCall, m.Injected.Event)
			}
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
		if state == steer.Injected && m.Injected != nil && m.Injected.Event == steer.InboxEvent {
			state = "delivered"
		}
		if m.Escalated != nil {
			state += " (hard stop)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", m.ID, m.Target, m.Level, state, m.Delivery, m.QueuedAt.Format("15:04:05"), injected, latency, acked, m.Blocked, text)
	}
	w.Flush()
	return b.String()
}

// localSteerDispatchTarget identifies local runs and orchestrators, including
// forwarding stubs. The dispatch RPC follows those stubs to the current owner.
func localSteerDispatchTarget(target string) bool {
	if !dispatch.ValidAddress(target) {
		return false
	}
	if name, ok := strings.CutPrefix(target, "o-"); ok {
		_, err := dispatch.LoadOrchestrator(name)
		return err == nil
	}
	_, err := os.Stat(filepath.Join(dispatch.Root(), target, "meta.json"))
	return err == nil
}

func lookupSteer(ctx context.Context, target, host, configPath string) ([]steer.Message, error) {
	q := dispatch.Request{Action: "show", Name: target}
	var r dispatch.Response
	var err error
	if host == "" && localSteerDispatchTarget(target) {
		r, err = dispatch.Handle(q)
		if err != nil {
			return nil, fmt.Errorf("could not look up local dispatch steering receipts for %s", target)
		}
	} else {
		if host == "" {
			home, _ := os.UserHomeDir()
			if store, ok := steer.PeersStore(home, target); ok {
				if _, statErr := os.Stat(store.Dir); statErr == nil || !os.IsNotExist(statErr) || tmuxHasSession(ctx, target) {
					messages, readErr := store.List()
					if readErr != nil {
						return nil, fmt.Errorf("could not read local peers steering receipts")
					}
					return messages, nil
				}
			}
		}
		r, err = remoteSteer(ctx, q, host, configPath)
		if err != nil {
			return nil, err
		}
	}
	var messages []steer.Message
	if raw := r.Files["steer.json"]; raw != "" && json.Unmarshal([]byte(raw), &messages) != nil {
		return nil, fmt.Errorf("invalid steering receipts for %s", target)
	}
	return messages, nil
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
	if host == "" {
		if localSteerDispatchTarget(target) {
			return dispatch.Handle(q)
		}
		if !strings.Contains(target, "/") && tmuxHasSession(ctx, target) {
			return steerPeers(ctx, target, text, level, caller)
		}
	}
	if strings.Contains(target, "/") {
		if level != steer.Note {
			return dispatch.Response{}, fmt.Errorf("%s is a remote peers session: only a note reaches it (peers send); urgent, halt and redirect need the session's own host", target)
		}
		return peersSendNote(target, text, false)
	}
	return remoteSteer(ctx, q, host, configPath)
}

func remoteSteer(ctx context.Context, q dispatch.Request, host, configPath string) (dispatch.Response, error) {
	if !dispatch.ValidAddress(q.Name) {
		return dispatch.Response{}, fmt.Errorf("invalid dispatch target")
	}
	cfg, err := dispatch.Load(dispatch.ConfigPath(configPath))
	if err != nil {
		if q.Action == "show" {
			return dispatch.Response{}, fmt.Errorf("could not load dispatch configuration")
		}
		return dispatch.Response{}, err
	}
	if host == "" {
		inv := dispatchInventory(cfg, "")
		var matches []string
		for _, name := range dispatch.SortedHosts(inv) {
			for _, row := range inv[name].Rows {
				if row.Name == q.Name {
					matches = append(matches, name)
					break
				}
			}
		}
		switch {
		case len(matches) > 1:
			return dispatch.Response{}, fmt.Errorf("dispatch target %s is ambiguous; select --host", q.Name)
		case len(matches) == 1:
			host = matches[0]
		default:
			for _, response := range inv {
				if response.Error != "" {
					return dispatch.Response{}, fmt.Errorf("could not locate dispatch target %s; some hosts were unreachable", q.Name)
				}
			}
			return dispatch.Response{}, fmt.Errorf("dispatch target %s not found", q.Name)
		}
	}
	h, ok := cfg.Hosts[host]
	if !ok {
		return dispatch.Response{}, fmt.Errorf("unknown dispatch host")
	}
	ctx = dispatch.WorkspaceContext(ctx, cfg.SelectedWorkspace)
	timeout := 5 * time.Minute
	if q.Action == "show" {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r, err := dispatch.RPC(ctx, h, q)
	if err != nil && q.Action == "show" {
		return dispatch.Response{}, fmt.Errorf("could not look up dispatch target %s on the selected host", q.Name)
	}
	return r, err
}

func tmuxHasSession(ctx context.Context, name string) bool {
	if !regexpSession(name) {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "tmux", "has-session", "-t", "="+name).Run() == nil
}

func regexpSession(name string) bool {
	_, ok := steer.PeersStore("/", name)
	return ok
}

func peersSendNote(target, text string, fallback bool) (dispatch.Response, error) {
	exe, err := os.Executable()
	if err != nil {
		return dispatch.Response{}, err
	}
	cmd := exec.Command(exe, "peers", "send", target, text)
	if fallback {
		cmd.Env = append(os.Environ(), "HEV_PEERS_STEER_FALLBACK=1")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return dispatch.Response{}, fmt.Errorf("peers send: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return dispatch.Response{Delivery: steer.TurnBoundary, Message: "note via peers send (its inbox receipt applies): " + strings.TrimSpace(string(out))}, nil
}

// steerPeers steers a local peers session. A session whose hooks have fired
// since it started gets tool-boundary delivery; otherwise urgent falls back to
// peers send and halt to the harness's own interrupt (Escape).
func steerPeers(ctx context.Context, session, text, level string, caller dispatch.Caller) (dispatch.Response, error) {
	if level == steer.Note {
		return peersSendNote(session, text, false)
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
	delivered := true
	switch {
	case hooked && level == steer.Urgent:
		r.Message = fmt.Sprintf("%s urgent queued for %s. Delivery: tool-boundary (kit hooks active in this session): injected at its next tool call; mutating calls blocked until it runs hev steer ack %s.", m.ID, session, m.ID)
	case hooked:
		r.Message = fmt.Sprintf("%s halt queued for %s. Delivery: tool-boundary: every tool call is blocked with \"end your turn now\". A peers session has no supervisor, so there is no hard stop; check it with hev peers read %s.", m.ID, session, session)
	case level == steer.Urgent:
		sent, err := peersSendNote(session, "URGENT ("+m.ID+"): "+text, true)
		if err != nil {
			return r, err
		}
		r.Message = fmt.Sprintf("%s urgent for %s. Delivery: turn-boundary fallback (no kit hook has fired in this session; hev hooks install wires new sessions): %s", m.ID, session, sent.Message)
	default:
		status := "sent the harness's own interrupt (Escape)"
		if paneCommand(session) == "pi" {
			// peers never types into pi, and kit has no verified pi hook.
			delivered = false
			status = "not delivered: pi has no kit hooks and kit does not type into a pi pane; the halt stays queued"
		} else if err := exec.Command("tmux", "send-keys", "-t", "="+session+":", "Escape").Run(); err != nil {
			delivered = false
			status = "interrupt failed: " + err.Error()
		}
		r.Message = fmt.Sprintf("%s halt for %s. Delivery: turn-boundary fallback (no kit hook has fired in this session): %s.", m.ID, session, status)
	}
	if delivered {
		sender := caller.Session
		if sender == "" {
			sender = caller.Sender
		}
		hooks.ObservePeerMessage(ctx, hooks.PeerMessage{
			Activity: hooks.Activity{ID: m.ID},
			From:     sender, To: session, Kind: level,
		}, text)
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
