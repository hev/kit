//go:build !linux && !darwin

package main

import "errors"

func hostOS() string         { return "unsupported" }
func harness([]string) error { return errors.New("execution unavailable on this platform") }
