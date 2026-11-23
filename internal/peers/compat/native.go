package compat

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/hev/kit/internal/peers/nativeinbox"
)

func nativeRow(peer nativeinbox.Presence) *Row {
	return &Row{Native: &peer, ID: peer.Name, Session: peer.Name, PID: peer.PID,
		AgentPID: peer.PID, Cwd: peer.Cwd, Agent: peer.Agent, Role: peer.Role,
		Activity: peer.UpdatedAt.Unix()}
}

func (p *P) withNative(rows []*Row) []*Row {
	peers, _ := nativeinbox.AtHome(p.home).List()
	for _, peer := range peers {
		ancestors := p.ps().ancestors(peer.PID)
		row := nativeRow(peer)
		replaced := false
		for i, pane := range rows {
			if pane.AgentPID == peer.PID || pane.Managed == nil && pane.ID == peer.TmuxPane && intIn(ancestors, pane.PID) {
				row.Attached = pane.Attached
				rows[i] = row
				replaced = true
				break
			}
		}
		if !replaced {
			rows = append(rows, row)
		}
	}
	return rows
}

func (p *P) rowBranch(row *Row) string {
	if row.Native != nil {
		return row.Native.Branch
	}
	return p.branch(row.Cwd)
}

func (p *P) rowStarted(row *Row) any {
	if row.Native != nil {
		return row.Native.StartedAt.Unix()
	}
	return p.agentStarted(row.AgentPID)
}

// The native harness exports its address to its child commands. Use it only
// while that incarnation is visible in this workspace.
func (p *P) nativeSender() (who, bool) {
	if os.Getenv("PEERS_NO_SELF") != "" {
		return who{}, false
	}
	from := os.Getenv("HEV_NATIVE_PEER")
	if explicit := os.Getenv("PEERS_FROM"); explicit != "" && explicit != from {
		return who{}, false
	}
	peer, err := nativeinbox.AtHome(p.home).Lookup(from)
	if err != nil || !p.allowsNative(peer) {
		return who{}, false
	}
	return who{Address: from, Agent: peer.Agent, Cwd: p.tilde(peer.Cwd), Replyable: true}, true
}

func (p *P) allowsNative(peer nativeinbox.Presence) bool {
	return participates(peer.Role) && (!p.scope.Enabled ||
		p.scope.Workspace.AllowsHost(p.selfLabel()) && p.scope.AllowsLocal(nativeRow(peer)))
}

var nativeOrigin = regexp.MustCompile(`^[A-Za-z0-9._:%$/-]{1,160}$`)

func (p *P) sendNative(address, input string, interrupt bool, metadata *Obj, replyTo string) bool {
	host, target := splitHost(address)
	if host != "" && host != p.selfLabel() && host != p.hostLabel {
		return false
	}
	store := nativeinbox.AtHome(p.home)
	peer, err := store.Lookup(target)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		p.die(err.Error())
	}
	if !p.allowsNative(peer) {
		p.die("native peer is unavailable in this workspace")
	}
	body := p.nativeBody(input)
	sender, localPeer := p.nativeSender()
	if !localPeer {
		for _, from := range []string{os.Getenv("PEERS_REMOTE_FROM"), os.Getenv("PEERS_FROM"), os.Getenv("TMUX_PANE")} {
			if nativeOrigin.MatchString(from) {
				sender.Address = from
				break
			}
		}
		if sender.Address == "" {
			sender.Address = "local"
		}
	}
	if localPeer && sender.Address == target {
		p.die("that's your own native peer")
	}
	id := metadata.Str("id")
	if id == "" {
		id = uuidHex()
	}
	mode := nativeinbox.Background
	if interrupt {
		mode = nativeinbox.Interrupt
	}
	if err := store.Send(peer, nativeinbox.Message{ID: id, From: sender.Address, Body: body, Mode: mode}); err != nil {
		p.die(err.Error())
	}
	fmt.Fprintf(p.out, "queued for %s (hev, %s) [id %s]\n", target, mode, id)
	p.observeSend(sender, target, id, replyTo, body)
	return true
}

func (p *P) nativeBody(input string) string {
	if input == "-" {
		reader := p.in
		if p.stdinB64 != nil {
			reader = base64.NewDecoder(base64.StdEncoding, strings.NewReader(*p.stdinB64))
		}
		data, err := io.ReadAll(io.LimitReader(reader, nativeinbox.MaxBodyBytes+1))
		if err != nil {
			p.die("could not read native peer message")
		}
		input = string(data)
	}
	if len(input) > nativeinbox.MaxBodyBytes {
		p.die("native peer message exceeds 16 KiB")
	}
	body := pyStrip(input)
	if body == "" {
		p.die("empty message")
	}
	return body
}
