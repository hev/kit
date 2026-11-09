package nodes

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/ssh"
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
	pub, e := s.install(i, i.Address, i.User, home, config, func(key, comment string) error {
		if out, e := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", key).CombinedOutput(); e != nil {
			return fmt.Errorf("key generation: %w: %s", e, out)
		}
		return nil
	})
	if e != nil {
		return "", e
	}
	return Encode(Request{Invite: i, Key: pub})
}

// install writes the dedicated key (via writeKey), pinned known_hosts, SSH
// alias and dispatch host for a validated invitation. The caller holds the
// lock. It returns the public key line.
func (s Store) install(i Invite, address, sshUser, home, config string, writeKey func(path, comment string) error) (string, error) {
	alias := "hev-node-" + i.Token[:16]
	sshdir := filepath.Join(home, ".ssh")
	if e := os.MkdirAll(sshdir, 0700); e != nil {
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
	if e = writeKey(key, alias); e != nil {
		return "", e
	}
	pub, e := os.ReadFile(key + ".pub")
	if e != nil {
		return "", e
	}
	known := key + ".known_hosts"
	if e = os.WriteFile(known, []byte(address+" "+i.HostKey+"\n"), 0600); e != nil {
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
	stanza := fmt.Sprintf("Host %s\n  HostName %s\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  UserKnownHostsFile %s\n  StrictHostKeyChecking yes\n  RequestTTY no\n\n", alias, address, sshUser, identity, kh)
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
	if e = save(filepath.Join(s.Dir, "joined-"+alias+".json"), Joined{alias, address, i.Fingerprint, time.Now().UTC()}); e != nil {
		return "", e
	}
	return string(pub), nil
}

// SplitTarget splits [user@]host. Neither part may contain SSH option
// syntax.
func SplitTarget(target string) (user, host string, err error) {
	user, host, ok := strings.Cut(target, "@")
	if !ok {
		user, host = "", target
	}
	if !atom(host) || strings.Contains(host, "@") || (ok && !atom(user)) {
		return "", "", fmt.Errorf("invalid host %q: expected HOST or USER@HOST", target)
	}
	return user, host, nil
}

// Dialer opens the enrollment channel to the host.
type Dialer func() (io.ReadWriteCloser, error)

// JoinShort exchanges a short code with the host at target ([user@]host) and,
// once the host has proved the code and recorded the request, installs the
// dedicated key and pinned host key like Join. The host's own address is used
// for SSH, with the invitation's account unless target names one.
func (s Store) JoinShort(target, code, home, config string, dial Dialer) (Joined, error) {
	sshUser, host, e := SplitTarget(target)
	if e != nil {
		return Joined{}, e
	}
	if _, _, e = ParseCode(code); e != nil {
		return Joined{}, e
	}
	_, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return Joined{}, e
	}
	signer, e := ssh.NewSignerFromKey(priv)
	if e != nil {
		return Joined{}, e
	}
	conn, e := dial()
	if e != nil {
		return Joined{}, e
	}
	i, e := ExchangeCode(conn, code, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))))
	closeErr := conn.Close()
	if e != nil {
		return Joined{}, e
	}
	if closeErr != nil {
		return Joined{}, closeErr
	}
	if sshUser == "" {
		sshUser = i.User
	}
	unlock, e := s.lock()
	if e != nil {
		return Joined{}, e
	}
	defer unlock()
	_, e = s.install(i, host, sshUser, home, config, func(path, comment string) error {
		b, e := ssh.MarshalPrivateKey(priv, comment)
		if e != nil {
			return e
		}
		if e = os.WriteFile(path, pem.EncodeToMemory(b), 0600); e != nil {
			return e
		}
		return os.WriteFile(path+".pub", append(bytes.TrimSpace(ssh.MarshalAuthorizedKey(signer.PublicKey())), []byte(" "+comment+"\n")...), 0600)
	})
	if e != nil {
		return Joined{}, fmt.Errorf("the host recorded your request but local setup failed: %w", e)
	}
	return Joined{"hev-node-" + i.Token[:16], host, i.Fingerprint, time.Now().UTC()}, nil
}

// EnrollTimeout bounds a whole bootstrap exchange, from starting ssh to its
// exit. The host is not authenticated until the code checks, so it must not
// be able to hold the member indefinitely.
const EnrollTimeout = 2 * time.Minute

// stderrLimit caps how much ssh error output is kept; the rest is drained.
const stderrLimit = 8 << 10

// SSHDial reaches the host's enrollment forced command with the public
// enrollment key. Host key checking is off for this one connection only: the
// connection is untrusted transport, and ExchangeCode authenticates the host
// (and the host key it pins) with the code. Because the server is not yet
// trusted, the member's own forwarding, agent, X11, local-command and
// connection-sharing settings are switched off, and the whole exchange has a
// deadline (EnrollTimeout) after which ssh is killed and reaped.
func SSHDial(target string) Dialer {
	return sshDial(target, "ssh", EnrollTimeout)
}

func sshDial(target, sshBin string, timeout time.Duration) Dialer {
	return func() (io.ReadWriteCloser, error) {
		if _, _, e := SplitTarget(target); e != nil {
			return nil, e
		}
		dir, e := os.MkdirTemp("", "hev-enroll-")
		if e != nil {
			return nil, e
		}
		known := filepath.ToSlash(filepath.Join(dir, "known_hosts"))
		if strings.ContainsAny(known, "\"\r\n") {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("unsupported temporary directory %s", dir)
		}
		key, e := EnrollPrivateKey()
		if e == nil {
			e = os.WriteFile(filepath.Join(dir, "enroll"), key, 0600)
		}
		if e != nil {
			_ = os.RemoveAll(dir)
			return nil, e
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		c := exec.CommandContext(ctx, sshBin, enrollSSHArgs(filepath.Join(dir, "enroll"), known, target)...)
		// Bound Wait even if something ssh started still holds its pipes.
		c.WaitDelay = 5 * time.Second
		stderr := &cappedBuffer{max: stderrLimit}
		c.Stderr = stderr
		fail := func(e error) (io.ReadWriteCloser, error) {
			cancel()
			_ = os.RemoveAll(dir)
			return nil, e
		}
		in, e := c.StdinPipe()
		if e != nil {
			return fail(e)
		}
		out, e := c.StdoutPipe()
		if e != nil {
			return fail(e)
		}
		if e = c.Start(); e != nil {
			return fail(e)
		}
		return &sshConn{WriteCloser: in, r: out, cmd: c, ctx: ctx, cancel: cancel, timeout: timeout, stderr: stderr, dir: dir}, nil
	}
}

func enrollSSHArgs(identity, known, target string) []string {
	return []string{"-T",
		"-i", identity,
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
		"-o", "BatchMode=yes", "-o", "PasswordAuthentication=no",
		"-o", "StrictHostKeyChecking=no",
		// Quoted: ssh splits option values on spaces (Windows profiles).
		"-o", "UserKnownHostsFile=\"" + known + "\"",
		"-o", "GlobalKnownHostsFile=\"" + known + "\"",
		// The server is unauthenticated until the code checks: give it
		// nothing from the member's own ssh config.
		"-o", "ClearAllForwardings=yes", "-o", "ForwardX11=no", "-o", "ForwardAgent=no",
		"-o", "PermitLocalCommand=no", "-o", "ControlMaster=no", "-o", "ControlPath=none",
		"-o", "ConnectTimeout=20", "-o", "LogLevel=ERROR",
		"--", target, "hev-nodes-enroll"}
}

// cappedBuffer keeps the first max bytes written and discards the rest
// while still accepting it, so ssh never blocks on a full pipe.
type cappedBuffer struct {
	mu  sync.Mutex
	b   bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.max - c.b.Len(); room > 0 {
		c.b.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}

type sshConn struct {
	io.WriteCloser
	r       io.Reader
	cmd     *exec.Cmd
	ctx     context.Context
	cancel  context.CancelFunc
	timeout time.Duration
	stderr  *cappedBuffer
	dir     string
	wait    sync.Once
	err     error
}

func (c *sshConn) finish() error {
	c.wait.Do(func() { c.err = c.cmd.Wait() })
	return c.err
}

// Read reports ssh's own error (unreachable host, enrollment key refused)
// when the channel ends without an answer, and the deadline when it fired.
func (c *sshConn) Read(p []byte) (int, error) {
	n, e := c.r.Read(p)
	if n == 0 && e != nil {
		if c.ctx.Err() != nil {
			return 0, fmt.Errorf("enrollment exchange timed out after %s", c.timeout)
		}
		if e == io.EOF && c.finish() != nil && strings.TrimSpace(c.stderr.String()) != "" {
			return 0, fmt.Errorf("ssh: %s", strings.TrimSpace(c.stderr.String()))
		}
	}
	return n, e
}

// Close ends the exchange without waiting on the server: it closes stdin,
// kills ssh and reaps it. Callers only close once the exchange is over or
// has failed, so nothing is lost by not waiting for a clean exit.
func (c *sshConn) Close() error {
	_ = c.WriteCloser.Close()
	c.cancel()
	_ = c.finish()
	return os.RemoveAll(c.dir)
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
