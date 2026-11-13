//go:build windows

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTokenStoreCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "token.dpapi")
	for _, verb := range []string{"create", "inspect"} {
		c := tokenStoreCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetIn(strings.NewReader("fake-stdin-token\n"))
		c.SetArgs([]string{verb, "--file", path, "--scope", "LocalMachine", "--entropy", "example-entropy"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "fake-stdin-token") {
			t.Fatal("token printed")
		}
		for _, want := range []string{path, "LocalMachine", "S-1-5-18", "S-1-5-32-544"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("missing %s", want)
			}
		}
	}
}
