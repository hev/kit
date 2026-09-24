package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInstallPathsRequiresBothPaths(t *testing.T) {
	if _, _, err := resolveInstallPaths("", "dest"); err == nil {
		t.Fatal("resolveInstallPaths should require src")
	}
	if _, _, err := resolveInstallPaths("src", ""); err == nil {
		t.Fatal("resolveInstallPaths should require dest")
	}
}

func TestResolveInstallPathsCleansAbsolutePaths(t *testing.T) {
	src, dest, err := resolveInstallPaths("bin/../bin/hev", "prefix/../prefix/bin/hev")
	if err != nil {
		t.Fatalf("resolveInstallPaths: %v", err)
	}
	if !filepath.IsAbs(src) || filepath.Base(src) != "hev" {
		t.Fatalf("src path not absolute/clean: %q", src)
	}
	if !filepath.IsAbs(dest) || filepath.Base(dest) != "hev" {
		t.Fatalf("dest path not absolute/clean: %q", dest)
	}
}

func TestInstallBuiltBinaryReplacesWritableStaleBinary(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "new-hev")
	dest := filepath.Join(tmp, "bin", "hev")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installBuiltBinary(src, dest); err != nil {
		t.Fatalf("installBuiltBinary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("dest contents = %q, want new", got)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("dest mode = %o, want 755", st.Mode().Perm())
	}
}

func TestIsPermissionErrorDetectsWrappedPermission(t *testing.T) {
	err := &os.PathError{Op: "open", Path: "/usr/local/bin/hev", Err: os.ErrPermission}
	if !isPermissionError(err) {
		t.Fatalf("isPermissionError(%v) = false, want true", err)
	}
	if isPermissionError(errors.New("different failure")) {
		t.Fatal("isPermissionError returned true for unrelated error")
	}
}
