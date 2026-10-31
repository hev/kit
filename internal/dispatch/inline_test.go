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
