//go:build !linux

package dispatch

func roundProcessIdentity() string { return "" }

func processAncestors() map[int]bool { return nil }
