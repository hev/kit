package nodes

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func testKey(t *testing.T) string {
	t.Helper()
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	k, e := ssh.NewPublicKey(pub)
	if e != nil {
		t.Fatal(e)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k)))
}
func invitation(t *testing.T, s Store) (string, Invite) {
	t.Helper()
	code, e := s.Invite(Invite{Name: "teammate", Address: "example.test", User: "bot", HostKey: testKey(t)}, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	var i Invite
	if e = Decode(code, &i); e != nil {
		t.Fatal(e)
	}
	return code, i
}
func TestCodeExpiryReplayBindingAndRevoke(t *testing.T) {
	s := Store{t.TempDir()}
	_, i := invitation(t, s)
	if e := ValidateInvite(i, i.Expires); e == nil {
		t.Fatal("accepted expired invite")
	}
	if e := ValidateInvite(i, i.Expires.Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	key := testKey(t)
	tampered := i
	tampered.Name = "other"
	code, _ := Encode(Request{tampered, key})
	if _, e := s.Accept(code, path, "/bin/hev", false); e == nil {
		t.Fatal("accepted changed name")
	}
	code, _ = Encode(Request{i, key})
	a, e := s.Accept(code, path, "/bin/hev", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Accept(code, path, "/bin/hev", false); e == nil {
		t.Fatal("accepted replay")
	}
	for _, command := range []string{"", "sh -c id", "hev dispatch rpc; id", "hev dispatch rpc --node other"} {
		if _, e = s.Authenticate(a.ID, command); e == nil {
			t.Fatalf("accepted command %q", command)
		}
	}
	if name, e := s.Authenticate(a.ID, "hev dispatch rpc"); e != nil || name != "teammate" {
		t.Fatalf("name=%q error=%v", name, e)
	}
	grants, e := s.Accesses()
	if e != nil || len(grants) != 1 || grants[0].LastUsed.IsZero() {
		t.Fatalf("grants=%+v err=%v", grants, e)
	}
	original := []byte("# unrelated\r\nssh-ed25519 AAA unrelated\n")
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, append(original, b...), 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.Remove("teammate"); e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile(path)
	if e != nil || string(b) != string(original) {
		t.Fatalf("unrelated keys changed: %q %v", b, e)
	}
	if _, e = s.Authenticate(a.ID, "hev dispatch rpc"); e == nil {
		t.Fatal("authenticated revoked key")
	}
	if runtime.GOOS != "windows" {
		st, e := os.Stat(path)
		if e != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("mode=%v err=%v", st, e)
		}
	}
}
func TestKeyLinesUnixWindowsPathsACLs(t *testing.T) {
	key := testKey(t)
	for _, tc := range []struct{ os, exe string }{{"linux", "/opt/kit tools/hev"}, {"windows", `C:\Program Files\kit\hev.exe`}} {
		line, e := KeyLine(tc.os, tc.exe, "grant", key)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(line, `restrict,command="\"`) || !strings.Contains(line, " dispatch rpc --node grant") || !strings.Contains(line, key) {
			t.Fatalf("bad line: %s", line)
		}
		if got := RemoveLine([]byte("before\r\n"+line+"\r\nafter"), line); string(got) != "before\r\nafter" {
			t.Fatalf("removal: %q", got)
		}
	}
	if p := AuthorizedPath("windows", `C:\Users\bot`, `C:\ProgramData`, true); p != `C:\ProgramData/ssh/administrators_authorized_keys` {
		t.Fatal(p)
	}
	if p := AuthorizedPath("linux", "/home/bot", "", false); p != "/home/bot/.ssh/authorized_keys" {
		t.Fatal(p)
	}
	for _, admin := range []bool{false, true} {
		args := strings.Join(ACLArgs("keys", `DOMAIN\bot`, admin), " ")
		if !strings.Contains(args, "/inheritance:r /grant:r *S-1-5-18:F") {
			t.Fatal(args)
		}
		if admin && !strings.Contains(args, "*S-1-5-32-544:F") {
			t.Fatal(args)
		}
		if !admin && !strings.Contains(args, `DOMAIN\bot:F`) {
			t.Fatal(args)
		}
	}
}
func TestJoinPinsHostAndPreservesConfig(t *testing.T) {
	s := Store{t.TempDir()}
	code, i := invitation(t, s)
	home := t.TempDir()
	config := filepath.Join(home, "dispatch.toml")
	original := "# original\n[hosts.local]\nlocal=true\ncap=2\n"
	if e := os.WriteFile(config, []byte(original), 0600); e != nil {
		t.Fatal(e)
	}
	request, e := s.Join(code, home, config)
	if e != nil {
		t.Fatal(e)
	}
	var q Request
	if e = Decode(request, &q); e != nil || q.Invite != i || q.Key == "" {
		t.Fatalf("request: %+v %v", q, e)
	}
	alias := "hev-node-" + i.Token[:16]
	known, e := os.ReadFile(filepath.Join(home, ".ssh", alias+".known_hosts"))
	if e != nil || string(known) != i.Address+" "+i.HostKey+"\n" {
		t.Fatalf("host pin: %s %v", known, e)
	}
	b, e := os.ReadFile(config)
	if e != nil || !strings.HasPrefix(string(b), original) || !strings.Contains(string(b), alias) {
		t.Fatalf("config: %s %v", b, e)
	}
	if _, e = s.Join(code, home, config); e == nil {
		t.Fatal("overwrote joined key")
	}
}

func TestRemoveRestoresMissingFinalNewline(t *testing.T) {
	s := Store{t.TempDir()}
	_, i := invitation(t, s)
	path := filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	original := []byte("# existing file without final newline")
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	code, _ := Encode(Request{i, testKey(t)})
	if _, e := s.Accept(code, path, "/bin/hev", false); e != nil {
		t.Fatal(e)
	}
	if e := s.Remove(i.Name); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(path)
	if e != nil || string(got) != string(original) {
		t.Fatalf("restored %q: %v", got, e)
	}
}
