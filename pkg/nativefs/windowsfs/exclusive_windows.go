//go:build windows

package windowsfs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// PublishExclusive publishes complete, flushed private bytes without replacing
// any existing name. Errors after rename may have taken effect; reconcile the
// immutable target and never replay or repair it. No cross-volume fallback is used.
func PublishExclusive(path string, data []byte) (result error) {
	if err := PathSyntax(path); err != nil {
		return err
	}
	if err := VerifyPrivate(filepath.Dir(path), true, true); err != nil {
		return err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(path), ".native-publish-"+hex.EncodeToString(nonce[:]))
	f, err := CreatePrivateFile(temp)
	if err != nil {
		return err
	}
	moved := false
	defer func() {
		if !moved {
			result = errors.Join(result, os.Remove(temp))
		}
	}()
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// Deliberately omit REPLACE_EXISTING and COPY_ALLOWED: an existing name,
	// including unsafe state, must survive unchanged.
	if err = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "publish", Old: temp, New: path, Err: err}
	}
	moved = true
	target, err := OpenExisting(path, true)
	if err != nil {
		return err
	}
	if err = errors.Join(target.Sync(), target.Close()); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(path))
}
