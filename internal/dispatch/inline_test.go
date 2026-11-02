package dispatch

import (
	"strings"
	"testing"
	"time"
)

func TestInline(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	used := map[string]bool{"fix-the-thing-20261031": true, "fix-the-thing-20261031-2": true}
	o, text, err := Inline(Order{Repo: "sample", Host: "here"}, "Fix the thing!", nil, used, now)
	if err != nil || o.Name != "fix-the-thing-20261031-3" || !strings.Contains(text, "- The task above is done as described") {
		t.Fatalf("%+v %s %v", o, text, err)
	}
	o, text, err = Inline(Order{Name: "chosen", Repo: "sample", Model: "example", Timeout: 4}, "task", []string{"first", "second"}, nil, now)
	if err != nil || o.Name != "chosen" || !strings.Contains(text, "- first\n- second") || strings.Contains(text, "done as described") {
		t.Fatalf("%+v %s %v", o, text, err)
	}
	if _, _, err = Inline(Order{Repo: "sample"}, " ", nil, nil, now); err == nil {
		t.Fatal("empty task accepted")
	}
}

func TestSelectHost(t *testing.T) {
	c := Config{Hosts: map[string]Host{"a": {Cap: 2}, "b": {Cap: 4}}}
	inv := map[string]Response{"a": {Running: 0}, "b": {Running: 1}}
	if n, _ := SelectHost(c, inv); n != "b" {
		t.Fatal(n)
	}
	c.DefaultHost = "a"
	if n, _ := SelectHost(c, inv); n != "a" {
		t.Fatal(n)
	}
}

func TestPreferredHostFallback(t *testing.T) {
	c := Config{DefaultHost: "a", Hosts: map[string]Host{"a": {Cap: 1}, "b": {Cap: 2}}}
	for _, r := range []Response{{Running: 1}, {Error: "unreachable"}} {
		n, err := SelectHost(c, map[string]Response{"a": r, "b": {}})
		if err != nil || n != "b" {
			t.Fatalf("%s %v", n, err)
		}
	}
	if _, err := SelectHost(c, map[string]Response{"a": {Running: 1}, "b": {Running: 2}}); err == nil {
		t.Fatal("full hosts selected")
	}
}

func TestExplicitNameCollision(t *testing.T) {
	if _, _, err := Inline(Order{Name: "taken", Repo: "sample"}, "task", nil, map[string]bool{"taken": true}, time.Now()); err == nil {
		t.Fatal("existing name accepted")
	}
}

func TestSelectHostReceiverCapacity(t *testing.T) {
	c := Config{Hosts: map[string]Host{"a": {Cap: 100}, "b": {Cap: 8}}}
	inv := map[string]Response{"a": {Cap: 2, Running: 2}, "b": {Cap: 8, Running: 3}}
	if got, err := SelectHost(c, inv); err != nil || got != "b" {
		t.Fatalf("host=%s err=%v", got, err)
	}
	c.DefaultHost = "a"
	if got, err := SelectHost(c, inv); err != nil || got != "b" {
		t.Fatalf("full default: host=%s err=%v", got, err)
	}
	for _, tc := range []struct{ sender, receiver, running, want int }{
		{8, 0, 3, 5}, {0, 8, 3, 5}, {2, 8, 1, 1}, {100, 2, 2, 0},
	} {
		if got := AvailableSlots(Host{Cap: tc.sender}, Response{Cap: tc.receiver, Running: tc.running}); got != tc.want {
			t.Errorf("%+v: got %d", tc, got)
		}
	}
}
