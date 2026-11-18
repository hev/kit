package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

// TestFilesNoHardLinksKilledWriter kills a real writer process on the
// no-hard-link path after its post is written and synced but before it is
// published, while it holds the board lock. The final number must not exist, nothing is listed, and the next
// post takes the number (the lock dies with the writer).
func TestFilesNoHardLinksKilledWriter(t *testing.T) {
	if dir := os.Getenv("BOARD_KILLED_DIR"); dir != "" {
		killedWriter(t, dir)
		return
	}
	dir := filepath.Join(t.TempDir(), "board")
	s, err := NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestFilesNoHardLinksKilledWriter$")
	cmd.Env = append(os.Environ(), "BOARD_KILLED_DIR="+dir, "BOARD_KILLED_READY="+ready)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			cmd.Wait()
			t.Fatal("writer never reached publication")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	final := filepath.Join(dir, "threads", "1.json")
	if _, err := os.Lstat(final); !os.IsNotExist(err) {
		t.Fatalf("killed writer left a final post file: %v", err)
	}
	ctx := context.Background()
	if ts, err := s.Threads(ctx, ListOpts{}); err != nil || len(ts) != 0 {
		t.Fatalf("killed post listed: %+v %v", ts, err)
	}
	noHardLinks(t)
	th, err := s.CreateThread(ctx, NewThread{Title: "after", Body: "b", Poster: Poster{Author: "a"}})
	if err != nil || th.ID != 1 {
		t.Fatalf("next post: %+v %v", th, err)
	}
	if got, err := s.Thread(ctx, 1); err != nil || got.Title != "after" {
		t.Fatalf("read back: %+v %v", got, err)
	}
}

// killedWriter posts on the no-hard-link path and stops just before
// publishing, after signalling the parent, which kills it there.
func killedWriter(t *testing.T, dir string) {
	noHardLinks(t)
	s, err := NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	beforePublish = func(path string) {
		if err := os.WriteFile(os.Getenv("BOARD_KILLED_READY"), []byte(path), 0o600); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(time.Hour)
	}
	s.CreateThread(context.Background(), NewThread{Title: "killed", Body: "never published", Poster: Poster{Author: "a"}})
	t.Fatal("writer was not killed")
}
