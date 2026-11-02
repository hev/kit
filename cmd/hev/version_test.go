package main

import (
	"bytes"
	"github.com/hev/kit/internal/version"
	"testing"
)

func TestVersionCommands(t *testing.T) {
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "0.6.0-test"
	for _, args := range [][]string{{"--version"}, {"version"}} {
		var out bytes.Buffer
		rootCmd.Version = version.String()
		rootCmd.SetOut(&out)
		rootCmd.SetArgs(args)
		if err := rootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if out.String() != "0.6.0-test\n" {
			t.Fatalf("%v: %q", args, out.String())
		}
	}
	rootCmd.SetOut(nil)
	rootCmd.SetArgs(nil)
}
