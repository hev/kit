package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResourceBridgeStreamsArgumentsAndExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture; native Windows coverage is in platform/service tests")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "backend")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\ncat\nprintf 'backend error\\n' >&2\nexit 7\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_RESOURCE_BACKEND", path)
	var out, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader("stdin resource\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	err := runResourceBackend(cmd, "apply", []string{"-f", "-", "--dry-run", "-o", "json"})
	exit, ok := err.(*resourceBackendExit)
	if !ok || exit.code != 7 || out.String() != "apply\n-f\n-\n--dry-run\n-o\njson\nstdin resource\n" || stderr.String() != "backend error\n" {
		t.Fatalf("%v %q %q", err, out.String(), stderr.String())
	}
}
func TestResourceBridgeUnavailableAndHelp(t *testing.T) {
	t.Setenv("HEV_RESOURCE_BACKEND", filepath.Join(t.TempDir(), "absent"))
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runResourceBackend(cmd, "get", []string{"orders"})
	if err == nil || !strings.Contains(err.Error(), `"reason":"ResourceBackendUnavailable"`) {
		t.Fatal(err)
	}
	for _, verb := range []string{"apply", "get", "describe", "logs", "watch", "wait", "api-resources"} {
		resource := resourceBackendCommand(verb)
		var b bytes.Buffer
		resource.SetOut(&b)
		if err := resource.RunE(resource, []string{"--help"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "staged declarations") {
			t.Fatal(b.String())
		}
	}
	for _, name := range []string{"index", "native-job"} {
		found := false
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("lost existing command %s", name)
		}
	}
}

func TestWatchAliasDoesNotShadowResourceCommand(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"watch", "order/example"})
	if err != nil || command.Name() != "watch" {
		t.Fatalf("watch resolved to %v: %v", command, err)
	}
	original := legacyWatchBrowser
	defer func() { legacyWatchBrowser = original }()
	calls := 0
	legacyWatchBrowser = func(*cobra.Command, []string) error { calls++; return nil }
	if err := command.RunE(command, nil); err != nil || calls != 1 {
		t.Fatalf("bare watch compatibility: %v calls=%d", err, calls)
	}
}
