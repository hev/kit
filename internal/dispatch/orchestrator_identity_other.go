//go:build !linux && !windows

package dispatch

func roundProcessIdentity() string { return "" }
