package nodes

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Joined struct {
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Fingerprint string    `json:"fingerprint"`
	Added       time.Time `json:"added"`
}

// Join writes dedicated SSH material. Existing dispatch configuration is preserved
// byte-for-byte; aliases and host names are deliberately not overwritten.
func (s Store) Join(code, home, config string) (string, error) {
	var i Invite
	if e := Decode(code, &i); e != nil {
		return "", e
	}
	if e := ValidateInvite(i, time.Now()); e != nil {
		return "", e
	}
	unlock, e := s.lock()
	if e != nil {
		return "", e
	}
	defer unlock()
	alias := "hev-node-" + i.Token[:16]
	sshdir := filepath.Join(home, ".ssh")
	if e = os.MkdirAll(sshdir, 0700); e != nil {
		return "", e
	}
	cfg, e := os.ReadFile(config)
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	var parsed map[string]any
	if _, e = toml.Decode(string(cfg), &parsed); e != nil {
		return "", e
	}
	if hosts, ok := parsed["hosts"].(map[string]any); ok {
		if _, exists := hosts[alias]; exists {
			return "", fmt.Errorf("host already joined")
		}
	}
	key := filepath.Join(sshdir, alias)
	if _, e = os.Stat(key); e == nil {
		return "", fmt.Errorf("join key already exists")
	}
	if out, e := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", alias, "-f", key).CombinedOutput(); e != nil {
		return "", fmt.Errorf("key generation: %w: %s", e, out)
	}
	pub, e := os.ReadFile(key + ".pub")
	if e != nil {
		return "", e
	}
	known := key + ".known_hosts"
	if e = os.WriteFile(known, []byte(i.Address+" "+i.HostKey+"\n"), 0600); e != nil {
		return "", e
	}
	sshconfig := filepath.Join(sshdir, "config")
	old, e := os.ReadFile(sshconfig)
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	// Quotes protect paths; reject characters interpreted by SSH config expansion.
	quote := func(p string) (string, error) {
		if strings.ContainsAny(p, "\"\r\n%$") {
			return "", fmt.Errorf("unsupported SSH path")
		}
		return "\"" + filepath.ToSlash(p) + "\"", nil
	}
	identity, e := quote(key)
	if e != nil {
		return "", e
	}
	kh, e := quote(known)
	if e != nil {
		return "", e
	}
	stanza := fmt.Sprintf("Host %s\n  HostName %s\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  UserKnownHostsFile %s\n  StrictHostKeyChecking yes\n  RequestTTY no\n\n", alias, i.Address, i.User, identity, kh)
	// Put the specific alias first: OpenSSH takes the first value encountered.
	if e = os.WriteFile(sshconfig, append([]byte(stanza), old...), 0600); e != nil {
		return "", e
	}
	var entry bytes.Buffer
	entry.WriteString("\n")
	if e = toml.NewEncoder(&entry).Encode(map[string]any{"hosts": map[string]any{alias: map[string]any{"ssh": alias, "os": i.OS, "cap": 1}}}); e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(config), 0700); e != nil {
		return "", e
	}
	if e = os.WriteFile(config, append(cfg, entry.Bytes()...), 0600); e != nil {
		return "", e
	}
	if e = save(filepath.Join(s.Dir, "joined-"+alias+".json"), Joined{alias, i.Address, i.Fingerprint, time.Now().UTC()}); e != nil {
		return "", e
	}
	return Encode(Request{Invite: i, Key: string(pub)})
}
func (s Store) Joined() ([]Joined, error) {
	entries, e := os.ReadDir(s.Dir)
	if os.IsNotExist(e) {
		return []Joined{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Joined{}
	for _, f := range entries {
		if strings.HasPrefix(f.Name(), "joined-") {
			var j Joined
			if e = read(filepath.Join(s.Dir, f.Name()), &j); e != nil {
				return nil, e
			}
			out = append(out, j)
		}
	}
	return out, nil
}
