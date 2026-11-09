package nodes

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type shortHost struct {
	s    Store
	keys string // owner authorized_keys
	ek   EnrollKey
}

func newShortHost(t *testing.T) shortHost {
	t.Helper()
	keys := filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	return shortHost{Store{t.TempDir()}, keys, EnrollKey{Path: keys, Exe: "/opt/hev/bin/hev"}}
}

func (h shortHost) invite(t *testing.T, name string, allow ...string) ShortInvite {
	t.Helper()
	r, e := h.s.ShortInvite(Invite{Name: name, Address: "100.64.0.10", User: "bot", HostKey: testKey(t)}, time.Hour, allow, h.ek)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func (h shortHost) authorized(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile(h.keys)
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	return string(b)
}

func (h shortHost) enrollOpen(t *testing.T) bool {
	t.Helper()
	line, e := EnrollLine("linux", h.ek.Exe)
	if e != nil {
		t.Fatal(e)
	}
	return strings.Contains(h.authorized(t), line)
}

// pipeConn joins the member to an in-process host Enroll, optionally
// recording everything both sides send.
type pipeConn struct {
	io.Reader
	w    io.WriteCloser
	r    io.Closer
	done chan struct{}
}

func (c *pipeConn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *pipeConn) Close() error {
	_ = c.w.Close()
	<-c.done
	return c.r.Close()
}

func pipeDial(enroll func(io.Reader, io.Writer) error, hostErr *error, transcript *bytes.Buffer) Dialer {
	return func() (io.ReadWriteCloser, error) {
		mr, hw := io.Pipe()
		hr, mw := io.Pipe()
		done := make(chan struct{})
		var hostIn io.Reader = hr
		var hostOut io.Writer = hw
		if transcript != nil {
			hostIn, hostOut = io.TeeReader(hr, transcript), io.MultiWriter(hw, transcript)
		}
		go func() {
			defer close(done)
			e := enroll(hostIn, hostOut)
			if hostErr != nil {
				*hostErr = e
			}
			_ = hw.Close()
			// Drain so a member write after the host returns cannot block.
			_, _ = io.Copy(io.Discard, hr)
		}()
		return &pipeConn{Reader: mr, w: mw, r: mr, done: done}, nil
	}
}

func exchange(t *testing.T, h shortHost, code string) (Invite, error, error) {
	t.Helper()
	var hostErr error
	conn, _ := pipeDial(h.s.Enroll, &hostErr, nil)()
	i, e := ExchangeCode(conn, code, testKey(t))
	_ = conn.Close()
	return i, e, hostErr
}

func TestCodeGenerationAndAlphabet(t *testing.T) {
	if len(CodeAlphabet) != 32 || strings.ContainsAny(CodeAlphabet, "ILOU") {
		t.Fatalf("alphabet %q", CodeAlphabet)
	}
	seen := map[rune]bool{}
	for _, c := range CodeAlphabet {
		if seen[c] {
			t.Fatalf("duplicate %q", c)
		}
		seen[c] = true
	}
	h := newShortHost(t)
	format := regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{2}-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`)
	slots := map[string]bool{}
	for n := range 20 {
		r := h.invite(t, "m"+string(rune('a'+n)))
		if !format.MatchString(r.Code()) || len(r.Secret) != 8 || slots[r.Slot] {
			t.Fatalf("code %q slot reuse=%v", r.Code(), slots[r.Slot])
		}
		slots[r.Slot] = true
	}
	// 8 symbols of 5 bits each are the secret: 40 bits; the 2-symbol slot is public.
	if bits := big.NewInt(1).Lsh(big.NewInt(1), 40); new(big.Int).Exp(big.NewInt(32), big.NewInt(secretLen), nil).Cmp(bits) != 0 {
		t.Fatal("secret is not 40 bits")
	}
	for in, want := range map[string]string{
		"7k-q4m2-xpwd":   "7KQ4M2XPWD",
		" 7K Q4M2 XPWD ": "7KQ4M2XPWD",
		"7K-Q4M2-XPWO":   "7KQ4M2XPW0",
		"7K-I4L2-XPWD":   "7K1412XPWD",
	} {
		slot, secret, e := ParseCode(in)
		if e != nil || slot+secret != want {
			t.Fatalf("%q -> %q %v", in, slot+secret, e)
		}
	}
	for _, bad := range []string{"", "7K-Q4M2-XPW", "7K-Q4M2-XPWDD", "7K-Q4M2-XPWU", "7K-Q4M2-XP*D"} {
		if _, _, e := ParseCode(bad); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestGeneratorIsOnCurveAndCodeDependent(t *testing.T) {
	a, b := cpaceGenerator("7K", "Q4M2XPWD"), cpaceGenerator("7K", "Q4M2XPWE")
	if bytes.Equal(a, b) || !bytes.Equal(a, cpaceGenerator("7K", "Q4M2XPWD")) || bytes.Equal(a, cpaceGenerator("7M", "Q4M2XPWD")) {
		t.Fatal("generator must be a deterministic function of slot and secret")
	}
	for _, g := range [][]byte{a, b} {
		be := make([]byte, 32)
		for i := range g {
			be[i] = g[31-i]
		}
		x := new(big.Int).SetBytes(be)
		rhs := new(big.Int).Exp(x, big.NewInt(3), fieldP)
		rhs.Add(rhs, new(big.Int).Mul(curveJ, new(big.Int).Mul(x, x)))
		rhs.Add(rhs, x).Mod(rhs, fieldP)
		if rhs.ModSqrt(rhs, fieldP) == nil {
			t.Fatalf("generator %x is not on Curve25519", g)
		}
	}
}

// TestShortInviteJoinPendingAcceptByName is the whole owner and member flow
// in process: the enrollment key exists only while the invitation is open,
// the member pins the host key the host proved, and accept by name installs
// exactly the key the member proved, with the same forced command as today.
func TestShortInviteJoinPendingAcceptByName(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex", "repos")
	if !h.enrollOpen(t) || !strings.Contains(h.authorized(t), `restrict,command="\"/opt/hev/bin/hev\" nodes enroll" ssh-ed25519 `) {
		t.Fatalf("enrollment key not installed: %q", h.authorized(t))
	}
	if _, e := h.s.ShortInvite(Invite{Name: "alex", Address: "100.64.0.10", User: "bot", HostKey: testKey(t)}, time.Hour, nil, h.ek); e == nil {
		t.Fatal("second invitation for the same name")
	}
	if _, e := h.s.AcceptPending("alex", h.keys, h.ek.Exe, false, nil); e == nil || !strings.Contains(e.Error(), "open, not pending") {
		t.Fatalf("accepted before join: %v", e)
	}

	member := Store{t.TempDir()}
	home := t.TempDir()
	config := filepath.Join(home, ".config", "hev", "dispatch.toml")
	var hostErr error
	j, e := member.JoinShort("bot@100.64.0.10", strings.ToLower(r.Code()), home, config, pipeDial(h.s.Enroll, &hostErr, nil))
	if e != nil || hostErr != nil {
		t.Fatalf("join: %v host: %v", e, hostErr)
	}
	if j.Fingerprint != r.Fingerprint || j.Name != "hev-node-"+r.Token[:16] {
		t.Fatalf("joined %+v", j)
	}
	known, _ := os.ReadFile(filepath.Join(home, ".ssh", j.Name+".known_hosts"))
	if string(known) != "100.64.0.10 "+r.HostKey+"\n" {
		t.Fatalf("pinned %q", known)
	}
	sshConfig, _ := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if !strings.Contains(string(sshConfig), "Host "+j.Name+"\n  HostName 100.64.0.10\n  User bot\n") || !strings.Contains(string(sshConfig), "StrictHostKeyChecking yes") {
		t.Fatalf("ssh config %s", sshConfig)
	}
	pub, _ := os.ReadFile(filepath.Join(home, ".ssh", j.Name+".pub"))
	memberKey, memberFP, e := publicKey(strings.TrimSpace(string(pub)))
	if e != nil {
		t.Fatal(e)
	}
	if st, e := os.Stat(filepath.Join(home, ".ssh", j.Name)); e != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("private key %v %v", st, e)
	}

	all, _ := h.s.Invitations()
	if len(all) != 1 || all[0].State(time.Now()) != "pending" || all[0].Pending.Fingerprint != memberFP {
		t.Fatalf("pending %+v", all)
	}
	if h.enrollOpen(t) {
		t.Fatal("enrollment key left open with no open invitation")
	}
	if _, e, _ := exchange(t, h, r.Code()); e == nil || !strings.Contains(e.Error(), "already used") {
		t.Fatalf("second use: %v", e)
	}

	a, e := h.s.AcceptPending("alex", h.keys, h.ek.Exe, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	want, _ := KeyLine("linux", h.ek.Exe, r.Token, memberKey)
	if a.Key != memberKey || a.Line != want || h.authorized(t) != want+"\n" || strings.Join(a.Allow, ",") != "repos" {
		t.Fatalf("access %+v keys %q", a, h.authorized(t))
	}
	if name, e := h.s.Authenticate(a.ID, "hev dispatch rpc"); e != nil || name != "alex" {
		t.Fatalf("authenticate %q %v", name, e)
	}
	if _, e = h.s.AcceptPending("alex", h.keys, h.ek.Exe, false, nil); e == nil {
		t.Fatal("accepted twice")
	}
	if all, _ = h.s.Invitations(); len(all) != 0 {
		t.Fatalf("invitation not consumed: %+v", all)
	}
}

func TestWrongCodeLocksInvitation(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	wrong := r.Slot + "-" + strings.Repeat("Z", 8)
	if r.Secret == strings.Repeat("Z", 8) {
		wrong = r.Slot + "-" + strings.Repeat("Y", 8)
	}
	for n := 1; n <= MaxAttempts; n++ {
		_, e, hostErr := exchange(t, h, wrong)
		if e == nil || !strings.Contains(e.Error(), "could not prove") || hostErr != errNotProven {
			t.Fatalf("attempt %d: member %v host %v", n, e, hostErr)
		}
	}
	if h.enrollOpen(t) {
		t.Fatal("enrollment key still open after lockout")
	}
	if _, e, _ := exchange(t, h, r.Code()); e == nil || !strings.Contains(e.Error(), "locked") {
		t.Fatalf("right code after lockout: %v", e)
	}
	all, _ := h.s.Invitations()
	if len(all) != 1 || all[0].State(time.Now()) != "locked" {
		t.Fatalf("state %+v", all)
	}
	// Remove withdraws a locked invitation so the owner can invite again.
	if e := h.s.Remove("alex"); e != nil {
		t.Fatal(e)
	}
	h.invite(t, "alex")
}

func TestShortInviteExpiry(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	if _, e := h.s.ShortInvite(Invite{Name: "sam", Address: "100.64.0.10", User: "bot", HostKey: testKey(t)}, 25*time.Hour, nil, h.ek); e == nil {
		t.Fatal("accepted expiry over 24h")
	}
	r.Expires = time.Now().Add(-time.Second)
	if e := save(h.s.shortPath(r.Token), r); e != nil {
		t.Fatal(e)
	}
	if _, e, _ := exchange(t, h, r.Code()); e == nil || !strings.Contains(e.Error(), "expired") {
		t.Fatalf("expired exchange: %v", e)
	}
	if e := h.s.Tidy(); e != nil {
		t.Fatal(e)
	}
	if all, _ := h.s.Invitations(); len(all) != 0 || h.enrollOpen(t) {
		t.Fatalf("expired invitation or enrollment key left: %+v", all)
	}

	// A request pending past expiry cannot be accepted either.
	r = h.invite(t, "sam")
	if _, e, _ := exchange(t, h, r.Code()); e != nil {
		t.Fatal(e)
	}
	all, _ := h.s.Invitations()
	all[0].Expires = time.Now().Add(-time.Second)
	_ = save(h.s.shortPath(r.Token), all[0])
	if _, e := h.s.AcceptPending("sam", h.keys, h.ek.Exe, false, nil); e == nil || !strings.Contains(e.Error(), "expired") {
		t.Fatalf("accepted expired request: %v", e)
	}
	if strings.Contains(h.authorized(t), "dispatch rpc") {
		t.Fatal("installed a key")
	}
}

// fakeHost answers like Enroll but with a code it guessed, a host key of its
// own and no access to the real invitation. got records whether the member
// sent anything after the host's confirmation.
func fakeHost(t *testing.T, slot, guess, hostKey string, got *bool) func(io.Reader, io.Writer) error {
	return func(r io.Reader, w io.Writer) error {
		dec, enc := json.NewDecoder(r), json.NewEncoder(w)
		var h enrollHello
		if e := dec.Decode(&h); e != nil {
			return e
		}
		c, _ := newCPace(slot, guess)
		isk, e := c.key(h.Y, c.Y, true)
		if e != nil {
			return e
		}
		details, _ := json.Marshal(Invite{Version: 2, Token: strings.Repeat("ab", 32), Name: "alex", Address: "100.64.0.10", User: "bot", HostKey: hostKey, Fingerprint: "", OS: "linux", Expires: time.Now().Add(time.Hour)})
		_ = enc.Encode(enrollOffer{Y: c.Y, Invite: details, Confirm: confirm(isk, "host", details)})
		var f enrollFinish
		if dec.Decode(&f) == nil {
			*got = true
		}
		return nil
	}
}

func TestHostImpersonationWithoutCodeFails(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	attackerKey := testKey(t)
	for _, guess := range []string{"ZZZZZZZZ", r.Secret[:7] + "0", r.Secret[:7] + "1"} {
		if guess == r.Secret {
			continue
		}
		got := false
		home := t.TempDir()
		_, e := Store{t.TempDir()}.JoinShort("bot@100.64.0.10", r.Code(), home, filepath.Join(home, "dispatch.toml"), pipeDial(fakeHost(t, r.Slot, guess, attackerKey, &got), nil, nil))
		if e == nil || !strings.Contains(e.Error(), "could not prove") {
			t.Fatalf("joined an impersonated host: %v", e)
		}
		if got {
			t.Fatal("member sent its key to a host that did not prove the code")
		}
		if entries, _ := os.ReadDir(home); len(entries) != 0 {
			t.Fatalf("member wrote %v for an impersonated host", entries)
		}
	}
}

// TestTranscriptAllowsNoOfflineGuessing records complete exchanges and shows
// they are not an oracle for the code.
//
// Why offline brute force is impossible: every message is either an
// ephemeral X25519 point Y = y*G(code) with a fresh secret scalar y, or an
// HMAC under a key derived from K = y_member*y_host*G(code). To test a
// candidate code c from a transcript, an attacker must compute K for G(c),
// which needs y_member or y_host: the discrete log of a recorded Y with
// respect to G(c). Without it, the transcript is equally consistent with
// every code, so the 2^40 secret space cannot be searched offline. An active
// attacker can run the protocol with one guess, which is one online attempt
// counted against MaxAttempts (TestWrongCodeLocksInvitation). The SSH
// encryption around the exchange is not relied on.
func TestTranscriptAllowsNoOfflineGuessing(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	r2 := h.invite(t, "sam")
	var first, second bytes.Buffer
	for _, tc := range []struct {
		code string
		tr   *bytes.Buffer
	}{{r.Code(), &first}, {r2.Code(), &second}} {
		conn, _ := pipeDial(h.s.Enroll, nil, tc.tr)()
		if _, e := ExchangeCode(conn, tc.code, testKey(t)); e != nil {
			t.Fatal(e)
		}
		_ = conn.Close()
	}
	dec := json.NewDecoder(&first)
	var hello enrollHello
	var offer enrollOffer
	var finish enrollFinish
	if dec.Decode(&hello) != nil || dec.Decode(&offer) != nil || dec.Decode(&finish) != nil {
		t.Fatalf("transcript %q", first.String())
	}
	// No fixed function of the code is on the wire: neither the generator
	// nor anything derived from the code without a fresh scalar.
	g := cpaceGenerator(r.Slot, r.Secret)
	if bytes.Contains(first.Bytes(), g) || strings.Contains(first.String(), r.Secret) {
		t.Fatal("transcript contains the code or its generator")
	}
	// Even the right code does not check a recorded transcript: an observer
	// who picks its own scalar for G(code) derives a different session key,
	// so the host's confirmation cannot be recomputed for any candidate.
	for _, candidate := range []string{r.Secret, "ZZZZZZZZ"} {
		c, _ := newCPace(r.Slot, candidate)
		isk, e := c.key(hello.Y, offer.Y, false)
		if e != nil {
			t.Fatal(e)
		}
		if hmac.Equal(confirm(isk, "host", offer.Invite), offer.Confirm) {
			t.Fatalf("transcript verified candidate %q without a session secret", candidate)
		}
	}
	// A second exchange shares no message with the first.
	var hello2 enrollHello
	_ = json.NewDecoder(&second).Decode(&hello2)
	if bytes.Equal(hello.Y, hello2.Y) {
		t.Fatal("exchange messages repeat")
	}
}

func TestRemoveWithdrawsInvitationAndClosesEnrollment(t *testing.T) {
	h := newShortHost(t)
	unrelated := "ssh-ed25519 AAAA unrelated"
	_ = os.MkdirAll(filepath.Dir(h.keys), 0700)
	_ = os.WriteFile(h.keys, []byte(unrelated), 0600) // no trailing newline
	h.invite(t, "alex")
	h.invite(t, "sam")
	if e := h.s.Remove("alex"); e != nil {
		t.Fatal(e)
	}
	if !h.enrollOpen(t) {
		t.Fatal("enrollment closed while sam's invitation is open")
	}
	if e := h.s.Remove("sam"); e != nil {
		t.Fatal(e)
	}
	if got := h.authorized(t); got != unrelated {
		t.Fatalf("authorized_keys %q", got)
	}
	if e := h.s.Remove("sam"); e == nil {
		t.Fatal("removed unknown teammate")
	}
}
