package main

import "github.com/hev/kit/internal/dispatch"

// Batch files cannot represent arbitrary argv safely. Refuse shell syntax rather
// than letting cmd expand user arguments or executable paths.
func cmdPluginLine(path string, args []string) (string, error) {
	return dispatch.CmdPluginLine(path, args)
}
