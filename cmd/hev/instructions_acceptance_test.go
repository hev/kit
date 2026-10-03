package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// This opt-in test writes two synthetic versions to the configured archive.
// It never scans the operator's instruction files or changes daemon config.
func TestLiveInstructionVersions(t *testing.T) {
	if os.Getenv("HEV_LIVE_INSTRUCTIONS") != "1" {
		t.Skip("explicit live archive opt-in required")
	}
	cl, err := client("")
	if err != nil {
		t.Fatal("configured archive unavailable")
	}
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "acceptance-fixture", "memory", "MEMORY.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	src := trace.InstructionSource{Home: home}
	var ids []string
	for _, text := range []string{"Synthetic acceptance memory: amber otter protocol.", "Synthetic acceptance memory: violet kestrel protocol."} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		rep, err := index.RunInstructions(src, cl)
		if err != nil || rep == nil || len(rep.Errors) != 0 || rep.UnitsIndexed != 1 {
			t.Fatal("fixture ingestion failed (inspect privately)")
		}
		files, err := src.Files()
		if err != nil || len(files) != 1 {
			t.Fatal("fixture discovery failed")
		}
		versions, err := cl.InstructionVersions(files[0].Path, layer.Hostname())
		if err != nil {
			t.Fatal("fixture metadata read failed")
		}
		current := ""
		for _, v := range versions {
			if v.ValidTo == "" {
				if current != "" {
					t.Fatal("multiple current versions")
				}
				current = v.ID
			}
		}
		if current == "" {
			t.Fatal("no current version")
		}
		ids = append(ids, current)
	}
	files, _ := src.Files()
	versions, err := cl.InstructionVersions(files[0].Path, layer.Hostname())
	if err != nil || len(versions) != 2 || ids[0] == ids[1] {
		t.Fatal("two distinct retained versions required")
	}
	var old, latest layer.InstructionVersion
	for _, v := range versions {
		if v.ID == ids[0] {
			old = v
		}
		if v.ID == ids[1] {
			latest = v
		}
	}
	if old.ValidTo == "" || old.ValidTo != latest.ValidFrom || latest.ValidTo != "" || old.ContentHash == latest.ContentHash {
		t.Fatal("invalid version boundary or hashes")
	}
	for i, phrase := range []string{"amber otter", "violet kestrel"} {
		found := false
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			hits, err := cl.SearchPhrasings([]string{phrase}, 10, layer.And([]any{"path", "Eq", files[0].Path}, []any{"version_id", "Eq", ids[i]}))
			if err != nil {
				t.Fatal("fixture search failed")
			}
			for _, hit := range hits {
				if hit.VersionID == ids[i] && hit.Host == layer.Hostname() && hit.Project == "acceptance-fixture" && strings.Contains(hit.Text, phrase) {
					found = true
				}
			}
			if found {
				break
			}
			time.Sleep(time.Second)
		}
		if !found {
			t.Fatal("retained version not independently searchable")
		}
	}
	t.Log("PASS: two searchable synthetic versions; prior valid_to equals replacement valid_from")
}
