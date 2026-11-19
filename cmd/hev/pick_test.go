package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/workspace"
)

func TestPickerCachedEmitUnder20ms(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	snapshot := pickerSnapshot{}
	for i := 0; i < 100; i++ {
		snapshot.Panes = append(snapshot.Panes, pickerPane{ID: fmt.Sprintf("$%d", i), Name: fmt.Sprintf("foh-hev-%d", i), Workspace: "hev", Title: "Captured title", Prompt: "Most recent prompt"})
	}
	path := pickerCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writePickerSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	var samples []time.Duration
	for i := 0; i < 7; i++ {
		var out bytes.Buffer
		cmd := newPickCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--emit-items", "--workspace", "hev"})
		start := time.Now()
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
		if strings.Count(out.String(), "Captured title") != 100 {
			t.Fatal("missing synthetic sessions")
		}
	}
	// Use the median so an unrelated scheduling pause does not fail the check.
	for i := 1; i < len(samples); i++ {
		for j := i; j > 0 && samples[j] < samples[j-1]; j-- {
			samples[j], samples[j-1] = samples[j-1], samples[j]
		}
	}
	median := samples[len(samples)/2]
	t.Logf("cached Cobra invocation median: %s", median)
	if median > 20*time.Millisecond {
		t.Fatalf("cached item render took %s, want <= 20ms", median)
	}
}

func TestPickerWorkspaceScope(t *testing.T) {
	cfg := workspace.Config{Workspaces: map[string]workspace.Workspace{"hev": {}, "bcc": {}}}
	snapshot := pickerSnapshot{Panes: []pickerPane{{ID: "$1", Name: "foh-hev", Workspace: "hev"}, {ID: "$2", Name: "foh-bcc", Workspace: "bcc", Title: "Archive title"}}}
	var out bytes.Buffer
	emitPickerItems(&out, cfg, pickOptions{workspace: "bcc"}, snapshot, false)
	for _, want := range []string{"bcc · new session", "archive", "foh-bcc · Archive title"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"\nshell\n", "foh-hev"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("unexpected %q", unwanted)
		}
	}
}
