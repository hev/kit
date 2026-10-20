//go:build windows

package windowsfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWindowsExclusiveCompleteAndNeverReplace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := CreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ready.json")
	const writers = 8
	var wg sync.WaitGroup
	results := make(chan error, writers)
	payloads := make(map[string]bool)
	for i := 0; i < writers; i++ {
		payload := []byte(fmt.Sprintf("{\"writer\":%d,\"body\":\"%s\"}\n", i, bytes.Repeat([]byte("x"), 64000)))
		payloads[string(payload)] = true
		wg.Add(1)
		go func(data []byte) { defer wg.Done(); results <- PublishExclusive(path, data) }(payload)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful publications=%d", successes)
	}
	f, err := OpenExisting(path, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err = errors.Join(err, f.Close()); err != nil || !payloads[string(data)] {
		t.Fatal("incomplete or unrecognized publication", err)
	}
	if err = PublishExclusive(path, []byte("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing immutable evidence was not refused", err)
	}
	again, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("existing evidence changed", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary files leaked", len(entries), err)
	}
}
