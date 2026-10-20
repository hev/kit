//go:build windows

package nativegate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsReceiverCheckedFilesAndImmutableReady(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := platformCreatePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := platformPrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ready.json")
	data := []byte("{\"ready\":true}\n")
	if err := platformPublishExclusive(path, data); err != nil {
		t.Fatal(err)
	}
	if err := privateFile(path); err != nil {
		t.Fatal(err)
	}
	if err := platformPublishExclusive(path, []byte("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatal("immutable replacement accepted", err)
	}
	f, err := openPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	if err = errors.Join(err, f.Close()); err != nil || !bytes.Equal(got, data) {
		t.Fatal("historical bytes changed", err)
	}
	if err := privateFile(root); err == nil {
		t.Fatal("directory accepted as journal")
	}
	missing := filepath.Join(root, "absent")
	if err := privateFile(missing); err == nil {
		t.Fatal("missing journal accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("journal verification created state", err)
	}
	script := filepath.Join(root, "supervisor.ps1")
	if err := platformPublishExclusive(script, []byte("Write-Output forbidden")); err != nil {
		t.Fatal(err)
	}
	if f, err := openPinned(script, true); err == nil {
		f.Close()
		t.Fatal("script accepted as native supervisor")
	}
}
