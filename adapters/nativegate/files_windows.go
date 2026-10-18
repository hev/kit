//go:build windows

package nativegate

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
)

func openPrivate(path string) (*os.File, error) { return fs.OpenExisting(path, false) }
func openPinned(path string, executable bool) (*os.File, error) {
	f, err := fs.OpenTrusted(path, false)
	if err != nil {
		return nil, err
	}
	if executable {
		var magic [2]byte
		_, readErr := io.ReadFull(f, magic[:])
		if readErr != nil || magic != [2]byte{'M', 'Z'} || !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil, errors.Join(ErrReceiverCall, readErr, f.Close())
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	return f, nil
}
func platformPrivateDirectory(path string) error { return fs.VerifyPrivate(path, true, true) }
func platformCreatePrivateDirectory(path string) error {
	if err := fs.CreatePrivateDir(path, true); err != nil {
		return err
	}
	return errors.Join(fs.SyncDirectory(path), fs.SyncDirectory(filepath.Dir(path)))
}
func platformSyncDirectory(path string) error                 { return fs.SyncDirectory(path) }
func platformPublishExclusive(path string, data []byte) error { return fs.PublishExclusive(path, data) }
