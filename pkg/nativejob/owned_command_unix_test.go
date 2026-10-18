//go:build linux || darwin

package nativejob

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type unknownTerminalCommand struct{ ownedCommand }

func (p unknownTerminalCommand) Wait() (int, error) {
	if _, e := p.ownedCommand.Wait(); e != nil {
		return 0, e
	}
	return 0, ErrOwnedCommandUnknown
}
func TestUnknownOwnedTerminalNeverCompletesOrReplays(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	j := mustSubmit(t, s, testRequest(root))
	starter := func(cmd *exec.Cmd) (ownedCommand, error) {
		p, e := platformStartCommand(cmd)
		if p == nil {
			return nil, e
		}
		return unknownTerminalCommand{p}, e
	}
	if _, e := s.executeWithCommand(context.Background(), j.ID, nil, nil, 0, starter); !errors.Is(e, ErrOwnedCommandUnknown) {
		t.Fatal(e)
	}
	got, e := s.Query(j.ID)
	if e != nil || got.State != "Uncertain" || got.Result != nil || got.ResultDigest != "" {
		t.Fatal(got, e)
	}
	events, e := s.Outbox()
	if e != nil || len(events) != 0 {
		t.Fatal("unknown terminal emitted completion", events, e)
	}
	if _, e = s.Execute(j.ID); e == nil {
		t.Fatal("unknown terminal replayed")
	}
	effects, e := os.ReadFile(filepath.Join(root, "effects"))
	if e != nil || strings.Count(string(effects), "one") != 1 {
		t.Fatalf("effect count changed: %q %v", effects, e)
	}
}
func TestOwnedUnixOrdinaryNonzeroIsKnownRootExit(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := testRequest(root)
	r.Argv = []string{"/bin/sh", "-c", "exit 7"}
	r.PayloadDigest = RequestDigest(r)
	j := mustSubmit(t, s, r)
	got, e := s.Execute(j.ID)
	if e != nil || got.State != "Completed" || got.Result == nil || got.Result.ExitCode != 7 {
		t.Fatal(got, e)
	}
}
