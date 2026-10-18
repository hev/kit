//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeOriginalCLIReadOnlyAndNoArgvEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	enrollment := n.Enrollment{HostID: "host", ExecutorID: "executor"}
	s, e := n.Open(root, enrollment)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	run := func(host, job string) ([]byte, error) {
		cmd := newOriginalInspectCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		args := []string{"--root", root, "--host", host, "--executor", enrollment.ExecutorID}
		if job != "" {
			args = append(args, "--job", job)
		}
		cmd.SetArgs(args)
		e := cmd.ExecuteContext(context.Background())
		return out.Bytes(), e
	}
	b, e := run(enrollment.HostID, "")
	if e != nil {
		t.Fatal(string(b), e)
	}
	var proof n.OwnerProof
	if json.Unmarshal(b, &proof) != nil || proof.JobID != "" || proof.Identity != nil || proof.Version != n.OwnerProofVersion {
		t.Fatal(string(b))
	}
	for _, key := range []string{`"argv"`, `"environment"`, `"owner_birth"`, `"child_birth"`, `"acknowledged"`} {
		if strings.Contains(string(b), key) {
			t.Fatal("enrollment-only leaked job facts", string(b))
		}
	}
	if _, e = run("foreign", ""); e == nil {
		t.Fatal("foreign enrollment accepted")
	}
	if _, e = run(enrollment.HostID, "unknown-job"); e == nil {
		t.Fatal("unknown job accepted")
	}
	missing := filepath.Join(t.TempDir(), "never-create")
	cmd := newOriginalInspectCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--root", missing, "--host", "host", "--executor", "executor"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if cmd.Execute() == nil {
		t.Fatal("missing journal accepted")
	}
	if _, e = os.Stat(missing); !os.IsNotExist(e) {
		t.Fatal("read-only CLI created journal")
	}
}
