package main

import "github.com/hev/kit/internal/update"

// No test in this package reaches the real repository: anything that looks a
// build up without a fake of its own finds nothing listening.
func init() {
	updateSource = update.Source{API: "http://127.0.0.1:1/api", Download: "http://127.0.0.1:1/download"}
}
