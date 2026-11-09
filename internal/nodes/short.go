package nodes

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Short invitations replace the pasted code and pasted-back request with a
// short code a person can read aloud. The member reaches the host over its
// existing sshd with a well-known enrollment key whose forced command runs
// only `hev nodes enroll`; that key line exists only while a short invitation
// is open. Both sides then run a PAKE keyed by the code (cpace.go): the host
// proves it knows the code before the member trusts the host key it reports,
// and the member proves it before the host records its public key as a
// pending request. The owner approves the request by name.

// CodeAlphabet is Crockford base32: no I, L, O or U.
const CodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const (
	slotLen   = 2 // public selector among open invitations
	secretLen = 8 // 40-bit shared secret
	// MaxAttempts is how many exchanges an invitation allows before it locks.
	// Every exchange counts until it proves the code, so aborting early does
	// not avoid the count.
	MaxAttempts = 5
)

// ShortInvite is the host record of a short invitation. Invite carries the
// details the member pins once the host proves the code (Version 2; Token is
// the access ID, not a secret).
type ShortInvite struct {
	Invite
	Slot     string   `json:"slot"`
	Secret   string   `json:"secret"`
	Failures int      `json:"failures,omitempty"`
	Allow    []string `json:"allow,omitempty"`
	Pending  *Pending `json:"pending,omitempty"`
	// Unreadable is the path of a record that did not parse. It matches no
	// code and is never tidied, so it fails closed until the owner removes it.
	Unreadable string `json:"-"`
}

// Pending is a member's public key bound to the invitation it proved.
type Pending struct {
	Key         string    `json:"key"`
	Fingerprint string    `json:"fingerprint"`
	Requested   time.Time `json:"requested"`
}

// State is open, locked, pending, expired or unreadable.
func (r ShortInvite) State(now time.Time) string {
	switch {
	case r.Unreadable != "":
		return "unreadable"
	case !now.Before(r.Expires):
		return "expired"
	case r.Pending != nil:
		return "pending"
	case r.Failures >= MaxAttempts:
		return "locked"
	}
	return "open"
}

// Code formats the invitation code, e.g. 7K-Q4M2-XPWD.
func (r ShortInvite) Code() string {
	return r.Slot + "-" + r.Secret[:4] + "-" + r.Secret[4:]
}

// EnrollKey says where the owner's authorized_keys file is and which hev
// executable the enrollment forced command runs.
type EnrollKey struct {
	Path  string
	Exe   string
	Admin bool
}

type enrollRecord struct {
	Path            string `json:"path"`
	Line            string `json:"line"`
	Admin           bool   `json:"admin,omitempty"`
	SeparatorPrefix string `json:"separator_prefix,omitempty"`
}

func randomSymbols(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = CodeAlphabet[int(v)%len(CodeAlphabet)] // 256 is a multiple of 32: unbiased
	}
	return string(out), nil
}

// ParseCode accepts a code with any case, dashes or spaces, reading I and L
// as 1 and O as 0.
func ParseCode(code string) (slot, secret string, err error) {
	s := strings.NewReplacer("-", "", " ", "", "I", "1", "L", "1", "O", "0").Replace(strings.ToUpper(strings.TrimSpace(code)))
	if len(s) != slotLen+secretLen || strings.Trim(s, CodeAlphabet) != "" {
		return "", "", fmt.Errorf("invalid invite code: expected %d letters and digits like 7K-Q4M2-XPWD", slotLen+secretLen)
	}
	return s[:slotLen], s[slotLen:], nil
}

// enrollSigner is the enrollment client key. It is public by design: every
// hev build derives the same key, so possessing it proves nothing. sshd only
// lets it run `hev nodes enroll`, which grants nothing without the code.
func enrollSigner() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("hev nodes enroll v1: this private key is intentionally public"))
	return ed25519.NewKeyFromSeed(seed[:])
}

// EnrollPrivateKey returns the public enrollment key in OpenSSH PEM form.
func EnrollPrivateKey() ([]byte, error) {
	b, e := ssh.MarshalPrivateKey(enrollSigner(), "hev-nodes-enroll")
	if e != nil {
		return nil, e
	}
	return pem.EncodeToMemory(b), nil
}

// EnrollLine is the authorized_keys line installed while an invitation is open.
func EnrollLine(goos, exe string) (string, error) {
	if strings.ContainsAny(exe, "\r\n\x00\"'`$%!&|<>^") {
		return "", fmt.Errorf("unsafe forced command path")
	}
	pub, e := ssh.NewPublicKey(enrollSigner().Public())
	if e != nil {
		return "", e
	}
	key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	return forcedLine(goos, exe, "nodes enroll", key, "hev-nodes-enroll"), nil
}

func (s Store) shortPath(token string) string {
	return filepath.Join(s.Dir, "short-"+token+".json")
}

// Invitations lists short invitation records, including expired ones not yet
// tidied. A record that does not parse is listed as unreadable rather than
// failing the listing, so one damaged invitation cannot block the others.
func (s Store) Invitations() ([]ShortInvite, error) {
	entries, e := os.ReadDir(s.Dir)
	if os.IsNotExist(e) {
		return []ShortInvite{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []ShortInvite{}
	for _, f := range entries {
		if strings.HasPrefix(f.Name(), "short-") && strings.HasSuffix(f.Name(), ".json") {
			var r ShortInvite
			p := filepath.Join(s.Dir, f.Name())
			if e = read(p, &r); errors.Is(e, os.ErrNotExist) {
				continue // removed since the directory was read
			} else if e != nil {
				r = ShortInvite{Unreadable: p}
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// ShortInvite records a short invitation, opens the enrollment key if needed
// and returns the record (its Code is the shared secret).
func (s Store) ShortInvite(i Invite, ttl time.Duration, allow []string, ek EnrollKey) (ShortInvite, error) {
	var r ShortInvite
	if !ValidName(i.Name) || !atom(i.Address) || !atom(i.User) || strings.Contains(i.Address, "@") {
		return r, fmt.Errorf("invalid name, address or SSH user")
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		return r, fmt.Errorf("expiry must be between zero and 24h")
	}
	allow, e := validCapabilities(allow)
	if e != nil {
		return r, e
	}
	k, fp, e := publicKey(i.HostKey)
	if e != nil {
		return r, e
	}
	i.HostKey, i.Fingerprint, i.Version, i.OS = k, fp, 2, runtime.GOOS
	i.Expires = time.Now().Add(ttl).UTC().Truncate(time.Second)
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return r, e
	}
	i.Token = hex.EncodeToString(b)
	r = ShortInvite{Invite: i}
	if len(allow) > 0 {
		r.Allow = allow
	}
	if r.Secret, e = randomSymbols(secretLen); e != nil {
		return r, e
	}
	unlock, e := s.lock()
	if e != nil {
		return r, e
	}
	defer unlock()
	if e = s.tidyLocked(); e != nil {
		return r, e
	}
	grants, e := s.Accesses()
	if e != nil {
		return r, e
	}
	for _, g := range grants {
		if g.Name == i.Name {
			return r, fmt.Errorf("%s already has access; remove first", i.Name)
		}
	}
	open, e := s.Invitations()
	if e != nil {
		return r, e
	}
	used := map[string]bool{}
	for _, o := range open {
		if o.Name == i.Name {
			return r, fmt.Errorf("%s already has an invitation (%s); remove it first", i.Name, o.State(time.Now()))
		}
		used[o.Slot] = true
	}
	for tries := 0; r.Slot == "" || used[r.Slot]; tries++ {
		if tries > 64 {
			return r, fmt.Errorf("too many open invitations")
		}
		if r.Slot, e = randomSymbols(slotLen); e != nil {
			return r, e
		}
	}
	if e = save(s.shortPath(i.Token), r); e != nil {
		return r, e
	}
	if e = s.openEnroll(ek); e != nil {
		_ = os.Remove(s.shortPath(i.Token))
		return r, e
	}
	return r, nil
}

func (s Store) openEnroll(ek EnrollKey) error {
	var rec enrollRecord
	p := filepath.Join(s.Dir, "enroll.json")
	if e := read(p, &rec); e == nil {
		if rec.Path != ek.Path {
			return fmt.Errorf("enrollment key is open in %s; accept or remove open invitations first", rec.Path)
		}
		return nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return fmt.Errorf("enrollment record %s is unreadable (%v); remove the hev-nodes-enroll line from authorized_keys and that file", p, e)
	}
	line, e := EnrollLine(runtime.GOOS, ek.Exe)
	if e != nil {
		return e
	}
	rec = enrollRecord{Path: ek.Path, Line: line, Admin: ek.Admin}
	// Record first, with the separator: a crash after appending still leaves
	// a record that closes the line and restores the file.
	if _, e = appendLine(ek.Path, ek.Admin, line, func(sep string) error {
		rec.SeparatorPrefix = sep
		return save(p, rec)
	}); e != nil {
		_ = os.Remove(p)
		return e
	}
	return nil
}

// tidyLocked deletes expired invitations and closes the enrollment key when
// no invitation is open. The caller holds the lock.
func (s Store) tidyLocked() error {
	all, e := s.Invitations()
	if e != nil {
		return e
	}
	now := time.Now()
	open := false
	for _, r := range all {
		switch r.State(now) {
		case "expired":
			if e = os.Remove(s.shortPath(r.Token)); e != nil {
				return e
			}
		case "open":
			open = true
		}
	}
	if open {
		return nil
	}
	var rec enrollRecord
	p := filepath.Join(s.Dir, "enroll.json")
	if read(p, &rec) != nil {
		return nil
	}
	if e = s.removeOwned(rec.Path, rec.Line, rec.SeparatorPrefix); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return os.Remove(p)
}

// Tidy removes expired invitations and closes the enrollment key when nothing
// is open.
func (s Store) Tidy() error {
	unlock, e := s.lock()
	if e != nil {
		return e
	}
	defer unlock()
	return s.tidyLocked()
}

// lockRetry waits briefly for the lock; enrollment runs unattended.
func (s Store) lockRetry() (func(), error) {
	var e error
	for range 50 {
		var unlock func()
		if unlock, e = s.lock(); e == nil {
			return unlock, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, e
}

// AcceptPending grants the key a member proved for the named invitation,
// with the invitation's capabilities plus allow.
func (s Store) AcceptPending(name, path, exe string, admin bool, allow []string) (Access, error) {
	unlock, e := s.lock()
	if e != nil {
		return Access{}, e
	}
	defer unlock()
	all, e := s.Invitations()
	if e != nil {
		return Access{}, e
	}
	for _, r := range all {
		if r.Name != name {
			continue
		}
		if st := r.State(time.Now()); st != "pending" {
			return Access{}, fmt.Errorf("invitation for %s is %s, not pending", name, st)
		}
		caps, e := validCapabilities(append(slices.Clone(r.Allow), allow...))
		if e != nil {
			return Access{}, e
		}
		key, _, e := publicKey(r.Pending.Key)
		if e != nil {
			return Access{}, e
		}
		a, e := s.grant(r.Invite, key, path, exe, admin, caps, s.shortPath(r.Token))
		if e != nil {
			return a, e
		}
		return a, s.tidyLocked()
	}
	return Access{}, fmt.Errorf("no invitation for %s", name)
}

// removeInvitations deletes every short invitation for name. The caller holds
// the lock.
func (s Store) removeInvitations(name string) (bool, error) {
	all, e := s.Invitations()
	if e != nil {
		return false, e
	}
	found := false
	for _, r := range all {
		if r.Name == name {
			if e = os.Remove(s.shortPath(r.Token)); e != nil {
				return found, e
			}
			found = true
		}
	}
	if found {
		return true, s.tidyLocked()
	}
	return false, nil
}

// Exchange messages, one JSON object per line.
type enrollHello struct {
	V    int    `json:"v"`
	Slot string `json:"slot"`
	Y    []byte `json:"y"`
}
type enrollOffer struct {
	Error   string          `json:"error,omitempty"`
	Y       []byte          `json:"y,omitempty"`
	Invite  json.RawMessage `json:"invite,omitempty"`
	Confirm []byte          `json:"confirm,omitempty"`
}
type enrollFinish struct {
	Key     string `json:"key"`
	Confirm []byte `json:"confirm"`
}
type enrollDone struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

var errNotProven = errors.New("the code did not match")

// Enroll is the host side of a short-code exchange, run as the enrollment
// key's forced command. On success the member's key is recorded as pending.
func (s Store) Enroll(r io.Reader, w io.Writer) error {
	dec := json.NewDecoder(io.LimitReader(r, 64<<10))
	enc := json.NewEncoder(w)
	refuse := func(msg string) error {
		_ = enc.Encode(enrollOffer{Error: msg})
		return errors.New(msg)
	}
	var h enrollHello
	if e := dec.Decode(&h); e != nil || h.V != 1 || len(h.Y) != 32 || len(h.Slot) != slotLen || strings.Trim(h.Slot, CodeAlphabet) != "" {
		return refuse("malformed enrollment request")
	}
	rec, e := s.attempt(h.Slot)
	if e != nil {
		return refuse(e.Error())
	}
	c, e := newCPace(rec.Slot, rec.Secret)
	if e != nil {
		return refuse("key exchange failed")
	}
	isk, e := c.key(h.Y, c.Y, true)
	if e != nil {
		return refuse("key exchange failed")
	}
	details, e := json.Marshal(rec.Invite)
	if e != nil {
		return refuse("internal error")
	}
	if e = enc.Encode(enrollOffer{Y: c.Y, Invite: details, Confirm: confirm(isk, "host", details)}); e != nil {
		return e
	}
	var f enrollFinish
	if e = dec.Decode(&f); e != nil {
		// The member aborts here when the host's confirmation fails.
		return errNotProven
	}
	if !hmac.Equal(f.Confirm, confirm(isk, "member", lengthPrefixed(details, []byte(f.Key)))) {
		_ = enc.Encode(enrollDone{Error: "the code did not match"})
		return errNotProven
	}
	if e = s.recordPending(rec.Token, f.Key); e != nil {
		_ = enc.Encode(enrollDone{Error: e.Error()})
		return e
	}
	return enc.Encode(enrollDone{OK: true})
}

// attempt counts one exchange against the slot's invitation before the host
// answers, so a guesser cannot avoid the count by disconnecting. save is
// durable before attempt returns, so the count survives a crash or power
// loss after the host has sent its offer.
func (s Store) attempt(slot string) (ShortInvite, error) {
	unknown := fmt.Errorf("no open invitation for this code; ask the owner for a new one")
	unlock, e := s.lockRetry()
	if e != nil {
		return ShortInvite{}, fmt.Errorf("host busy; try again")
	}
	defer unlock()
	all, e := s.Invitations()
	if e != nil {
		return ShortInvite{}, unknown
	}
	for _, r := range all {
		if r.Slot != slot {
			continue
		}
		switch r.State(time.Now()) {
		case "unreadable":
			return ShortInvite{}, unknown
		case "locked":
			return ShortInvite{}, fmt.Errorf("invitation locked after %d wrong codes; ask the owner for a new one", MaxAttempts)
		case "pending":
			return ShortInvite{}, fmt.Errorf("invitation already used")
		case "expired":
			return ShortInvite{}, fmt.Errorf("invitation expired")
		}
		r.Failures++
		if e = save(s.shortPath(r.Token), r); e != nil {
			return ShortInvite{}, unknown
		}
		if r.Failures >= MaxAttempts {
			// This is the last allowed exchange; later ones are refused, and
			// the enrollment key closes if nothing else is open.
			_ = s.tidyLocked()
		}
		return r, nil
	}
	return ShortInvite{}, unknown
}

func (s Store) recordPending(token, key string) error {
	k, fp, e := publicKey(key)
	if e != nil {
		return e
	}
	unlock, e := s.lockRetry()
	if e != nil {
		return fmt.Errorf("host busy; try again")
	}
	defer unlock()
	var r ShortInvite
	if e = read(s.shortPath(token), &r); e != nil {
		return fmt.Errorf("invitation withdrawn")
	}
	if r.Pending != nil || !time.Now().Before(r.Expires) {
		return fmt.Errorf("invitation already used or expired")
	}
	r.Pending = &Pending{Key: k, Fingerprint: fp, Requested: time.Now().UTC()}
	if e = save(s.shortPath(token), r); e != nil {
		return e
	}
	return s.tidyLocked()
}

// ExchangeCode is the member side of the exchange over rw. It returns the
// invitation details the host proved with the code; nothing about the
// member's key is sent until that proof checks.
func ExchangeCode(rw io.ReadWriter, code, memberKey string) (Invite, error) {
	var i Invite
	slot, secret, e := ParseCode(code)
	if e != nil {
		return i, e
	}
	c, e := newCPace(slot, secret)
	if e != nil {
		return i, e
	}
	dec := json.NewDecoder(io.LimitReader(rw, 64<<10))
	enc := json.NewEncoder(rw)
	if e = enc.Encode(enrollHello{V: 1, Slot: slot, Y: c.Y}); e != nil {
		return i, e
	}
	var o enrollOffer
	if e = dec.Decode(&o); e != nil {
		return i, fmt.Errorf("no enrollment answer from host: %w", e)
	}
	if o.Error != "" {
		return i, fmt.Errorf("host refused: %s", o.Error)
	}
	isk, e := c.key(c.Y, o.Y, false)
	if e != nil {
		return i, e
	}
	if !hmac.Equal(o.Confirm, confirm(isk, "host", o.Invite)) {
		return i, fmt.Errorf("the host could not prove it knows this code (wrong code, or not the host that issued it); nothing was sent")
	}
	if e = json.Unmarshal(o.Invite, &i); e != nil {
		return i, e
	}
	if e = ValidateInvite(i, time.Now()); e != nil {
		return i, e
	}
	if e = enc.Encode(enrollFinish{Key: memberKey, Confirm: confirm(isk, "member", lengthPrefixed(o.Invite, []byte(memberKey)))}); e != nil {
		return i, e
	}
	var d enrollDone
	if e = dec.Decode(&d); e != nil {
		return i, fmt.Errorf("no confirmation from host: %w", e)
	}
	if !d.OK {
		return i, fmt.Errorf("host refused: %s", d.Error)
	}
	return i, nil
}
