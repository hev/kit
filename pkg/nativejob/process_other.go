//go:build !linux

package nativejob

import "errors"

// Native macOS execution remains disabled until a birth-identity adapter and
// exact-candidate native acceptance are delivered. Never degrade to PID checks.
func processBirth(pid int) (Birth, error) {
	return Birth{}, errors.New("native process birth adapter unsupported on this platform")
}
