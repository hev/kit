//go:build !linux && !darwin && !windows

package nativeinbox

func processBirth(int) string { return "" }
