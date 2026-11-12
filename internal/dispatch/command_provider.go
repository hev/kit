package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CommandScripts are executable paths, never shell fragments. Each receives a
// JSON object on stdin; stdout is reserved for the protocol, stderr is private.
type CommandScripts struct {
	Create string `toml:"create"`
	Delete string `toml:"delete"`
	List   string `toml:"list"`
	Stop   string `toml:"stop"`
	Resume string `toml:"resume"`
}
type CommandProvider struct{ host Host }

func NewCommandProvider(h Host) (*CommandProvider, error) { return &CommandProvider{h}, nil }
func (p *CommandProvider) call(ctx context.Context, script string, input, output any) error {
	if script == "" {
		return fmt.Errorf("command provider operation is not configured")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, Expand(script))
	cmd.Stdin = bytes.NewReader(data)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	for _, name := range p.host.Env {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	// Never include script output in an error: CLIs can print credentials.
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("command provider script failed (output withheld)")
	}
	if output == nil {
		return nil
	}
	if err = json.Unmarshal(out, output); err != nil {
		return fmt.Errorf("command provider returned invalid JSON")
	}
	return nil
}
func (p *CommandProvider) Create(ctx context.Context, s MachineSpec) (Machine, error) {
	var m Machine
	err := p.call(ctx, p.host.Commands.Create, map[string]any{"name": s.Name, "host": s.Host, "from": s.From, "size": s.Size, "region": s.Region, "environment": s.Environment, "labels": s.Labels, "auto_stop_seconds": int(s.AutoStop.Seconds())}, &m)
	if err == nil && (m.ID == "" || m.SSH == "" || m.HostKey == "") {
		err = fmt.Errorf("command create requires id, ssh and host_key")
	}
	return m, err
}
func (p *CommandProvider) List(ctx context.Context, host string) ([]Machine, error) {
	var machines []Machine
	err := p.call(ctx, p.host.Commands.List, map[string]string{"host": host}, &machines)
	if err != nil {
		return nil, err
	}
	filtered := []Machine{}
	for _, m := range machines {
		if m.ID == "" {
			return nil, fmt.Errorf("command inventory requires machine id")
		}
		if m.Labels["hev.dispatch"] == host {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}
func (p *CommandProvider) machine(ctx context.Context, id string) (Machine, error) {
	machines, err := p.List(ctx, p.host.Name)
	if err != nil {
		return Machine{}, err
	}
	for _, m := range machines {
		if m.ID == id {
			return m, nil
		}
	}
	return Machine{}, fmt.Errorf("command machine not found")
}
func (p *CommandProvider) Transport(ctx context.Context, id string) (MachineTransport, error) {
	m, err := p.machine(ctx, id)
	return MachineTransport{Kind: TransportSSH, SSH: m.SSH, HostKey: m.HostKey}, err
}
func (p *CommandProvider) WaitReady(ctx context.Context, id string) error {
	for {
		m, err := p.machine(ctx, id)
		if err != nil {
			return err
		}
		if m.State == "ready" || m.State == "running" || m.State == "idle" {
			_, err = p.Exec(ctx, id, "true")
			return err
		}
		if !m.live() {
			return fmt.Errorf("command machine is %s", m.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func (p *CommandProvider) Exec(ctx context.Context, id, command string) (string, error) {
	transport, err := p.Transport(ctx, id)
	if err != nil {
		return "", err
	}
	return sshMachineExec(ctx, transport, command)
}
func (p *CommandProvider) Copy(ctx context.Context, id, path string) ([]byte, error) {
	out, err := p.Exec(ctx, id, "cat -- "+Quote(path))
	return []byte(out), err
}
func (p *CommandProvider) Stop(ctx context.Context, id string) error {
	if p.host.Commands.Stop == "" {
		_, err := p.Exec(ctx, id, "sudo -n systemctl poweroff --no-block")
		return err
	}
	return p.call(ctx, p.host.Commands.Stop, map[string]string{"id": id}, nil)
}
func (p *CommandProvider) Delete(ctx context.Context, id string) error {
	return p.call(ctx, p.host.Commands.Delete, map[string]string{"id": id}, nil)
}
func (p *CommandProvider) Resume(ctx context.Context, id string, ttl time.Duration) error {
	return p.call(ctx, p.host.Commands.Resume, map[string]any{"id": id, "auto_stop_seconds": int(ttl.Seconds())}, nil)
}

func sshMachineExec(ctx context.Context, t MachineTransport, command string) (string, error) {
	if t.SSH == "" || strings.HasPrefix(t.SSH, "-") || strings.ContainsAny(t.SSH, "\r\n \t") || t.HostKey == "" || strings.ContainsAny(t.HostKey, "\r\n") {
		return "", fmt.Errorf("invalid SSH transport")
	}
	dir, err := os.MkdirTemp("", "hev-host-key-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "known_hosts")
	if err = os.WriteFile(path, []byte(t.HostKey+"\n"), 0600); err != nil {
		return "", err
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + path}
	if t.Port != "" {
		args = append(args, "-p", t.Port)
	}
	if t.HostKeyAlias != "" {
		args = append(args, "-o", "HostKeyAlias="+t.HostKeyAlias)
	}
	args = append(args, "--", t.SSH, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = []string{"HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH"), "SSH_AUTH_SOCK=" + os.Getenv("SSH_AUTH_SOCK")}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cloud SSH command failed (output withheld)")
	}
	return string(out), nil
}

var _ CloudProvider = (*CommandProvider)(nil)
