//go:build !windows

package nativejob

func journalPlatformPreflight() error { return nil }
