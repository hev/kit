package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
)

func TestPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, name := range []string{"hello", "builtin", "alias"} {
		if err := os.WriteFile(filepath.Join(dir, "hev-"+name), []byte("#!/bin/sh\n/bin/cat\nprintf '%s' \"$1\"\nexit 7\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	root := &cobra.Command{Use: "hev"}
	root.AddCommand(&cobra.Command{Use: "builtin", Aliases: []string{"alias"}})
	var out bytes.Buffer
	handled, code := executePlugin(root, []string{"hello", "argument"}, bytes.NewBufferString("stdin:"), &out, &out)
	if !handled || code != 7 || out.String() != "stdin:argument" {
		t.Fatalf("%v %d %q", handled, code, out.String())
	}
	for _, name := range []string{"builtin", "alias", "missing", "help"} {
		if handled, _ := executePlugin(root, []string{name}, nil, &out, &out); handled {
			t.Fatalf("handled %s", name)
		}
	}
	if got := discoveredPlugins(root); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Fatal(got)
	}
	installPluginHelp(root)
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Plugins:\n  hello")) {
		t.Fatal(out.String())
	}
}
