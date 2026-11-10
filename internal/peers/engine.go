package peers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/pflag"
)

// Transport is the remote boundary. Local operations never use it.
type Transport interface {
	Execute(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}
type SSHTransport struct{ Quiet bool }

func (t SSHTransport) Execute(ctx context.Context, host string, args []string, in io.Reader, out, errout io.Writer) error {
	if strings.HasPrefix(host, "-") || strings.ContainsAny(host, "/\n\r\x00 ") {
		return fmt.Errorf("invalid SSH host")
	}
	var body []byte
	var err error
	if len(args) >= 3 && args[0] == "send" && args[2] == "-" {
		body, err = io.ReadAll(in)
		if err != nil {
			return err
		}
	}
	payload, err := json.Marshal(RPCRequest{Args: args, Stdin: string(body)})
	if err != nil {
		return err
	}
	q := dispatch.Request{Action: "peers", Note: string(payload)}
	if len(args) >= 3 && args[0] == "send" && strings.HasPrefix(args[1], "d-") {
		for _, arg := range args[3:] {
			if arg == "--inbox-only" || arg == "--id" || arg == "--order" || arg == "--run" {
				return fmt.Errorf("%s is unsupported for dispatch peer-send", arg)
			}
		}
		message := args[2]
		if message == "-" {
			message = string(body)
		}
		message = strings.TrimSpace(message)
		if message == "" {
			return fmt.Errorf("empty message")
		}
		sender := "unknown"
		for i := 3; i+1 < len(args); i++ {
			if args[i] == "--from" {
				sender = args[i+1]
			}
		}
		q = dispatch.Request{Action: "peer-send", Name: strings.TrimPrefix(args[1], "d-"), Note: messageHeader(sender) + "\n" + message}
	}
	request, err := json.Marshal(q)
	if err != nil {
		return err
	}
	// A per-call deadline as in compat: a host that accepts and then hangs
	// must not hold the caller.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", "--", host, "hev dispatch rpc")
	c.WaitDelay = 2 * time.Second
	if !t.Quiet && len(args) > 0 && args[0] == "send" {
		fmt.Fprintln(errout, "using ssh")
	}
	c.Stdin = bytes.NewReader(request)
	var response bytes.Buffer
	c.Stdout, c.Stderr = &response, errout
	if err := c.Run(); err != nil {
		return fmt.Errorf("SSH peers RPC: %w; delivery outcome may be unknown, inspect before retrying", err)
	}
	var result dispatch.Response
	if err := json.Unmarshal(response.Bytes(), &result); err != nil {
		return fmt.Errorf("invalid peers RPC response; delivery outcome may be unknown: %w", err)
	}
	if _, err := io.WriteString(out, result.Log); err != nil {
		return err
	}
	if q.Action == "peer-send" {
		if result.Error != "" {
			return fmt.Errorf("remote peers: %s", result.Error)
		}
		_, err := fmt.Fprintln(out, result.Message)
		return err
	}
	if _, err := io.WriteString(errout, result.Message); err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("remote peers: %s", result.Error)
	}
	return nil
}

type DeliveryStatus string

const (
	DeliveredSession DeliveryStatus = "delivered to session inbox"
	DeliveredRun     DeliveryStatus = "delivered to run inbox"
	HeldRun          DeliveryStatus = "held for ended run"
)

type Engine struct {
	deliveryStatus *DeliveryStatus
	State, Host    string
	Transport      Transport
	Storage        Store
	inboxOnly      bool
	remote         bool
	// Command is injectable for platform and terminal acceptance tests.
	Command func(context.Context, string, io.Reader, ...string) (string, error)
}

// hostname is replaced in tests that stand in for another machine.
var hostname = os.Hostname

func New() *Engine {
	home, _ := os.UserHomeDir()
	host := os.Getenv("PEERS_SELF")
	if host == "" {
		host, _ = hostname()
		host = dispatch.PeersLabel(host)
	}
	state := os.Getenv("HEV_PEERS_STATE")
	if state == "" {
		state = filepath.Join(home, ".local", "state", "peers-v2")
	}
	return &Engine{State: state, Host: host, Transport: configuredTransport{}, Command: command}
}
func command(ctx context.Context, name string, in io.Reader, args ...string) (string, error) {
	if name == "tmux" {
		// Without a UTF-8 locale (launchd, systemd, ssh without LANG) the
		// tmux client prints control characters, including the tab that
		// separates list-panes fields, as "_". -u keeps output independent
		// of the caller's locale.
		args = append([]string{"-u"}, args...)
	}
	c := exec.CommandContext(ctx, name, args...)
	c.Stdin = in
	c.Env = os.Environ()
	if name == "ps" {
		c.Env = append(c.Env, "LC_ALL=C")
	}
	b, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}
func (e *Engine) tmux(ctx context.Context, args ...string) (string, error) {
	return e.Command(ctx, "tmux", nil, args...)
}

type Peer struct {
	Headless bool   `json:"headless"`
	PanePID  int    `json:"-"`
	Address  string `json:"address"`
	Session  string `json:"session"`
	Pane     string `json:"pane"`
	Agent    string `json:"agent"`
	State    string `json:"state"`
	Cwd      string `json:"cwd"`
	Title    string `json:"title"`
	Inbox    bool   `json:"inbox"`
	Run      bool   `json:"run"`
	Identity string `json:"recipient_incarnation"`
	PID      int    `json:"agent_pid"`
}

func (e *Engine) identity(ctx context.Context, session string) (string, error) {
	raw, err := e.tmux(ctx, "display-message", "-p", "-t", "="+session+":", "#{pid}:#{session_id}:#{session_created}")
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid tmux identity")
	}
	start, err := e.Command(ctx, "ps", nil, "-o", "lstart=", "-p", parts[0])
	if err != nil {
		return "", err
	}
	birth, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(start), " "), time.Local)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%.1f", e.Host, raw, float64(birth.Unix()))))
	return hex.EncodeToString(sum[:]), nil
}
func harness(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	base := filepath.Base(fields[0])
	for _, name := range []string{"claude", "codex", "gemini", "opencode", "pi"} {
		if base == name || (base == "node" && strings.Contains(cmd, "/"+name+"/")) || (name == "codex" && strings.Contains(cmd, "/codex ")) {
			return name
		}
	}
	return ""
}
func (e *Engine) list(ctx context.Context, all bool) ([]Peer, error) {
	peers := []Peer{}
	if runtime.GOOS != "windows" {
		raw, err := e.tmux(ctx, "list-panes", "-a", "-F", "#{session_name}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}\t#{pane_current_path}\t#{pane_title}\t#{@peers-inbox}")
		if err != nil && !errors.Is(err, exec.ErrNotFound) && !strings.Contains(err.Error(), "no server") && !strings.Contains(err.Error(), "No such file") && !strings.Contains(err.Error(), "no sessions") {
			return nil, err
		}
		processes, _ := e.Command(ctx, "ps", nil, "-axo", "pid=,ppid=,args=")
		type proc struct {
			pid, ppid int
			cmd       string
		}
		var procs []proc
		for _, line := range strings.Split(processes, "\n") {
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			pid, _ := strconv.Atoi(f[0])
			ppid, _ := strconv.Atoi(f[1])
			procs = append(procs, proc{pid, ppid, strings.Join(f[2:], " ")})
		}
		for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 7 {
				continue
			}
			pid, _ := strconv.Atoi(f[2])
			agent := harness(f[3])
			agentPID := pid
			headless := false
			queue, seen := []int{pid}, map[int]bool{}
		search:
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				if seen[current] {
					continue
				}
				seen[current] = true
				for _, p := range procs {
					if p.pid == current {
						if h := harness(p.cmd); h != "" {
							agent, agentPID, headless = h, p.pid, headlessCommand(p.cmd)
							break search
						}
					}
				}
				for _, p := range procs {
					if p.ppid == current {
						queue = append(queue, p.pid)
					}
				}
			}
			inbox := f[6] == "on"
			if !all && agent == "" && !inbox {
				continue
			}
			id, err := e.identity(ctx, f[0])
			if err != nil {
				return nil, err
			}
			peers = append(peers, Peer{Address: e.Host + "/" + f[0], Session: f[0], Pane: f[1], Agent: agent, State: e.paneState(ctx, f[1], agent, headless), Cwd: f[4], Title: f[5], Inbox: inbox, Identity: id, PID: agentPID, PanePID: pid, Headless: headless})
		}
	}
	// Dispatch metadata, rather than a transient pane name, defines run addresses.
	dirs, err := os.ReadDir(dispatch.Root())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() || !dispatch.ValidName(d.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dispatch.Root(), d.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var m dispatch.Meta
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		runPane := ""
		filtered := peers[:0]
		for _, p := range peers {
			if p.Session == "d-"+d.Name() {
				runPane = p.Pane
			} else {
				filtered = append(filtered, p)
			}
		}
		peers = filtered
		peers = append(peers, Peer{Pane: runPane, Address: e.Host + "/d-" + d.Name(), Session: d.Name(), Agent: m.Harness, State: "run", Cwd: filepath.Join(dispatch.Root(), d.Name(), "wt"), Inbox: true, Run: true, Identity: "dispatch:" + e.Host + ":" + d.Name()})
	}
	return peers, nil
}
func find(ps []Peer, target string) (Peer, error) {
	var found []Peer
	for _, p := range ps {
		if target == p.Session || target == p.Pane || target == p.Address || (p.Run && target == strings.TrimSuffix(p.Address, "d-"+p.Session)+p.Session) || target == p.Session+":0.0" || (p.Run && target == "d-"+p.Session) {
			found = append(found, p)
		}
	}
	if len(found) != 1 {
		return Peer{}, fmt.Errorf("peer %q not found or ambiguous; use a unique address or pane", target)
	}
	return found[0], nil
}

// owner never trusts --from or PEERS_FROM for receipt mutations.
func (e *Engine) owner(ctx context.Context, ps []Peer) (Peer, error) {
	pane := os.Getenv("TMUX_PANE")
	if pane != "" {
		for _, p := range ps {
			if p.Pane == pane {
				return p, nil
			}
		}
	}
	cwd, _ := os.Getwd()
	for _, p := range ps {
		if p.Run && (cwd == p.Cwd || strings.HasPrefix(cwd, p.Cwd+string(os.PathSeparator))) {
			return p, nil
		}
	}
	// Detached tool processes can still prove ancestry to the terminal's pane PID.
	pid := os.Getpid()
	for i := 0; i < 100 && pid > 1; i++ {
		for _, p := range ps {
			if !p.Run && (p.PID == pid || p.PanePID == pid) {
				return p, nil
			}
		}
		raw, err := e.Command(ctx, "ps", nil, "-o", "ppid=", "-p", strconv.Itoa(pid))
		if err != nil {
			break
		}
		pid, _ = strconv.Atoi(strings.TrimSpace(raw))
	}
	return Peer{}, fmt.Errorf("cannot establish current recipient; run inside its pane")
}
func (e *Engine) split(address string) (string, string) {
	if h, n, ok := strings.Cut(address, "/"); ok {
		if h == e.Host {
			return "", n
		}
		return h, n
	}
	return "", address
}
func (e *Engine) Run(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) error {
	action := "ls"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}
	if action == "list" {
		action = "ls"
	}
	if action == "show" {
		action = "read"
	}
	if action == "chat" {
		return e.chat(ctx, args, in, out, errout)
	}
	fs := pflag.NewFlagSet("peers", pflag.ContinueOnError)
	fs.SetOutput(errout)
	asJSON := fs.Bool("json", false, "Print JSON")
	all := fs.BoolP("all", "a", false, "Include shells / retained history")
	peek := fs.Bool("peek", false, "Inspect without consuming")
	pending := fs.Bool("pending", false, "Replay unacknowledged messages")
	count := fs.Bool("count", false, "Unread count")
	currentAll := fs.Bool("current-all", false, "Inspect current history")
	session := fs.String("session", "", "Inbox session")
	sender := fs.String("sender", "unknown", "Held message sender")
	from := fs.String("from", os.Getenv("PEERS_FROM"), "Sender address")
	inboxOnly := fs.Bool("inbox-only", false, "Require durable recipient inbox")
	id := fs.String("id", "", "Stable message retry ID")
	order := fs.String("order", "", "Order provenance")
	runID := fs.String("run", "", "Run provenance")
	limit := fs.Int("limit", 0, "Batch size (default unlimited)")
	n := fs.IntP("lines", "n", 120, "Screen lines")
	force := fs.Bool("force", false, "Allow unknown harness screen")
	everywhere := fs.Bool("everywhere", false, "Also list PEERS_HOSTS over SSH")
	help := fs.BoolP("help", "h", false, "Help")
	if err := fs.Parse(args); err != nil {
		return err
	}
	args = fs.Args()
	if e.remote && *everywhere {
		return fmt.Errorf("remote requests must remain local")
	}
	// The HTTP adapter retains its existing opaque-ID contract.
	if action == "send" && !e.inboxOnly {
		for flag, value := range map[string]string{"id": *id, "order": *order, "run": *runID} {
			if flag == "id" && *inboxOnly {
				continue
			}
			if fs.Changed(flag) && !regexp.MustCompile(`^[A-Za-z0-9._:-]{1,160}$`).MatchString(value) {
				return fmt.Errorf("--%s needs a stable identifier (letters, digits, . _ : -)", flag)
			}
		}
	}
	if *help {
		fmt.Fprintln(out, "hev peers ls [-a] [--json] [--everywhere]\nhev peers read HOST/NAME [-n 120]\nhev peers send HOST/NAME MESSAGE|- [--from NAME] [--id ID]\nhev peers inbox [on|off|hold|consume ID...|ack ID...] [--session NAME] [--peek] [--pending] [--count] [--json]")
		return nil
	}
	if fs.Changed("limit") && (*limit < 1 || *limit > 64) {
		return fmt.Errorf("--limit must be 1..64")
	}
	if action == "send" && len(args) > 0 && !strings.ContainsAny(args[0], "/:%") {
		local, err := e.list(ctx, true)
		if err != nil {
			return err
		}
		args[0], err = e.relayAlias(args[0], local)
		if err != nil {
			return err
		}
	}
	if (action == "read" || action == "send") && len(args) > 0 {
		host, target := e.split(args[0])
		if host != "" {
			if e.remote {
				return fmt.Errorf("remote requests must remain local")
			}
			remote := []string{action, target}
			remote = append(remote, args[1:]...)
			if action == "read" {
				remote = append(remote, "-n", strconv.Itoa(*n))
			}
			if action == "send" {
				if len(args) < 2 {
					return fmt.Errorf("send NAME MESSAGE|-")
				}
				remote = []string{action, target, strings.Join(args[1:], " ")}
				if *from == "" {
					ps, err := e.list(ctx, true)
					if err != nil {
						return err
					}
					own, err := e.owner(ctx, ps)
					if err != nil {
						return err
					}
					*from = own.Address
				}
				remote = append(remote, "--from", *from)
				for flag, value := range map[string]string{"--order": *order, "--run": *runID} {
					if value != "" {
						remote = append(remote, flag, value)
					}
				}
				if *id != "" {
					remote = append(remote, "--id", *id)
				}
				if *inboxOnly {
					remote = append(remote, "--inbox-only")
				}
				if *force {
					remote = append(remote, "--force")
				}
			}
			return e.Transport.Execute(ctx, host, remote, in, out, errout)
		}
		args[0] = target
	}
	if action == "inbox" && *session != "" {
		host, target := e.split(*session)
		if host != "" {
			if e.remote {
				return fmt.Errorf("remote requests must remain local")
			}
			if !(*peek || *all || *count || *currentAll) || len(args) > 0 {
				return fmt.Errorf("remote inbox access is inspection-only")
			}
			remote := []string{"inbox", "--session", target}
			for flag, v := range map[string]bool{"--peek": *peek, "--all": *all, "--pending": *pending, "--count": *count, "--current-all": *currentAll, "--json": *asJSON} {
				if v {
					remote = append(remote, flag)
				}
			}
			return e.Transport.Execute(ctx, host, remote, in, out, errout)
		}
		*session = target
	}
	ps, err := e.list(ctx, true)
	if err != nil {
		return err
	}
	switch action {
	case "ls":
		visible := []Peer{}
		for _, p := range ps {
			if *all || p.Agent != "" || p.Inbox {
				visible = append(visible, p)
			}
		}
		inventory := map[string]any{"version": 1, "host": e.Host, "at": now(), "panes": visible}
		if !*asJSON {
			for _, p := range visible {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\tinbox=%t\n", p.Address, p.Agent, p.State, p.Cwd, p.Inbox)
			}
		}
		hosts := []any{}
		if *everywhere {
			for _, h := range strings.Fields(os.Getenv("PEERS_HOSTS")) {
				if h == e.Host {
					continue
				}
				remote := []string{"ls"}
				if *asJSON {
					remote = append(remote, "--json")
				}
				if *all {
					remote = append(remote, "-a")
				}
				var buf bytes.Buffer
				destination := out
				if *asJSON {
					destination = &buf
				}
				if err := e.Transport.Execute(ctx, h, remote, in, destination, errout); err != nil {
					return err
				}
				if *asJSON {
					var snapshot map[string]any
					if err := json.Unmarshal(buf.Bytes(), &snapshot); err != nil {
						return fmt.Errorf("host %s inventory: %w", h, err)
					}
					hosts = append(hosts, snapshot)
				}
			}
			inventory["hosts"] = hosts
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(inventory)
		}
		return nil
	case "read":
		if len(args) != 1 || *n < 1 {
			return fmt.Errorf("read NAME [-n positive-lines]")
		}
		p, err := find(ps, args[0])
		if err != nil {
			return err
		}
		if p.Run {
			b, err := os.ReadFile(filepath.Join(dispatch.Root(), p.Session, "log"))
			if err != nil {
				return err
			}
			lines := strings.Split(string(b), "\n")
			if len(lines) > *n {
				lines = lines[len(lines)-*n:]
			}
			_, err = fmt.Fprintln(out, strings.Join(lines, "\n"))
			return err
		}
		s, err := e.tmux(ctx, "capture-pane", "-p", "-t", p.Pane, "-S", "-"+strconv.Itoa(*n))
		if err == nil {
			_, err = io.WriteString(out, s)
		}
		return err
	case "send":
		if len(args) < 2 {
			return fmt.Errorf("send NAME MESSAGE|-")
		}
		p, err := find(ps, args[0])
		if err != nil {
			return err
		}
		if (e.inboxOnly || *inboxOnly) && (!p.Inbox || (*inboxOnly && p.Run)) {
			return fmt.Errorf("recipient requires a registered inbox")
		}
		body := strings.Join(args[1:], " ")
		if body == "-" {
			b, err := io.ReadAll(in)
			if err != nil {
				return err
			}
			body = string(b)
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return fmt.Errorf("empty message")
		}
		if *from == "" {
			own, err := e.owner(ctx, ps)
			if err != nil {
				return err
			}
			*from = own.Address
		}
		header := messageHeader(*from)
		if p.Run {
			r, err := dispatch.Handle(dispatch.Request{Action: "peer-send", Name: p.Session, Note: header + "\n" + body})
			if err != nil {
				return err
			}
			if e.deliveryStatus != nil {
				*e.deliveryStatus = DeliveryStatus(r.DeliveryStatus)
			}
			fmt.Fprintln(out, r.Message)
			return nil
		}
		previousHold := false
		if *id != "" {
			ms, err := e.store().Rows(e.path(p.Session))
			if err != nil {
				return err
			}
			for _, m := range ms {
				if m["id"] == *id {
					previousHold = true
				}
			}
		}
		if p.Inbox || previousHold {
			actual, err := e.identity(ctx, p.Session)
			if err != nil || actual != p.Identity {
				return fmt.Errorf("recipient lifetime changed before delivery")
			}
			record := Message{"id": *id, "from": *from, "header": header, "message": body}
			senderHost, senderName := e.split(*from)
			if senderHost == "" {
				if senderPeer, err := find(ps, senderName); err == nil {
					record["from_agent"] = senderPeer.Agent
					cwd := senderPeer.Cwd
					if home, err := os.UserHomeDir(); err == nil && (cwd == home || strings.HasPrefix(cwd, home+string(os.PathSeparator))) {
						cwd = "~" + strings.TrimPrefix(cwd, home)
					}
					record["from_cwd"] = cwd
				}
			}
			if *order != "" {
				record["order"] = *order
			}
			if *runID != "" {
				record["run"] = *runID
			}
			m, err := e.hold(p.Session, p.Identity, record, func() error {
				actual, err := e.identity(ctx, p.Session)
				if err != nil || actual != p.Identity {
					return fmt.Errorf("recipient lifetime changed before enqueue")
				}
				return nil
			})
			if err != nil {
				return err
			}
			if e.deliveryStatus != nil {
				*e.deliveryStatus = DeliveredSession
			}
			fmt.Fprintf(out, "%s: queued in inbox [id %s]; not acknowledged\n", p.Address, m["id"])
			return nil
		}
		if p.Headless {
			return fmt.Errorf("headless peer requires an inbox")
		}
		if p.Agent == "" {
			return fmt.Errorf("plain shell; register an inbox to receive messages")
		}
		screen, err := e.tmux(ctx, "capture-pane", "-e", "-p", "-t", p.Pane, "-S", "-50")
		if err != nil {
			return err
		}
		state := screenState(screen)
		if p.Agent != "claude" && p.Agent != "codex" && state != "prompt" && state != "draft" {
			state = "?"
		}
		if state == "prompt" || state == "draft" {
			return fmt.Errorf("recipient has a %s; delivery refused", state)
		}
		if p.Agent == "pi" || (!*force && state == "?") {
			return fmt.Errorf("unknown screen; register an inbox or use --force for an interactive harness")
		}
		buffer := "hev-peers-" + newID()
		if _, err = e.Command(ctx, "tmux", strings.NewReader(header+"\n"+body), "load-buffer", "-b", buffer, "-"); err != nil {
			return err
		}
		defer e.tmux(ctx, "delete-buffer", "-b", buffer)
		if _, err = e.tmux(ctx, "paste-buffer", "-p", "-b", buffer, "-t", p.Pane); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(600 * time.Millisecond):
		}
		_, err = e.tmux(ctx, "send-keys", "-t", p.Pane, "Enter")
		if err == nil {
			fmt.Fprintln(out, "delivered")
		}
		return err
	case "inbox":
		return e.inbox(ctx, ps, args, *session, *sender, *id, *asJSON, *all, *peek, *pending, *count, *currentAll, *limit, in, out)
	default:
		return fmt.Errorf("unsupported peers action %q; use ls, read, send or inbox", action)
	}
}

func headlessCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	for _, arg := range fields[1:] {
		if arg == "exec" || arg == "-p" || arg == "--print" {
			return true
		}
	}
	return false
}

// OwnAddress resolves the current participant without accepting a borrowed identity.
func (e *Engine) OwnAddress(ctx context.Context) (string, error) {
	ps, err := e.list(ctx, true)
	if err != nil {
		return "", err
	}
	p, err := e.owner(ctx, ps)
	if err != nil {
		return "", err
	}
	if p.Run {
		return "", fmt.Errorf("expedite must start from an interactive front-of-house session")
	}
	return p.Address, nil
}

func messageHeader(sender string) string {
	return fmt.Sprintf("[peer message from %s. Reply with: hev peers send %s \"...\"]", sender, sender)
}
