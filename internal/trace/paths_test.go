package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsTranscriptPaths(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`C:\Users\hev\.claude\projects\C--Users-hev-x\s.jsonl`, `C:\Users\hev\x`},
		{`C:/Users/hev/.claude/projects/C--Users-hev-x/s.jsonl`, `C:\Users\hev\x`},
		{`/home/hev/.claude/projects/-home-hev-x/s.jsonl`, `-home-hev-x`},
	} {
		if got := workdirOf(tc.input); got != tc.want {
			t.Errorf("workdirOf(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

}

func TestWindowsHomeTranscriptDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct {
		root     string
		relative string
		body     string
		source   func(string) Source
	}{
		{DefaultClaudeRoot(), filepath.Join("C--Users-hev-x", "session.jsonl"), `{"type":"user","uuid":"u","sessionId":"s","cwd":"C:\\Users\\hev\\x","message":{"role":"user","content":"hello"}}`, func(root string) Source { return &ClaudeSource{Root: root} }},
		{DefaultCodexRoot(), filepath.Join("2026", "10", "22", "session.jsonl"), "{\"type\":\"session_meta\",\"payload\":{\"id\":\"s\",\"cwd\":\"C:\\\\Users\\\\hev\\\\x\"}}\n" + `{"type":"event_msg","payload":{"type":"user_message","message":"hello"}}`, func(root string) Source { return &CodexSource{Root: root} }},
	} {
		path := filepath.Join(tc.root, tc.relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(tc.body+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		source := tc.source(tc.root)
		units, err := source.Units()
		if err != nil || len(units) != 1 {
			t.Fatalf("discovery: %v %v", units, err)
		}
		turns, err := source.Read(units[0])
		if err != nil || len(turns) != 1 || turns[0].Workdir != `C:\Users\hev\x` {
			t.Fatalf("Windows workdir: %+v %v", turns, err)
		}
	}
}
