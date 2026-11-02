//go:build !windows

package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/hev/kit/internal/dispatch"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchPeerStableAcrossRealRework(t *testing.T) {
	e, _ := fixture(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dispatch.Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: "example", Harness: "codex"}, Branch: "d/example"})
	os.WriteFile(filepath.Join(dir, "order.md"), []byte("order"), 0600)
	ps, err := e.list(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	before, err := find(ps, "local/example")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := e.Run(context.Background(), []string{"send", "local/example", "ended result", "--from", "local/desk"}, strings.NewReader(""), &out, io.Discard); err != nil || !strings.Contains(out.String(), "held") {
		t.Fatal(out.String(), err)
	}
	if _, err := dispatch.Handle(dispatch.Request{Action: "rework", Name: "example", Note: "another round", Cap: 1}); err != nil {
		t.Fatal(err)
	}
	ps, err = e.list(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	after, err := find(ps, "local/example")
	if err != nil {
		t.Fatal(err)
	}
	if before.Address != after.Address || before.Identity != after.Identity {
		t.Fatal("run address changed", before, after)
	}
	archives, _ := filepath.Glob(filepath.Join(dir, "inbox.md.*"))
	if len(archives) != 1 {
		t.Fatal("held run message lost during rework", archives)
	}
	b, _ := os.ReadFile(archives[0])
	if !strings.Contains(string(b), "ended result") {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(after)
	if !bytes.Contains(b, []byte(`"run":true`)) {
		t.Fatal(string(b))
	}
}
