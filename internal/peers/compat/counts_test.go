package compat

import "testing"

func TestCountsSessionLifecycle(t *testing.T) {
	rows := []*Row{{Session: "planted-session", Cwd: "/private/path", Title: "repo-private", Agent: "claude", Role: "foh"}, {Agent: "codex", Role: "boh"}, {Agent: "pi", Role: "offline"}}
	state := "working"
	read := func(*Row) string { return state }
	c := countRows(rows, read)
	if c.Harness["claude"][0] != 1 || c.Role["foh"][0] != 1 || len(c.Harness) != 2 {
		t.Fatal(c)
	}
	state = "idle"
	c = countRows(rows, read)
	if c.Harness["claude"][0] != 0 || c.Harness["claude"][1] != 1 {
		t.Fatal(c)
	}
	rows = rows[1:]
	c = countRows(rows, read)
	if _, ok := c.Harness["claude"]; ok {
		t.Fatal(c)
	}
}
