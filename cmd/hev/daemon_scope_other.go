//go:build !linux

package main

// Platforms without safe process-environment inspection use their scoped service
// manager. Never signal a process discovered only through a host-wide name scan.
func daemonInScope(pid int) bool { return false }
