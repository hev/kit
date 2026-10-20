//go:build linux || darwin

package nativehost

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// fileHash observes fixture bytes before fixtures protect writable hosted tools.
// Production pin validation hashes the checked handle in pinned().
func fileHash(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
