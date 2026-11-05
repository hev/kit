// Package nodes manages paste-based SSH invitations using host-owned plain files.
package nodes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Invite struct {
	Version     int       `json:"version"`
	Token       string    `json:"token"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	User        string    `json:"user"`
	HostKey     string    `json:"host_key"`
	Fingerprint string    `json:"fingerprint"`
	OS          string    `json:"os"`
	Expires     time.Time `json:"expires"`
}
type Request struct {
	Invite Invite `json:"invite"`
	Key    string `json:"key"`
}
type Access struct {
	SeparatorPrefix string    `json:"separator_prefix,omitempty"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Key             string    `json:"key"`
	Path            string    `json:"path"`
	Line            string    `json:"line"`
	Added           time.Time `json:"added"`
	LastUsed        time.Time `json:"last_used"`
}
type Store struct{ Dir string }

func Default() Store {
	h, _ := os.UserHomeDir()
	return Store{filepath.Join(h, ".config", "hev", "nodes")}
}
func ValidName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func atom(s string) bool {
	return s != "" && !strings.ContainsAny(s, " \t\r\n\x00\"'\\;") && !strings.HasPrefix(s, "-")
}
func Encode(v any) (string, error) {
	b, e := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func Decode(code string, v any) error {
	if len(code) > 32768 {
		return fmt.Errorf("code too large")
	}
	b, e := base64.RawURLEncoding.DecodeString(code)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func publicKey(s string) (string, string, error) {
	k, _, opts, rest, e := ssh.ParseAuthorizedKey([]byte(s))
	if e != nil {
		return "", "", e
	}
	if len(opts) != 0 || len(strings.TrimSpace(string(rest))) != 0 {
		return "", "", fmt.Errorf("expected one public key without options")
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k))), ssh.FingerprintSHA256(k), nil
}
func (s Store) lock() (func(), error) {
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return nil, e
	}
	p := filepath.Join(s.Dir, "lock")
	if e := os.Mkdir(p, 0700); e != nil {
		return nil, fmt.Errorf("nodes busy (if an interrupted command left %s, remove it after checking no nodes command is running): %w", p, e)
	}
	return func() { _ = os.Remove(p) }, nil
}
func save(p string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}
func read(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s Store) Invite(i Invite, ttl time.Duration) (string, error) {
	if !ValidName(i.Name) || !atom(i.Address) || !atom(i.User) || strings.Contains(i.Address, "@") {
		return "", fmt.Errorf("invalid name, address or SSH user")
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		return "", fmt.Errorf("expiry must be between zero and 24h")
	}
	k, fp, e := publicKey(i.HostKey)
	if e != nil {
		return "", e
	}
	i.HostKey = k
	i.Fingerprint = fp
	i.Version = 1
	i.OS = runtime.GOOS
	i.Expires = time.Now().Add(ttl)
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return "", e
	}
	i.Token = hex.EncodeToString(b)
	unlock, e := s.lock()
	if e != nil {
		return "", e
	}
	defer unlock()
	if e = save(filepath.Join(s.Dir, "invite-"+i.Token+".json"), i); e != nil {
		return "", e
	}
	return Encode(i)
}
func ValidateInvite(i Invite, now time.Time) error {
	if i.Version != 1 || len(i.Token) != 64 || !ValidName(i.Name) || !atom(i.Address) || !atom(i.User) || strings.Contains(i.Address, "@") {
		return fmt.Errorf("invalid invitation")
	}
	if _, e := hex.DecodeString(i.Token); e != nil {
		return e
	}
	if !now.Before(i.Expires) {
		return fmt.Errorf("invitation expired")
	}
	k, fp, e := publicKey(i.HostKey)
	if e != nil {
		return e
	}
	if k != i.HostKey || fp != i.Fingerprint {
		return fmt.Errorf("host fingerprint mismatch")
	}
	if i.OS != "windows" && i.OS != "linux" && i.OS != "darwin" {
		return fmt.Errorf("unsupported host OS")
	}
	return nil
}
func AuthorizedPath(goos, home, programData string, admin bool) string {
	if goos == "windows" && admin {
		return strings.TrimRight(programData, "/\\") + "/ssh/administrators_authorized_keys"
	}
	return filepath.Join(home, ".ssh", "authorized_keys")
}

// ACLArgs uses well-known SIDs so localized Windows installations work.
func ACLArgs(path, username string, admin bool) []string {
	grants := []string{"*S-1-5-18:F"}
	if admin {
		grants = append(grants, "*S-1-5-32-544:F")
	} else {
		grants = append(grants, username+":F")
	}
	a := []string{path, "/inheritance:r", "/grant:r"}
	return append(a, grants...)
}
func KeyLine(goos, exe, id, key string) (string, error) {
	if !ValidName(id) || strings.ContainsAny(exe, "\r\n\x00\"'`$%!&|<>^") {
		return "", fmt.Errorf("unsafe forced command path or ID")
	}
	k, _, e := publicKey(key)
	if e != nil {
		return "", e
	}
	if goos == "windows" {
		exe = strings.ReplaceAll(exe, "\\", "/")
	}
	command := "\"" + exe + "\" dispatch rpc --node " + id
	// authorized_keys escapes double quotes and backslashes inside command="".
	command = strings.ReplaceAll(strings.ReplaceAll(command, "\\", "\\\\"), "\"", "\\\"")
	return "restrict,command=\"" + command + "\" " + k + " hev-node-" + id, nil
}
func RemoveLine(b []byte, line string) []byte {
	parts := strings.SplitAfter(string(b), "\n")
	out := ""
	for _, p := range parts {
		if strings.TrimSuffix(strings.TrimSuffix(p, "\n"), "\r") != line {
			out += p
		}
	}
	return []byte(out)
}
func secure(path string, admin bool) error {
	if runtime.GOOS == "windows" {
		u := os.Getenv("USERDOMAIN") + "\\" + os.Getenv("USERNAME")
		out, e := exec.Command("icacls", path, "/reset").CombinedOutput()
		if e != nil {
			return fmt.Errorf("reset authorized keys ACL: %w: %s", e, out)
		}
		out, e = exec.Command("icacls", ACLArgs(path, u, admin)...).CombinedOutput()
		if e != nil {
			return fmt.Errorf("secure authorized keys: %w: %s", e, out)
		}
		return nil
	}
	if e := os.Chmod(filepath.Dir(path), 0700); e != nil {
		return e
	}
	return os.Chmod(path, 0600)
}
func (s Store) Accesses() ([]Access, error) {
	entries, e := os.ReadDir(s.Dir)
	if os.IsNotExist(e) {
		return []Access{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Access{}
	for _, f := range entries {
		if strings.HasPrefix(f.Name(), "access-") && strings.HasSuffix(f.Name(), ".json") {
			var a Access
			if e = read(filepath.Join(s.Dir, f.Name()), &a); e != nil {
				return nil, e
			}
			out = append(out, a)
		}
	}
	return out, nil
}
func (s Store) Accept(code, path, exe string, admin bool) (Access, error) {
	var q Request
	var a Access
	if e := Decode(code, &q); e != nil {
		return a, e
	}
	if e := ValidateInvite(q.Invite, time.Now()); e != nil {
		return a, e
	}
	key, _, e := publicKey(q.Key)
	if e != nil {
		return a, e
	}
	unlock, e := s.lock()
	if e != nil {
		return a, e
	}
	defer unlock()
	var i Invite
	ip := filepath.Join(s.Dir, "invite-"+q.Invite.Token+".json")
	if e = read(ip, &i); e != nil {
		return a, fmt.Errorf("invitation missing or already consumed: %w", e)
	}
	if i != q.Invite {
		return a, fmt.Errorf("invitation differs from host record")
	}
	grants, e := s.Accesses()
	if e != nil {
		return a, e
	}
	for _, g := range grants {
		if g.Name == i.Name || g.Key == key {
			return a, fmt.Errorf("name or key already granted; remove first")
		}
	}
	line, e := KeyLine(runtime.GOOS, exe, i.Token, key)
	if e != nil {
		return a, e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return a, e
	}
	b, e := os.ReadFile(path)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return a, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return a, e
	}
	if e = f.Close(); e != nil {
		return a, e
	}
	if e = secure(path, admin); e != nil {
		return a, e
	}
	// Consume before granting; failures cannot accidentally replay a request.
	if e = os.Remove(ip); e != nil {
		return a, e
	}
	a = Access{ID: i.Token, Name: i.Name, Key: key, Path: path, Line: line, Added: time.Now().UTC()}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		a.SeparatorPrefix = fmt.Sprintf("%x", sha256.Sum256(b))
		b = append(b, '\n')
	}
	b = append(b, []byte(line+"\n")...)
	if e = os.WriteFile(path, b, 0600); e != nil {
		return a, e
	}
	if e = save(filepath.Join(s.Dir, "access-"+a.ID+".json"), a); e != nil {
		_ = os.WriteFile(path, RemoveLine(b, line), 0600)
		return a, e
	}
	return a, nil
}
func (s Store) Authenticate(id, original string) (string, error) {
	if !ValidName(id) || original != "hev dispatch rpc" {
		return "", fmt.Errorf("dispatch RPC only")
	}
	unlock, e := s.lock()
	if e != nil {
		return "", e
	}
	defer unlock()
	var a Access
	p := filepath.Join(s.Dir, "access-"+id+".json")
	if e = read(p, &a); e != nil {
		return "", fmt.Errorf("access revoked or unknown")
	}
	a.LastUsed = time.Now().UTC()
	if e = save(p, a); e != nil {
		return "", e
	}
	return a.Name, nil
}
func (s Store) Remove(name string) error {
	unlock, e := s.lock()
	if e != nil {
		return e
	}
	defer unlock()
	grants, e := s.Accesses()
	if e != nil {
		return e
	}
	for _, a := range grants {
		if a.Name != name {
			continue
		}
		if e = os.Remove(filepath.Join(s.Dir, "access-"+a.ID+".json")); e != nil {
			return e
		}
		b, e := os.ReadFile(a.Path)
		if e != nil {
			return e
		}
		clean := RemoveLine(b, a.Line)
		if a.SeparatorPrefix != "" && len(clean) > 0 && clean[len(clean)-1] == '\n' && fmt.Sprintf("%x", sha256.Sum256(clean[:len(clean)-1])) == a.SeparatorPrefix {
			clean = clean[:len(clean)-1]
		}
		return os.WriteFile(a.Path, clean, 0600)
	}
	return fmt.Errorf("unknown teammate %s", name)
}
