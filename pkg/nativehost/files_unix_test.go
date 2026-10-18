//go:build linux || darwin

package nativehost

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckedReadRetainsOpenedObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := openPrivate(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	replacement := filepath.Join(dir, "replacement")
	if e = os.WriteFile(replacement, []byte("replacement"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(replacement, path); e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(f)
	if e != nil || string(b) != "original" {
		t.Fatalf("checked handle changed object: %q %v", b, e)
	}
}
func TestCheckedReadRejectsAliasesAndUnsafeModes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	alias := filepath.Join(dir, "alias")
	if e := os.WriteFile(target, []byte("private"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, alias); e != nil {
		t.Fatal(e)
	}
	if f, e := openPrivate(alias); e == nil {
		f.Close()
		t.Fatal("private symlink accepted")
	}
	if f, e := openPinned(alias, false); e == nil {
		f.Close()
		t.Fatal("managed pin symlink accepted")
	}
	if e := os.Chmod(target, 0644); e != nil {
		t.Fatal(e)
	}
	if f, e := openPrivate(target); e == nil {
		f.Close()
		t.Fatal("public config accepted")
	}
	if f, e := openPinned(target, true); e == nil {
		f.Close()
		t.Fatal("nonexecutable accepted")
	}
}
