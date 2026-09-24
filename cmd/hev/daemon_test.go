package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePlistIndexesOnly(t *testing.T) {
	plist := generatePlist("/tmp/bin/hev", "/tmp/home", "/tmp/custom & config.toml")
	for _, want := range []string{"<string>com.hev.hevd</string>", "<string>/tmp/bin/hev</string>", "<string>daemon</string>", "<string>/tmp/custom &amp; config.toml</string>"} {
		if !strings.Contains(plist, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"serve", "fluent-bit", "7890"} {
		if strings.Contains(plist, unwanted) {
			t.Fatalf("unexpected %q", unwanted)
		}
	}
}

func TestIsDaemonProcess(t *testing.T) {
	for line, want := range map[string]bool{
		"42 /Users/me/.local/bin/hev daemon": true,
		"42 hev d":                           true,
		"42 hev daemon status":               false,
		"42 hev daemon stop":                 false,
		"42 hev serve":                       false,
		"42 another daemon":                  false,
	} {
		if got := isDaemonProcess(line); got != want {
			t.Errorf("%q = %v, want %v", line, got, want)
		}
	}
}

func TestCopyFileReplacesInstalledInode(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "source"), filepath.Join(dir, "hev")
	if err := os.WriteFile(src, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	oldContent, err := io.ReadAll(old)
	if err != nil {
		t.Fatal(err)
	}
	newContent, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(oldContent) != "old binary" || string(newContent) != "new binary" {
		t.Fatalf("old=%q new=%q", oldContent, newContent)
	}
}
