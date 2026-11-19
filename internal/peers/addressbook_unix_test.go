//go:build !windows

package peers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A send to the owner's session reaches ssh with the address book's
// destination, not the bare label that "Could not resolve hostname".
func TestSSHSendUsesAddressBookDestination(t *testing.T) {
	fakeAddressBook(t)
	bin := t.TempDir()
	argsLog := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsLog + "'\ncat >/dev/null\nprintf '{\"message\":\"delivered\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out, errout bytes.Buffer
	if err := (SSHTransport{Quiet: true}).Execute(context.Background(), "macbook", []string{"send", "foh-bcc", "hello", "--from", "oven/d-run"}, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal(err, errout.String())
	}
	b, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(b)), "\n")
	dest := ""
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			dest = args[i+1]
		}
	}
	if dest != "100.66.207.41" {
		t.Fatalf("ssh destination %q, args %q", dest, args)
	}
}
