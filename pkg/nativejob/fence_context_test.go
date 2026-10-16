//go:build linux || darwin

package nativejob

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestIssuedFenceExactRestartExpiryTransfer(t *testing.T) {
	root := t.TempDir()
	journal := filepath.Join(root, "journal")
	s := testStore(t, journal)
	f := Fence{OrderUID: "order", ChefID: "chef", Epoch: 1, Expires: time.Now().Add(time.Hour).UTC()}
	if err := s.AdvanceFence(f); err != nil {
		t.Fatal(err)
	}
	r := testRequest(root)
	j := mustSubmit(t, s, r)
	before := encode(j)
	for _, tuple := range []struct {
		o, c string
		e    uint64
	}{{"", "chef", 1}, {"order", "", 1}, {"order", "chef", 0}, {"other", "chef", 1}, {"order", "other", 1}, {"order", "chef", 2}} {
		got, err := s.IssuedFence(tuple.o, tuple.c, tuple.e)
		if !errors.Is(err, ErrFence) || got != (Fence{}) {
			t.Fatalf("foreign tuple: %#v %v", got, err)
		}
	}
	got, err := s.IssuedFence("order", "chef", 1)
	if err != nil || encode(got) != encode(f) {
		t.Fatal(got, err)
	}
	changed := f
	changed.Expires = changed.Expires.Add(time.Minute)
	if err = s.AdvanceFence(changed); !errors.Is(err, ErrFence) {
		t.Fatal("same epoch renewal", err)
	}
	s.Close()
	s = testStore(t, journal)
	got, err = s.IssuedFence("order", "chef", 1)
	if err != nil || encode(got) != encode(f) {
		t.Fatal("restart", got, err)
	}
	s.now = func() time.Time { return f.Expires }
	got, err = s.IssuedFence("order", "chef", 1)
	if !errors.Is(err, ErrFence) || got != (Fence{}) {
		t.Fatal("expired", got, err)
	}
	s.now = time.Now
	if err = s.AdvanceFence(Fence{OrderUID: "order", ChefID: "successor", Epoch: 2, Expires: f.Expires}); err != nil {
		t.Fatal(err)
	}
	got, err = s.IssuedFence("order", "chef", 1)
	if !errors.Is(err, ErrFence) || got != (Fence{}) {
		t.Fatal("history inferred", got, err)
	}
	after, err := s.Query(j.ID)
	if err != nil || encode(after) != before || RequestDigest(after.Request) != r.PayloadDigest {
		t.Fatal("read changed original job", after, err)
	}
}
