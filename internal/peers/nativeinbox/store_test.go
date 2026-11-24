package nativeinbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPresenceTracksStateAndRejectsStaleIncarnations(t *testing.T) {
	s := AtHome(t.TempDir())
	peer, err := s.Register("/private/work", "topic", "%7")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Lookup(peer.Name); err != nil || got.State != "idle" || got.Agent != "hev" || got.Branch != "topic" {
		t.Fatalf("initial presence: %+v, %v", got, err)
	}
	if _, err := s.Register("", "", ""); err == nil {
		t.Fatal("replaced a live registration")
	}
	if err := s.Update(peer, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Lookup(peer.Name); got.State != "working" {
		t.Fatalf("presence state = %q", got.State)
	}
	msg := Message{ID: "previous", From: "local", Body: "old message", Mode: Background}
	if err := s.Send(peer, msg); err != nil {
		t.Fatal(err)
	}
	dead := peer
	dead.Birth = "different-process"
	if err := s.writePresence(dead); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.List(); err != nil || len(rows) != 0 {
		t.Fatalf("listed a stale PID incarnation: %v, %v", rows, err)
	}
	if _, err := os.Stat(s.presencePath(peer.Name)); !os.IsNotExist(err) {
		t.Fatalf("stale presence remains: %v", err)
	}
	restarted, err := s.Register("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(peer, msg); err == nil {
		t.Fatal("sent to a replaced incarnation")
	}
	if got, err := s.Drain(restarted, 1); err != nil || len(got) != 0 {
		t.Fatalf("old inbox rebound: %v, %v", got, err)
	}
	if err := s.Close(peer); err == nil {
		t.Fatal("stale owner removed a replacement")
	}
	if err := s.Close(restarted); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(restarted.Name); !os.IsNotExist(err) {
		t.Fatalf("clean exit left presence: %v", err)
	}

	// A PID that cannot belong to a running process is pruned as well.
	dead.PID = 2147483647
	dead.Name = name(dead.PID)
	dead.UpdatedAt = time.Now().UTC()
	if err := s.writePresence(dead); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(dead.Name); !os.IsNotExist(err) {
		t.Fatalf("dead process accepted: %v", err)
	}
}

func TestInboxIsPrivateBoundedAndSerializesWriters(t *testing.T) {
	s := AtHome(t.TempDir())
	peer, err := s.Register("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(peer) })
	for _, mode := range []string{Background, Interrupt} {
		msg := Message{ID: mode, From: "sender", Body: "line one\nline two", Mode: mode}
		if err := s.Send(peer, msg); err != nil {
			t.Fatal(err)
		}
	}
	path := s.inboxPath(peer.Name)
	if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("inbox permissions: %v, %v", info, err)
	}
	got, err := s.Drain(peer, 1)
	if err != nil || len(got) != 1 || got[0].Mode != Background || got[0].Body != "line one\nline two" ||
		got[0].From != "sender" || got[0].TS.IsZero() {
		t.Fatalf("background delivery: %+v, %v", got, err)
	}
	got, err = s.Drain(peer, 8)
	if err != nil || len(got) != 1 || got[0].ID != Interrupt {
		t.Fatalf("interrupt delivery: %+v, %v", got, err)
	}

	var wg sync.WaitGroup
	for _, id := range []string{"one", "two", "three", "four"} {
		wg.Go(func() {
			if err := s.Send(peer, Message{ID: id, From: "sender", Body: id, Mode: Background}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got, err = s.Drain(peer, 8)
	if err != nil || len(got) != 4 {
		t.Fatalf("concurrent delivery: %+v, %v", got, err)
	}
	msg := Message{ID: "bounded", From: "sender", Body: "ok", Mode: Background}
	for i := 0; i < maxMessages; i++ {
		if err := s.Send(peer, msg); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Send(peer, msg); !errors.Is(err, errFull) {
		t.Fatalf("count limit: %v", err)
	}
	if got, err := s.Drain(peer, maxMessages); err != nil || len(got) != maxMessages {
		t.Fatalf("drain full inbox: %d, %v", len(got), err)
	}
	msg.Body = strings.Repeat("a", MaxBodyBytes)
	for i := 0; i < MaxBytes/MaxBodyBytes-1; i++ {
		if err := s.Send(peer, msg); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Send(peer, msg); !errors.Is(err, errFull) {
		t.Fatalf("byte limit: %v", err)
	}
}

func TestInboxErrorsCarryNeitherPathsNorMessageBodies(t *testing.T) {
	s := AtHome(t.TempDir())
	peer, err := s.Register("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	path := s.inboxPath(peer.Name)
	const secret = "private-message-sentinel"
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	err = s.Send(peer, Message{ID: "bad", From: "sender", Body: secret, Mode: Background})
	if err == nil || strings.Contains(err.Error(), s.dir) || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe store failure: %v", err)
	}
	if err := s.Close(peer); err != nil {
		t.Fatal(err)
	}
}

func TestAtHomeRespectsLegacyPeersV2Layout(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".local", "state", "peers-v2")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	s := AtHome(home)
	peer, err := s.Register(home, "main", "%9")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(peer) })
	if s.dir != filepath.Join(legacy, "native") {
		t.Fatalf("legacy store dir = %q, want %q", s.dir, filepath.Join(legacy, "native"))
	}
	if _, err := os.Stat(filepath.Join(home, ".hev", "peers")); !os.IsNotExist(err) {
		t.Fatalf("legacy host unexpectedly created ~/.hev/peers: %v", err)
	}

	// Fresh host with ~/.hev/peers uses ~/.hev/peers/native.
	freshHome := t.TempDir()
	fresh := AtHome(freshHome)
	if fresh.dir != filepath.Join(freshHome, ".hev", "peers", "native") {
		t.Fatalf("fresh store dir = %q", fresh.dir)
	}
}
