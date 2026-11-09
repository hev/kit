package nodes

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAttemptRecordedBeforeOffer checks the count is on disk, in a complete
// replaced record, by the time the host's offer reaches the member. Whether
// the bytes survive power loss rests on save fsyncing the sibling and the
// directory (platformops.WriteFileAtomic); no power cut is simulated here.
func TestAttemptRecordedBeforeOffer(t *testing.T) {
	h := newShortHost(t)
	r := h.invite(t, "alex")
	before, e := os.Stat(h.s.shortPath(r.Token))
	if e != nil {
		t.Fatal(e)
	}
	c, _ := newCPace(r.Slot, "ZZZZZZZZ")
	pr, pw := io.Pipe()
	out := &syncWriter{}
	done := make(chan error, 1)
	go func() { done <- h.s.Enroll(pr, out) }()
	_ = json.NewEncoder(pw).Encode(enrollHello{V: 1, Slot: r.Slot, Y: c.Y})
	var offer enrollOffer
	waitFor(t, func() bool { return json.Unmarshal(out.Bytes(), &offer) == nil })
	// The member holds the offer and has not answered. The count is already
	// in a new file, not a rewrite of the old one in place.
	var rec ShortInvite
	if e = read(h.s.shortPath(r.Token), &rec); e != nil || rec.Failures != 1 {
		t.Fatalf("failures on disk at offer: %d %v", rec.Failures, e)
	}
	after, _ := os.Stat(h.s.shortPath(r.Token))
	if os.SameFile(before, after) {
		t.Fatal("record rewritten in place rather than replaced")
	}
	_ = pw.Close()
	<-done
	entries, _ := os.ReadDir(h.s.Dir)
	for _, f := range entries {
		if strings.HasPrefix(f.Name(), ".hev-") {
			t.Fatalf("temporary file left: %s", f.Name())
		}
	}
}

// TestUnreadableInvitationFailsClosedAlone damages one record: it matches no
// code and stays put, while other invitations keep working.
func TestUnreadableInvitationFailsClosedAlone(t *testing.T) {
	h := newShortHost(t)
	alex := h.invite(t, "alex")
	sam := h.invite(t, "sam")
	bad := h.s.shortPath(alex.Token)
	if e := os.WriteFile(bad, []byte(`{"slot":"`+alex.Slot+`","secret":"`), 0600); e != nil {
		t.Fatal(e)
	}
	all, e := h.s.Invitations()
	if e != nil || len(all) != 2 {
		t.Fatalf("listing %v %v", all, e)
	}
	var states []string
	for _, r := range all {
		states = append(states, r.State(time.Now()))
	}
	slices.Sort(states)
	if strings.Join(states, ",") != "open,unreadable" {
		t.Fatalf("states %v", states)
	}
	if _, e, _ := exchange(t, h, alex.Code()); e == nil || !strings.Contains(e.Error(), "no open invitation") {
		t.Fatalf("damaged invitation answered: %v", e)
	}
	// An empty slot cannot select the damaged record either.
	if _, e := scripted(t, h.s, enrollHello{V: 1, Slot: "", Y: make([]byte, 32)}); e == nil {
		t.Fatal("empty slot accepted")
	}
	if _, e, _ := exchange(t, h, sam.Code()); e != nil {
		t.Fatalf("other invitation blocked: %v", e)
	}
	if e := h.s.Tidy(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(bad); e != nil {
		t.Fatal("damaged record tidied away; the owner should see it")
	}
	if h.enrollOpen(t) {
		t.Fatal("damaged record kept the enrollment key open")
	}
	if _, e := h.s.AcceptPending("sam", h.keys, h.ek.Exe, false, nil); e != nil {
		t.Fatal(e)
	}
}

func TestDamagedEnrollRecordRefusesNewInvitation(t *testing.T) {
	h := newShortHost(t)
	_ = os.MkdirAll(h.s.Dir, 0700)
	if e := os.WriteFile(filepath.Join(h.s.Dir, "enroll.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := h.s.ShortInvite(Invite{Name: "alex", Address: "100.64.0.10", User: "bot", HostKey: testKey(t)}, time.Hour, nil, h.ek); e == nil || !strings.Contains(e.Error(), "unreadable") {
		t.Fatalf("invited over a damaged enrollment record: %v", e)
	}
	if all, _ := h.s.Invitations(); len(all) != 0 {
		t.Fatalf("left %+v", all)
	}
}

// TestLockReleasedWhenHolderDies runs a child that takes the store lock and
// is killed holding it. The OS lock goes with the process, so nothing stale
// is left to clean up by hand.
func TestLockReleasedWhenHolderDies(t *testing.T) {
	if dir := os.Getenv("HEV_NODES_LOCK_HOLDER"); dir != "" {
		if _, e := (Store{dir}).lock(); e != nil {
			fmt.Println("lock failed:", e)
			os.Exit(1)
		}
		fmt.Println("locked")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	s := Store{t.TempDir()}
	c := exec.Command(os.Args[0], "-test.run=^TestLockReleasedWhenHolderDies$")
	c.Env = append(os.Environ(), "HEV_NODES_LOCK_HOLDER="+s.Dir)
	out, e := c.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	line := make([]byte, 7)
	if _, e = io.ReadFull(out, line); e != nil || string(line) != "locked\n"[:7] {
		t.Fatalf("child: %q %v", line, e)
	}
	if _, e = s.lock(); e == nil {
		t.Fatal("lock taken while the child holds it")
	}
	_ = c.Process.Kill()
	_ = c.Wait()
	unlock, e := s.lock()
	if e != nil {
		t.Fatalf("lock still held after the holder died: %v", e)
	}
	unlock()
}

// lifecycleHost starts from an authorized_keys file and runs overlapping
// invitations: alex and sam are accepted, kim is still open.
func lifecycleHost(t *testing.T, original string, admin bool) shortHost {
	t.Helper()
	h := newShortHost(t)
	if admin {
		h.keys = filepath.Join(filepath.Dir(h.keys), "administrators_authorized_keys")
		h.ek = EnrollKey{Path: h.keys, Exe: h.ek.Exe, Admin: true}
	}
	_ = os.MkdirAll(filepath.Dir(h.keys), 0700)
	if e := os.WriteFile(h.keys, []byte(original), 0600); e != nil {
		t.Fatal(e)
	}
	codes := map[string]string{}
	for _, n := range []string{"alex", "sam", "kim"} {
		codes[n] = h.invite(t, n).Code()
	}
	for _, n := range []string{"alex", "sam"} {
		if _, e, _ := exchange(t, h, codes[n]); e != nil {
			t.Fatal(e)
		}
		if _, e := h.s.AcceptPending(n, h.keys, h.ek.Exe, admin, nil); e != nil {
			t.Fatal(e)
		}
	}
	return h
}

// TestOverlappingLifecyclesRestoreOriginalBytes covers the separator hev
// inserts when authorized_keys lacks a final newline: whichever owned line
// goes last, the file ends as the original bytes.
func TestOverlappingLifecyclesRestoreOriginalBytes(t *testing.T) {
	originals := map[string]string{
		"no final newline": "ssh-ed25519 AAAA unrelated",
		"crlf":             "ssh-ed25519 AAAA unrelated\r\n# second\r\n",
		"crlf unfinished":  "ssh-ed25519 AAAA unrelated\r\n# second",
		"empty":            "",
	}
	orders := [][]string{
		{"alex", "sam", "kim"}, {"alex", "kim", "sam"},
		{"sam", "alex", "kim"}, {"sam", "kim", "alex"},
		{"kim", "alex", "sam"}, {"kim", "sam", "alex"},
	}
	for label, original := range originals {
		for _, admin := range []bool{false, true} {
			for _, order := range orders {
				t.Run(fmt.Sprintf("%s/admin=%v/%s", label, admin, strings.Join(order, "-")), func(t *testing.T) {
					h := lifecycleHost(t, original, admin)
					for _, n := range order {
						if e := h.s.Remove(n); e != nil {
							t.Fatal(e)
						}
					}
					if got := h.authorized(t); got != original {
						t.Fatalf("original %q final %q", original, got)
					}
					if _, e := os.Stat(filepath.Join(h.s.Dir, "separators.json")); !os.IsNotExist(e) {
						t.Fatalf("separator marker left behind: %v", e)
					}
				})
			}
		}
	}
}

// TestReviewOverlapScenario is the review's case: alex accepted while sam's
// invitation is open, then sam and alex removed, in both orders.
func TestReviewOverlapScenario(t *testing.T) {
	original := "ssh-ed25519 AAAA unrelated"
	for _, order := range [][]string{{"sam", "alex"}, {"alex", "sam"}} {
		t.Run(strings.Join(order, "-"), func(t *testing.T) {
			h := newShortHost(t)
			_ = os.MkdirAll(filepath.Dir(h.keys), 0700)
			_ = os.WriteFile(h.keys, []byte(original), 0600)
			r := h.invite(t, "alex")
			h.invite(t, "sam")
			if _, e, _ := exchange(t, h, r.Code()); e != nil {
				t.Fatal(e)
			}
			if _, e := h.s.AcceptPending("alex", h.keys, h.ek.Exe, false, nil); e != nil {
				t.Fatal(e)
			}
			for _, n := range order {
				if e := h.s.Remove(n); e != nil {
					t.Fatal(e)
				}
			}
			if got := h.authorized(t); got != original {
				t.Fatalf("original %q final %q", original, got)
			}
		})
	}
}

// TestHandEditedFileDropsMarker: if the owner edits the file so the inserted
// newline is no longer last, the marker is dropped with the last owned line
// and the owner's bytes are left alone.
func TestHandEditedFileDropsMarker(t *testing.T) {
	h := newShortHost(t)
	_ = os.MkdirAll(filepath.Dir(h.keys), 0700)
	_ = os.WriteFile(h.keys, []byte("ssh-ed25519 AAAA unrelated"), 0600)
	h.invite(t, "alex")
	f, _ := os.OpenFile(h.keys, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("ssh-ed25519 BBBB owner\n")
	_ = f.Close()
	if e := h.s.Remove("alex"); e != nil {
		t.Fatal(e)
	}
	if got := h.authorized(t); got != "ssh-ed25519 AAAA unrelated\nssh-ed25519 BBBB owner\n" {
		t.Fatalf("final %q", got)
	}
	if _, e := os.Stat(filepath.Join(h.s.Dir, "separators.json")); !os.IsNotExist(e) {
		t.Fatal("marker kept with no owned line left")
	}
}

// TestSeparatorRecordedBeforeAppend: the marker reaches the caller (which
// saves it) while the file is still unchanged, so a crash right after the
// append leaves a record that restores the original bytes.
func TestSeparatorRecordedBeforeAppend(t *testing.T) {
	h := newShortHost(t)
	original := "ssh-ed25519 AAAA unrelated"
	_ = os.MkdirAll(filepath.Dir(h.keys), 0700)
	_ = os.WriteFile(h.keys, []byte(original), 0600)
	_ = os.MkdirAll(h.s.Dir, 0700)
	line, _ := EnrollLine("linux", h.ek.Exe)
	p := filepath.Join(h.s.Dir, "enroll.json")
	if _, e := appendLine(h.keys, false, line, func(sep string) error {
		if sep == "" || h.authorized(t) != original {
			t.Fatalf("marker %q with file %q", sep, h.authorized(t))
		}
		return save(p, enrollRecord{Path: h.keys, Line: line, SeparatorPrefix: sep})
	}); e != nil {
		t.Fatal(e)
	}
	// Die here: nothing else is written. Recovery restores the file.
	if e := h.s.Tidy(); e != nil {
		t.Fatal(e)
	}
	if got := h.authorized(t); got != original {
		t.Fatalf("recovered %q", got)
	}

	// The same holds for a grant: its access record, with the marker, exists
	// before the key line does.
	r := h.invite(t, "alex")
	if _, e, _ := exchange(t, h, r.Code()); e != nil {
		t.Fatal(e)
	}
	a, e := h.s.AcceptPending("alex", h.keys, h.ek.Exe, false, nil)
	if e != nil {
		t.Fatal(e)
	}
	var rec Access
	if e = read(filepath.Join(h.s.Dir, "access-"+a.ID+".json"), &rec); e != nil || rec.SeparatorPrefix == "" {
		t.Fatalf("access record %+v %v", rec, e)
	}
	if e = h.s.Remove("alex"); e != nil {
		t.Fatal(e)
	}
	if got := h.authorized(t); got != original {
		t.Fatalf("final %q", got)
	}
}
