package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// TestFilesConcurrentProcesses runs real separate processes against one
// board directory: each opens threads and replies to a shared one.
func TestFilesConcurrentProcesses(t *testing.T) {
	if dir := os.Getenv("BOARD_WRITER_DIR"); dir != "" {
		writer(t, dir)
		return
	}
	dir := filepath.Join(t.TempDir(), "board")
	s, err := NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seed, err := s.CreateThread(ctx, NewThread{Board: "floor", Title: "shared", Body: "reply here", Poster: Poster{Author: "seed"}})
	if err != nil {
		t.Fatal(err)
	}
	const procs, each = 4, 15
	var cmds []*exec.Cmd
	for p := 0; p < procs; p++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestFilesConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "BOARD_WRITER_DIR="+dir, "BOARD_WRITER_ID="+strconv.Itoa(p), "BOARD_WRITER_N="+strconv.Itoa(each))
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer: %v", err)
		}
	}
	ts, err := s.Threads(ctx, ListOpts{Limit: 1000})
	if err != nil || len(ts) != procs*each+1 {
		t.Fatalf("threads %d, %v", len(ts), err)
	}
	posts, err := s.Posts(ctx, seed.ID)
	if err != nil || len(posts) != procs*each+1 {
		t.Fatalf("replies %d, %v", len(posts), err)
	}
	seen := map[string]bool{}
	for i, p := range posts {
		if p.N != i+1 || seen[p.Body] {
			t.Fatalf("post %d: n=%d body=%q", i, p.N, p.Body)
		}
		seen[p.Body] = true
	}
}

func writer(t *testing.T, dir string) {
	s, err := NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, n := os.Getenv("BOARD_WRITER_ID"), 0
	fmt.Sscan(os.Getenv("BOARD_WRITER_N"), &n)
	ctx := context.Background()
	who := Poster{Author: "proc-" + id}
	for i := 0; i < n; i++ {
		if _, err := s.CreateThread(ctx, NewThread{Board: "floor", Title: fmt.Sprintf("%s-%d", id, i), Body: "b", Poster: who}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Reply(ctx, NewPost{ThreadID: 1, Body: fmt.Sprintf("%s-%d", id, i), Poster: who}); err != nil {
			t.Fatal(err)
		}
	}
}
