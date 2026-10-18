//go:build linux || darwin

package nativegate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	n "github.com/hev/kit/pkg/nativejob"
)

func TestMaintainedReceiverCommandInspectionAndDeadline(t *testing.T) {
	root := t.TempDir()
	journal := filepath.Join(root, "journal")
	enrollment := n.Enrollment{HostID: "host", ExecutorID: "executor"}
	s, err := n.Open(journal, enrollment)
	if err != nil {
		t.Fatal(err)
	}
	c, p, _ := sample()
	r := c.Request
	r.StartAuthority = nil
	r.Directory = root
	r.Argv = []string{"/bin/sh", "-c", "printf command-secret"}
	r.PayloadDigest = n.RequestDigest(r)
	if err = s.AdvanceFence(p.OriginalFence); err != nil {
		t.Fatal(err)
	}
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(j.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	cfg := ReceiverConfig{Version: ReceiverVersion, JournalRoot: journal, Enrollment: enrollment, TimeoutSeconds: 5}
	configPath := filepath.Join(root, "receiver.json")
	raw, _ := json.Marshal(cfg)
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "hev-native-receiver")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hev-native-receiver")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v: %s", err, output)
	}
	call := ReceiverCall{Version: ReceiverVersion, Operation: "inspect", JobID: j.ID}
	raw, _ = json.Marshal(call)
	cmd := exec.Command(binary, "--config", configPath)
	cmd.Stdin = strings.NewReader(string(raw))
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var inspection Inspection
	if err = json.Unmarshal(output, &inspection); err != nil || inspection.Completion == nil || inspection.JobID != j.ID {
		t.Fatal(string(output), err)
	}
	if strings.Contains(string(output), "command-secret") {
		t.Fatal("request payload leaked")
	}
	call.Operation = "ack"
	call.Identity = &inspection.Identity
	call.EventID = inspection.Completion.EventID
	call.ResultDigest = inspection.Completion.ResultDigest
	raw, _ = json.Marshal(call)
	cmd = exec.Command(binary, "--config", configPath)
	cmd.Stdin = strings.NewReader(string(raw))
	output, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(output, &inspection); err != nil || !inspection.Acknowledged || inspection.Completion == nil {
		t.Fatal(string(output), err)
	}
	// Blocking stdin cannot turn this one-shot receiver into an unbounded service.
	cfg.TimeoutSeconds = 1
	raw, _ = json.Marshal(cfg)
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, binary, "--config", configPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	err = cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 124 || ctx.Err() != nil {
		t.Fatal("unbounded stdin", err, ctx.Err())
	}
	// Private owner configuration is required even for a read-only operation.
	if err = os.Chmod(configPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadReceiverConfig(configPath); err == nil {
		t.Fatal("public configuration accepted")
	}
}
