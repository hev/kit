package layer

import (
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/trace"
	"os"
	"path/filepath"
	"testing"
)

func TestDispatchAttributionOnChunksAndSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(dispatch.Root(), "example")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "order.md"), []byte("---\nname: example\nrepo: project\nissue: TASK-42\n---\n## Acceptance\n- done\n"), 0600); e != nil {
		t.Fatal(e)
	}
	turn := trace.Turn{SessionID: "session", Workdir: filepath.Join(dir, "wt"), Blocks: []trace.Block{{Type: "text", Text: "hello"}}}
	chunk := RowOf(trace.Chunks(turn)[0])
	sessions := trace.Sessions([]trace.Turn{turn}, nil, "")
	if chunk.Order != "example" || chunk.Issue != "TASK-42" || len(sessions) != 1 || sessions[0].Order != "example" || sessions[0].Issue != "TASK-42" {
		t.Fatalf("chunk=%+v sessions=%+v", chunk, sessions)
	}
}
