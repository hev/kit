package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/trace"
)

// TestRunIntoFileArchive captures a Claude transcript into the file archive:
// redaction runs before the write, the chunks, blocks and session land, and
// a fresh archive has no historical redaction migration pending.
func TestRunIntoFileArchive(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	root := t.TempDir()
	secret := "sk-ant-api03-" + strings.Repeat("B", 93) + "AA"
	lines := `{"type":"user","uuid":"u1","sessionId":"0a1b2c3d-1111-2222-3333-444455556666","timestamp":"2026-11-01T10:00:00Z","cwd":"/w","message":{"role":"user","content":"deploy with ` + secret + `"}}` + "\n" +
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"0a1b2c3d-1111-2222-3333-444455556666","timestamp":"2026-11-01T10:00:05Z","cwd":"/w","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"deployed to staging"}],"usage":{"input_tokens":5,"output_tokens":7}}}` + "\n"
	if err := os.MkdirAll(filepath.Join(root, "-w"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "-w", "0a1b2c3d-1111-2222-3333-444455556666.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	st := &State{Units: map[string]string{}}
	src := &trace.ClaudeSource{Root: root}
	rep, err := Run(src, store, st, Options{RepoURL: func(string) string { return "" }})
	if err != nil || len(rep.Errors) != 0 || rep.UnitsIndexed != 1 || rep.MigrationPending {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	sessions, err := store.ListSessionRows(-1, nil)
	if err != nil || len(sessions) != 1 || sessions[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	blocks, err := store.ListBlockRows(sessions[0].ID)
	if err != nil || len(blocks) != 2 {
		t.Fatalf("blocks = %+v, %v", blocks, err)
	}
	chunks, err := store.SessionRows("0a1b2c3d")
	if err != nil || len(chunks) == 0 {
		t.Fatalf("chunks = %+v, %v", chunks, err)
	}
	filepath.Walk(store.Dir(), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(path); strings.Contains(string(b), secret) {
				t.Errorf("unredacted secret in %s", path)
			}
		}
		return nil
	})
	// A second run with nothing changed writes nothing.
	again, err := Run(src, store, st, Options{RepoURL: func(string) string { return "" }})
	if err != nil || again.UnitsSkipped != 1 || again.UnitsIndexed != 0 {
		t.Fatalf("second run = %+v, %v", again, err)
	}
}
