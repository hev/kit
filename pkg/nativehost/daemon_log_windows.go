//go:build windows

package nativehost

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	"os"
	"path/filepath"
)

func platformDaemonLog(root string) (*os.File, error) {
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return nil, e
	}
	f, e := fs.CreatePrivateFile(filepath.Join(root, "hevd-"+hex.EncodeToString(nonce[:])+".log"))
	if e != nil {
		return nil, e
	}
	if e = errors.Join(f.Sync(), fs.SyncDirectory(root)); e != nil {
		return nil, errors.Join(e, f.Close())
	}
	return f, nil
}
