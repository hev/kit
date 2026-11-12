package compat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Usage is the reference's module docstring, plus the helpers this
// implementation folds in.
const Usage = `peers: see and talk to the other coding agents running in tmux.

  peers [ls] [-a] [--everywhere] [--json]
                                   every agent pane: address, harness, state, cwd, task
  peers read TARGET [-n LINES]     the last LINES of a pane (default 60)
  peers send TARGET MESSAGE|- [--id ID] [--order ID] [--run ID]
                                   submit a prompt or queue a bound inbox message
  peers wait TARGET [--timeout S]  block until TARGET is no longer working
  peers chat new MEMBER... [--title T] [-m MESSAGE|-]
                                   a group chat; chat say/add/leave/ls/show (peers chat)
  peers role [SESSION] [foh|boh|offline]  inspect/set local participation
  peers inbox [--peek|--count|--json|--all|--pending] [--limit N] [--session S]
                                   messages held for an inbox session (front of house);
                                   --all inspects history; --current-all inspects current;
                                   --pending replays consumed-unacknowledged items;
                                   inbox bind-runtime PID delegates to a live owned launcher;
                                   inbox consume ID... consumes exactly the carried IDs;
                                   inbox ack ID... explicitly acknowledges consumed messages;
                                   inbox on|off [SESSION] opts a session in or out;
                                   inbox hold [--sender X] - keeps a message that came another way
  peers managed register NAME --root ABS [--replace]
                                   bind an existing headless runner lifetime and inbox
  peers beacon                     push this machine's sessions to $PEERS_HOSTS and
                                   deliver what they queued for it (launchd, every minute)
  peers inbox-hook                 UserPromptSubmit hook: inject the bound inbox (peers-inbox-hook)
  peers inbox --wait [--timeout D] Stop hook: block until new bound inbox items, then exit 2
  peers runtime codex|claude|pi [ARGS...]
                                   launch a harness owning a receipt capability (peers-runtime)

Every message names its sender. peers finds yours from $TMUX_PANE, else from
the agent pane or run this process descends from; --from SESSION names it
outright. With none of those it refuses to send.

TARGET is a tmux session name, session:window.pane, pane id (%12), or pid:N for
a headless run outside tmux. Prefix HOST/ to reach another machine
(mini/lyr, mini/%3). --everywhere adds the hosts in $PEERS_HOSTS (default: mini oven).

Headless agents are listed too: hev dispatch runs (a worker in a tmux session
named d-NAME), which send reaches through ` + "`hev dispatch say`" + ` and read renders
from ` + "`hev dispatch logs`" + `, and bare ` + "`claude -p` / `codex exec`" + ` runs, which can be read but
take no input.

A host that can't ssh back (the mini) still sees the laptop: the laptop's
beacon leaves a snapshot there, which ls shows, read reads, and send queues
against; the next beacon delivers the message.

States: working (mid-turn), idle (waiting for a prompt), draft (someone has
typed into the input box and not sent it), prompt (a menu or permission dialog
is up), ? (not a harness peers knows how to read).

send refuses a pane in draft or prompt, and your own pane; --force overrides
the draft check only. The message goes in as a bracketed paste under a header
naming the sender and how to reply, so the receiver can tell it is a peer and
not the human.

A session with inbox on (tmux option @peers-inbox, set by ` + "`peers inbox on`" + `)
never has messages pasted into it: send, chat and relayed messages are held in
~/.local/state/peers-v2/inbox/SESSION.jsonl, whatever its state, and it reads them
with ` + "`peers inbox`" + ` on its own time.`

// Main runs one peers invocation and returns its exit status.
func Main(argv []string, in io.Reader, out, errw io.Writer) (code int) {
	p := newP(in, pyStream{w: out}, pyStream{w: errw, stderr: true})
	defer func() {
		if r := recover(); r != nil {
			if c, ok := r.(exitCode); ok {
				code = int(c)
				return
			}
			fmt.Fprintf(errw, "peers: %v\n", r)
			code = 1
		}
	}()
	p.main(argv)
	return 0
}

// cutoverReady is the reference's v2 gate. The reference refuses unless this
// host completed peers-cutover (a v2 readiness marker and a regular-file seal
// at the legacy path). Here a host with no legacy state at all has nothing to
// seal and may run; a legacy directory still demands the reviewed cutover.
func (p *P) cutoverReady() bool {
	legacy := filepath.Join(p.home, ".local/state/peers")
	info, err := os.Lstat(legacy)
	if os.IsNotExist(err) {
		return true
	}
	ready, rerr := readObj(filepath.Join(p.stateDir, "cutover-ready.json"))
	if rerr != nil {
		ready = NewObj()
	}
	return Equal(ready.Get("version"), 2) && Truthy(ready.Get("legacy_consumers_retired")) && err == nil && info.Mode().IsRegular()
}

func (p *P) main(argv []string) {
	if len(argv) > 0 && argv[0] == "inbox-hook" {
		p.inboxHook()
		return
	}
	if len(argv) > 0 && argv[0] == "runtime" {
		p.exit(p.runtime(argv[1:]))
	}
	if !p.cutoverReady() {
		p.die("peers v2 is not enabled here; run the reviewed peers-cutover stage procedure")
	}
	argv = append([]string{}, argv...)
	for i, a := range argv {
		if a == "--from" {
			if i+1 >= len(argv) {
				p.die("--from needs your session's address")
			}
			os.Setenv("PEERS_FROM", argv[i+1])
			argv = append(argv[:i:i], argv[i+2:]...)
			break
		}
	}
	if len(argv) == 0 || (strings.HasPrefix(argv[0], "-") && argv[0] != "-h" && argv[0] != "--help") {
		p.cmdLs(argv)
		return
	}
	cmd, args := argv[0], argv[1:]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Fprintln(p.out, Usage)
		return
	}
	commands := map[string]func([]string){
		"managed": p.cmdManaged, "ls": p.cmdLs, "read": p.cmdRead, "send": p.cmdSend, "chef-notify": p.cmdChefNotify,
		"wait": p.cmdWait, "beacon": p.cmdBeacon, "_recv": p.cmdRecv, "chat": p.cmdChat, "inbox": p.cmdInbox,
		"role": p.cmdRole, "_chat": p.cmdChatRecv, "_chatlog": p.cmdChatlog,
	}
	f, ok := commands[cmd]
	if !ok {
		p.die(fmt.Sprintf("unknown command %s; peers --help", pyRepr(cmd)))
	}
	f(args)
}
