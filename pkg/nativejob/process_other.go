//go:build !linux && !windows && !darwin

package nativejob

import "errors"

// Unsupported platforms never degrade to PID-only ownership checks.
func processBirth(pid int) (Birth, error) {
	return Birth{}, errors.New("native process birth adapter unsupported on this platform")
}
