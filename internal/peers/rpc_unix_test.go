//go:build !windows

package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestSSHUsesDispatchRPCEnvelope(t *testing.T) {
	bin := t.TempDir()
	capture := filepath.Join(bin, "request")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PEERS_RPC_CAPTURE", capture)
	script := `#!/bin/sh
[ "$*" = "-o BatchMode=yes -o ConnectTimeout=5 -o ServerAliveInterval=15 -o ServerAliveCountMax=2 -- remote hev dispatch rpc" ] || exit 7
cat > "$PEERS_RPC_CAPTURE"
printf '{"log":"rpc stdout","message":"rpc stderr"}\n'
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	args := []string{"send", "desk", "$(touch never)", "--from", "origin/sender"}
	if err := (SSHTransport{Quiet: true}).Execute(context.Background(), "remote", args, strings.NewReader("body"), &out, &errout); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var q dispatch.Request
	if err := json.Unmarshal(b, &q); err != nil {
		t.Fatal(err)
	}
	var payload RPCRequest
	if err := json.Unmarshal([]byte(q.Note), &payload); err != nil {
		t.Fatal(err)
	}
	if q.Action != "peers" || payload.Args[2] != args[2] || payload.Stdin != "" || out.String() != "rpc stdout" || errout.String() != "rpc stderr" {
		t.Fatal(q, payload, out.String(), errout.String())
	}
	// A Markdown bullet is a message, not a flag on the remote receiver.
	dash := []string{"send", "desk", "- review passed", "--from", "origin/sender", "--inbox-only"}
	if err := (SSHTransport{Quiet: true}).Execute(context.Background(), "remote", dash, strings.NewReader(""), &out, &errout); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &q); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(q.Note), &payload); err != nil {
		t.Fatal(err)
	}
	got := payload.Args
	if len(got) < 4 || got[len(got)-3] != "--" || got[len(got)-2] != "desk" || got[len(got)-1] != "- review passed" {
		t.Fatal(got)
	}
	out.Reset()
	errout.Reset()
	if err := (SSHTransport{Quiet: true}).Execute(context.Background(), "remote", []string{"send", "d-task", "-", "--from", "origin/sender"}, strings.NewReader("message"), &out, &errout); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &q); err != nil {
		t.Fatal(err)
	}
	if q.Action != "peer-send" || q.Name != "task" || q.Note != messageHeader("origin/sender")+"\nmessage" {
		t.Fatal(q)
	}
}
