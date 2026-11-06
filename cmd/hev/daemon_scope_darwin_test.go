//go:build darwin

package main

import "testing"

func TestParseLaunchctlPrint(t *testing.T) {
	out := `gui/501/com.hev.hevd = {
	active count = 1
	path = /Users/example/Library/LaunchAgents/com.hev.hevd.plist
	type = LaunchAgent
	state = running
	program = /bin/zsh
	pid = 4242
	endpoints = {
		"com.example.sub" = {
			pid = 7
		}
	}
}`
	pid, plist := parseLaunchctlPrint(out)
	if pid != 4242 || plist != "/Users/example/Library/LaunchAgents/com.hev.hevd.plist" {
		t.Fatalf("pid=%d plist=%q", pid, plist)
	}
	if pid, plist := parseLaunchctlPrint("state = not running\n"); pid != 0 || plist != "" {
		t.Fatalf("unloaded job: pid=%d plist=%q", pid, plist)
	}
}
