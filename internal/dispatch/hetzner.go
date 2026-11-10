package dispatch

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

var hetznerSSHExec = sshMachineExec

// Hetzner uses no CLI and resolves credentials only at request time.
type Hetzner struct {
	host     Host
	endpoint string
	client   *http.Client
}

func NewHetzner(h Host) (*Hetzner, error) {
	return &Hetzner{h, "https://api.hetzner.cloud/v1", &http.Client{Timeout: 60 * time.Second}}, nil
}
func (p *Hetzner) api(ctx context.Context, method, path string, body, out any) error {
	token := os.Getenv(p.host.APIKeyEnv)
	if token == "" {
		return fmt.Errorf("Hetzner token environment variable is unset")
	}
	var b io.Reader
	if body != nil {
		data, e := json.Marshal(body)
		if e != nil {
			return e
		}
		b = bytes.NewReader(data)
	}
	req, e := http.NewRequestWithContext(ctx, method, p.endpoint+path, b)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r, e := p.client.Do(req)
	if e != nil {
		return fmt.Errorf("Hetzner HTTP request failed (details withheld)")
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("Hetzner HTTP %d", r.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(out)
	}
	return nil
}

type hetznerServer struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
	PublicNet struct {
		IPv4 struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
	ServerType struct {
		Name   string `json:"name"`
		Prices []struct {
			Location string `json:"location"`
			Monthly  struct {
				Gross string `json:"gross"`
			} `json:"price_monthly"`
			Hourly struct {
				Gross string `json:"gross"`
			} `json:"price_hourly"`
		} `json:"prices"`
	} `json:"server_type"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	Datacenter struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"datacenter"`
}

func hetznerLabels(labels map[string]string) map[string]string {
	r := map[string]string{"kit-managed": "true"}
	for k, v := range labels {
		if v != "" {
			r[strings.ReplaceAll(k, ".", "-")] = v
		}
	}
	return r
}
func (p *Hetzner) keyPath(name string) string {
	return filepath.Join(Root(), ".hetzner", p.host.Name, name+".pub")
}
func (p *Hetzner) machine(s hetznerServer) Machine {
	labels := map[string]string{}
	for k, v := range s.Labels {
		labels[strings.ReplaceAll(k, "-", ".")] = v
	}
	pub, _ := os.ReadFile(p.keyPath(s.Name))
	address := s.PublicNet.IPv4.IP
	if p.host.Mode != "run" && p.host.PrivateAddress != "" {
		address = p.host.PrivateAddress
	}
	hostkey := ""
	if len(pub) > 0 {
		hostkey = address + " " + strings.TrimSpace(string(pub))
	}
	return Machine{ID: strconv.FormatInt(s.ID, 10), Name: s.Name, State: s.Status, Labels: labels, SSH: "user@" + address, HostKey: hostkey}
}
func (p *Hetzner) List(ctx context.Context, host string) ([]Machine, error) {
	var result []Machine
	for page := 1; ; page++ {
		var r struct {
			Servers []hetznerServer `json:"servers"`
			Meta    struct {
				Pagination struct {
					NextPage *int `json:"next_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if e := p.api(ctx, "GET", "/servers?label_selector="+url.QueryEscape("kit-managed=true,hev-dispatch="+host)+"&per_page=50&page="+strconv.Itoa(page), nil, &r); e != nil {
			return nil, e
		}
		for _, s := range r.Servers {
			if s.Labels["kit-managed"] == "true" && s.Labels["hev-dispatch"] == host {
				result = append(result, p.machine(s))
			}
		}
		if r.Meta.Pagination.NextPage == nil {
			return result, nil
		}
	}
}
func (p *Hetzner) owned(ctx context.Context, id string) (hetznerServer, error) {
	if _, e := strconv.ParseInt(id, 10, 64); e != nil {
		return hetznerServer{}, fmt.Errorf("invalid Hetzner server id")
	}
	var r struct {
		Server hetznerServer `json:"server"`
	}
	e := p.api(ctx, "GET", "/servers/"+id, nil, &r)
	if e == nil && (r.Server.Labels["kit-managed"] != "true" || r.Server.Labels["hev-dispatch"] != p.host.Name) {
		e = fmt.Errorf("server is not owned by this host")
	}
	return r.Server, e
}
func (p *Hetzner) Create(ctx context.Context, s MachineSpec) (Machine, error) {
	unlock, e := p.resourceLock(ctx)
	if e != nil {
		return Machine{}, e
	}
	defer unlock()
	labels := hetznerLabels(s.Labels)
	labels["hev-dispatch"] = s.Host
	// Only persistent hosts adopt: an existing order must never be silently reused.
	if p.host.Mode != "run" {
		ms, e := p.List(ctx, s.Host)
		if e != nil {
			return Machine{}, e
		}
		if len(ms) > 1 {
			return Machine{}, fmt.Errorf("ambiguous persistent Hetzner host")
		}
		if len(ms) == 1 {
			if ms[0].HostKey == "" {
				return Machine{}, fmt.Errorf("adoption requires the original pinned host key")
			}
			return ms[0], nil
		}
	}
	public := os.Getenv(p.host.SSHPublicKeyEnv)
	if _, _, _, _, e := ssh.ParseAuthorizedKey([]byte(public)); e != nil {
		return Machine{}, fmt.Errorf("invalid SSH public key environment variable")
	}
	bootstrap, e := os.ReadFile(Expand(p.host.BootstrapFile))
	if e != nil {
		return Machine{}, e
	}
	_, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return Machine{}, e
	}
	pub, e := ssh.NewPublicKey(priv.Public())
	if e != nil {
		return Machine{}, e
	}
	block, e := ssh.MarshalPrivateKey(priv, "")
	if e != nil {
		return Machine{}, e
	}
	key := ssh.MarshalAuthorizedKey(pub)
	path := p.keyPath(s.Name)
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return Machine{}, e
	}
	// Never replace a pin on uncertain creation; operator must reconcile first.
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return Machine{}, fmt.Errorf("host key pin already exists or cannot be saved; reconcile before create")
	}
	_, e = f.Write(key)
	e2 := f.Close()
	if e != nil {
		return Machine{}, e
	}
	if e2 != nil {
		return Machine{}, e2
	}
	// Authorize the client key through cloud-init. Provider SSH key fingerprints
	// are project-unique, so importing that same operator key per host would
	// make a second host fail. No API SSH key object is needed or created.
	firewallID, e := p.ensureResource(ctx, "firewalls", map[string]any{"name": "kit-" + s.Host, "labels": hetznerLabels(map[string]string{"hev.dispatch": s.Host}), "rules": []any{map[string]any{"direction": "in", "protocol": "tcp", "port": "22", "source_ips": []string{"0.0.0.0/0", "::/0"}}}})
	if e != nil {
		return Machine{}, e
	}
	cloud := map[string]any{"users": []any{map[string]any{"name": "user", "shell": "/bin/bash", "sudo": "ALL=(ALL) NOPASSWD:ALL", "ssh_authorized_keys": []string{strings.TrimSpace(public)}}}, "ssh_keys": map[string]string{"ed25519_private": string(pem.EncodeToMemory(block)), "ed25519_public": strings.TrimSpace(string(key))}, "packages": []string{"git", "tmux", "python3", "curl", "procps"}, "write_files": []any{map[string]any{"path": "/opt/kit-bootstrap", "permissions": "0755", "encoding": "b64", "content": base64.StdEncoding.EncodeToString(bootstrap)}}, "runcmd": []any{[]string{"bash", "/opt/kit-bootstrap"}}}
	data, _ := json.Marshal(cloud)
	var r struct {
		Server hetznerServer `json:"server"`
	}
	e = p.api(ctx, "POST", "/servers", map[string]any{"name": s.Name, "server_type": s.Size, "image": s.From, "location": s.Region, "labels": labels, "firewalls": []any{map[string]int64{"firewall": firewallID}}, "user_data": "#cloud-config\n" + string(data)}, &r)
	if e != nil {
		return Machine{}, e
	}
	return p.machine(r.Server), nil
}
func (p *Hetzner) Transport(ctx context.Context, id string) (MachineTransport, error) {
	s, e := p.owned(ctx, id)
	if e != nil {
		return MachineTransport{}, e
	}
	m := p.machine(s)
	if m.HostKey == "" {
		return MachineTransport{}, fmt.Errorf("missing pinned Hetzner host key")
	}
	return MachineTransport{Kind: TransportSSH, SSH: m.SSH, HostKey: m.HostKey}, nil
}
func (p *Hetzner) Exec(ctx context.Context, id, command string) (string, error) {
	t, e := p.Transport(ctx, id)
	if e != nil {
		return "", e
	}
	return hetznerSSHExec(ctx, t, "if [ -f /run/hev-runtime.env ]; then set -a; . /run/hev-runtime.env; set +a; fi; "+command)
}
func (p *Hetzner) Copy(ctx context.Context, id, path string) ([]byte, error) {
	s, e := p.Exec(ctx, id, "cat -- "+Quote(path))
	return []byte(s), e
}
func (p *Hetzner) WaitReady(ctx context.Context, id string) error {
	for {
		s, e := p.owned(ctx, id)
		if e != nil {
			return e
		}
		if s.Status == "running" {
			if _, e = p.Exec(ctx, id, "test -f /var/lib/cloud/instance/boot-finished && sudo -n cloud-init status --wait >/dev/null && command -v hev && command -v tmux"); e == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Power-off still bills. End-of-run cleanup always deletes.
func (p *Hetzner) Stop(ctx context.Context, id string) error { return p.Delete(ctx, id) }
func (p *Hetzner) Resume(context.Context, string, time.Duration) error {
	return fmt.Errorf("deleted Hetzner runs cannot resume; use a persistent host")
}
func (p *Hetzner) Delete(ctx context.Context, id string) error {
	unlock, e := p.resourceLock(ctx)
	if e != nil {
		return e
	}
	defer unlock()
	s, e := p.owned(ctx, id)
	if e != nil {
		return e
	}
	if e = p.api(ctx, "DELETE", "/servers/"+id, nil, nil); e != nil {
		return e
	}
	for {
		var out any
		e = p.api(ctx, "GET", "/servers/"+id, nil, &out)
		if e != nil {
			if e.Error() == "Hetzner HTTP 404" {
				break
			}
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	_ = os.Remove(p.keyPath(s.Name))
	ms, e := p.List(ctx, p.host.Name)
	if e != nil {
		return e
	}
	if len(ms) == 0 {
		return p.cleanupResources(ctx, "")
	}
	return nil
}

// Cleanup also finds resources left by a failed create, using ownership labels.
func (p *Hetzner) cleanupResources(ctx context.Context, name string) error {
	for _, kind := range []string{"firewalls", "ssh_keys"} {
		var raw map[string]json.RawMessage
		if e := p.api(ctx, "GET", "/"+kind+"?label_selector="+url.QueryEscape("kit-managed=true,hev-dispatch="+p.host.Name), nil, &raw); e != nil {
			return e
		}
		var items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		if e := json.Unmarshal(raw[kind], &items); e != nil {
			return e
		}
		for _, item := range items {
			if name == "" || item.Name == name {
				if e := p.api(ctx, "DELETE", "/"+kind+"/"+strconv.FormatInt(item.ID, 10), nil, nil); e != nil {
					return e
				}
			}
		}
	}
	if name != "" {
		_ = os.Remove(p.keyPath(name))
	}
	return nil
}
func (p *Hetzner) Show(ctx context.Context) (any, error) {
	ms, e := p.List(ctx, p.host.Name)
	if e != nil {
		return nil, e
	}
	result := []hetznerServer{}
	for _, m := range ms {
		s, e := p.owned(ctx, m.ID)
		if e != nil {
			return nil, e
		}
		result = append(result, s)
	}
	return result, nil
}

var _ CloudProvider = (*Hetzner)(nil)

// Destroy requires caller confirmation. It also cleans orphan keys/firewalls
// after failed creates; never stops a machine and leaves its billing running.
func (p *Hetzner) Destroy(ctx context.Context) error {
	ms, e := p.List(ctx, p.host.Name)
	if e != nil {
		return e
	}
	for _, m := range ms {
		if e = p.Delete(ctx, m.ID); e != nil {
			return e
		}
	}
	if e = p.cleanupResources(ctx, ""); e != nil {
		return e
	}
	return os.RemoveAll(filepath.Dir(p.keyPath("unused")))
}

func (p *Hetzner) ensureResource(ctx context.Context, kind string, body map[string]any) (int64, error) {
	var raw map[string]json.RawMessage
	if e := p.api(ctx, "GET", "/"+kind+"?label_selector="+url.QueryEscape("kit-managed=true,hev-dispatch="+p.host.Name), nil, &raw); e != nil {
		return 0, e
	}
	var items []struct {
		ID int64 `json:"id"`
	}
	if e := json.Unmarshal(raw[kind], &items); e != nil {
		return 0, e
	}
	if len(items) > 1 {
		return 0, fmt.Errorf("ambiguous Hetzner resource")
	}
	if len(items) == 1 {
		return items[0].ID, nil
	}
	if e := p.api(ctx, "POST", "/"+kind, body, &raw); e != nil {
		return 0, e
	}
	key := "ssh_key"
	if kind == "firewalls" {
		key = "firewall"
	}
	var item struct {
		ID int64 `json:"id"`
	}
	if e := json.Unmarshal(raw[key], &item); e != nil {
		return 0, e
	}
	return item.ID, nil
}

// Shared key/firewall teardown cannot race a new server's admission.
func (p *Hetzner) resourceLock(ctx context.Context) (func(), error) {
	for {
		unlock, e := lockBoat(p.host.Name, "resources", "lifecycle")
		if e == nil {
			return unlock, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
