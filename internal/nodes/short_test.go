package nodes

import (
	"bytes"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

// Why a recorded transcript gives no offline test for the code (reasoning,
// not something these tests prove): every message is either an ephemeral
// X25519 share Y = y*G(code) with a fresh secret scalar y, or an HMAC under a
// key derived from K = y_member*y_host*G(code). To check a candidate code c
// against a transcript an attacker must compute K for G(c), which needs
// y_member or y_host, the discrete log of a recorded Y with respect to G(c).
// Under the usual Diffie-Hellman assumptions that is infeasible, so the
// transcript is consistent with every code and the 2^40 secret space cannot
// be searched offline. An active attacker can run the protocol with one
// guess, which is one online attempt counted against MaxAttempts
// (TestWrongCodeLocksInvitation). The SSH encryption around the exchange is
// not relied on.

// TestTranscriptHasNoStaticCodeValue is a sanity check on recorded
// exchanges, not a proof of the argument above. It asserts that no decoded
// field carries the secret or the generator G(code), that recomputing the
// session from the transcript with a fresh scalar does not reproduce the
// host's MAC (even for the right code), and that shares are fresh per
// exchange.
func TestTranscriptHasNoStaticCodeValue(t *testing.T) {
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
	var done enrollDone
	if dec.Decode(&hello) != nil || dec.Decode(&offer) != nil || dec.Decode(&finish) != nil || dec.Decode(&done) != nil || !done.OK {
		t.Fatalf("transcript %q", first.String())
	}
	g := cpaceGenerator(r.Slot, r.Secret)
	for name, field := range map[string][]byte{"hello.y": hello.Y, "offer.y": offer.Y, "offer.confirm": offer.Confirm, "finish.confirm": finish.Confirm} {
		if bytes.Contains(field, g) || bytes.Contains(field, []byte(r.Secret)) {
			t.Fatalf("%s carries the generator or secret", name)
		}
	}
	var details Invite
	if e := json.Unmarshal(offer.Invite, &details); e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{details.Token, details.Name, details.Address, details.User, details.HostKey, details.Fingerprint, finish.Key} {
		if strings.Contains(v, r.Secret) {
			t.Fatalf("decoded field %q carries the secret", v)
		}
	}
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
	var hello2 enrollHello
	var offer2 enrollOffer
	dec2 := json.NewDecoder(&second)
	if dec2.Decode(&hello2) != nil || dec2.Decode(&offer2) != nil {
		t.Fatal("second transcript")
	}
	if bytes.Equal(hello.Y, hello2.Y) || bytes.Equal(offer.Y, offer2.Y) {
		t.Fatal("exchange shares repeat")
	}
}

// lowOrderPoints are the Curve25519 u-coordinates of small order, including
// non-canonical encodings (libsodium's blocklist).
var lowOrderPoints = []string{
	"0000000000000000000000000000000000000000000000000000000000000000",
	"0100000000000000000000000000000000000000000000000000000000000000",
	"e0eb7a7c3b41b8ae1656e3faf19fc46ada098deb9c32b1fd866205165f49b800",
	"5f9c95bca3508c24b1d0b1559c83ef5b04445cc4581c8e86d8224eddd09f1157",
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
	"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
	"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
}

// scripted runs the host's Enroll on fixed input and returns its output.
func scripted(t *testing.T, s Store, msgs ...any) ([]byte, error) {
	t.Helper()
	var in, out bytes.Buffer
	enc := json.NewEncoder(&in)
	for _, m := range msgs {
		if e := enc.Encode(m); e != nil {
			t.Fatal(e)
		}
	}
	e := s.Enroll(&in, &out)
	return out.Bytes(), e
}

func TestLowOrderSharesRejected(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	member, _ := newCPace(r.Slot, r.Secret)
	for _, p := range lowOrderPoints {
		y, _ := hex.DecodeString(p)
		// The member refuses a low-order host share.
		if _, e := member.key(member.Y, y, false); e == nil {
			t.Fatalf("member accepted low-order host share %s", p)
		}
		// The host refuses a low-order member share and sends no share or
		// confirmation of its own.
		host, _ := newCPace(r.Slot, r.Secret)
		if _, e := host.key(y, host.Y, true); e == nil {
			t.Fatalf("host accepted low-order member share %s", p)
		}
	}
	h2 := newShortHost(t)
	r2 := h2.invite(t, "alex")
	y, _ := hex.DecodeString(lowOrderPoints[2])
	out, e := scripted(t, h2.s, enrollHello{V: 1, Slot: r2.Slot, Y: y})
	var o enrollOffer
	_ = json.Unmarshal(out, &o)
	if e == nil || o.Error == "" || o.Y != nil || o.Confirm != nil || o.Invite != nil {
		t.Fatalf("host answered a low-order share: %s %v", out, e)
	}
}

func TestReflectionRejected(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	// Share reflection: a party that echoes the peer's own share is refused
	// on both sides.
	c, _ := newCPace(r.Slot, r.Secret)
	if _, e := c.key(c.Y, c.Y, false); e == nil {
		t.Fatal("member accepted its own share back")
	}
	if _, e := c.key(c.Y, c.Y, true); e == nil {
		t.Fatal("host accepted its own share back")
	}
	var got bool
	echo := func(rd io.Reader, w io.Writer) error {
		var hello enrollHello
		dec := json.NewDecoder(rd)
		if e := dec.Decode(&hello); e != nil {
			return e
		}
		_ = json.NewEncoder(w).Encode(enrollOffer{Y: hello.Y, Invite: []byte(`{}`), Confirm: make([]byte, 32)})
		var f enrollFinish
		got = dec.Decode(&f) == nil
		return nil
	}
	conn, _ := pipeDial(echo, nil, nil)()
	if _, e := ExchangeCode(conn, r.Code(), testKey(t)); e == nil || got {
		t.Fatalf("member continued with a reflected share: %v sent=%v", e, got)
	}
	_ = conn.Close()

	// Message reflection: without the code, an attacker cannot turn the
	// host's own confirmation into the member's; the role labels differ.
	attacker, _ := newCPace(r.Slot, "ZZZZZZZZ")
	pr, pw := io.Pipe()
	out := &syncWriter{}
	hostDone := make(chan error, 1)
	go func() { hostDone <- h.s.Enroll(pr, out) }()
	_ = json.NewEncoder(pw).Encode(enrollHello{V: 1, Slot: r.Slot, Y: attacker.Y})
	var offer enrollOffer
	waitFor(t, func() bool { return json.Unmarshal(out.Bytes(), &offer) == nil })
	_ = json.NewEncoder(pw).Encode(enrollFinish{Key: testKey(t), Confirm: offer.Confirm})
	_ = pw.Close()
	if e := <-hostDone; e != errNotProven {
		t.Fatalf("host accepted its own confirmation reflected: %v", e)
	}
	all, _ := h.s.Invitations()
	if len(all) != 1 || all[0].Pending != nil || all[0].Failures != 1 {
		t.Fatalf("record after reflection %+v", all)
	}
}

// syncWriter lets a test poll what Enroll wrote so far.
type syncWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncWriter) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.b.Bytes())
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 500 {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestReplayRejected(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	fresh, e := os.ReadFile(h.s.shortPath(r.Token))
	if e != nil {
		t.Fatal(e)
	}
	var tr bytes.Buffer
	conn, _ := pipeDial(h.s.Enroll, nil, &tr)()
	if _, e = ExchangeCode(conn, r.Code(), testKey(t)); e != nil {
		t.Fatal(e)
	}
	_ = conn.Close()
	dec := json.NewDecoder(&tr)
	var hello enrollHello
	var offer enrollOffer
	var finish enrollFinish
	if dec.Decode(&hello) != nil || dec.Decode(&offer) != nil || dec.Decode(&finish) != nil {
		t.Fatal("transcript")
	}

	// The same host: the invitation is single use.
	if _, e = scripted(t, h.s, hello, finish); e == nil || !strings.Contains(e.Error(), "already used") {
		t.Fatalf("replay to used invitation: %v", e)
	}

	// A host that still has the invitation open (as if the first run never
	// happened) answers with a fresh share, so the recorded member
	// confirmation no longer matches.
	h2 := newShortHost(t)
	_ = os.MkdirAll(h2.s.Dir, 0700)
	if e = os.WriteFile(h2.s.shortPath(r.Token), fresh, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = scripted(t, h2.s, hello, finish); e != errNotProven {
		t.Fatalf("replayed member messages: %v", e)
	}
	all, _ := h2.s.Invitations()
	if len(all) != 1 || all[0].Pending != nil || all[0].Failures != 1 {
		t.Fatalf("replay recorded %+v", all)
	}

	// A member with the right code gets the recorded host offer replayed: its
	// fresh share makes the old confirmation fail, and it sends nothing.
	var got bool
	replay := func(rd io.Reader, w io.Writer) error {
		d := json.NewDecoder(rd)
		var hh enrollHello
		if e := d.Decode(&hh); e != nil {
			return e
		}
		_ = json.NewEncoder(w).Encode(offer)
		var f enrollFinish
		got = d.Decode(&f) == nil
		return nil
	}
	conn, _ = pipeDial(replay, nil, nil)()
	if _, e = ExchangeCode(conn, r.Code(), testKey(t)); e == nil || !strings.Contains(e.Error(), "could not prove") || got {
		t.Fatalf("member accepted a replayed offer: %v sent=%v", e, got)
	}
	_ = conn.Close()
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
