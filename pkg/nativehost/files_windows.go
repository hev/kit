//go:build windows

package nativehost

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
)

func platformPrivate(path string, directory bool) error {
	if !canonical(path) {
		return ErrHeld
	}
	return fs.VerifyPrivate(path, true, directory)
}
func openPrivate(path string) (*os.File, error) {
	if !canonical(path) {
		return nil, ErrHeld
	}
	return fs.OpenExisting(path, false)
}
func openPinned(path string, executable bool) (*os.File, error) {
	if !canonical(path) {
		return nil, ErrHeld
	}
	f, err := fs.OpenTrusted(path, false)
	if err != nil {
		return nil, err
	}
	if executable {
		// The fixed Windows supervisor is a native executable. Do not silently
		// interpret a cmd/PowerShell/shebang script through another argv boundary.
		var magic [2]byte
		_, readErr := io.ReadFull(f, magic[:])
		if readErr != nil || magic != [2]byte{'M', 'Z'} || !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil, errors.Join(ErrHeld, readErr, f.Close())
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return nil, errors.Join(err, f.Close())
		}
	}
	return f, nil
}
func platformPinned(path string, executable bool) error {
	f, err := openPinned(path, executable)
	if err != nil {
		return err
	}
	return f.Close()
}
func trustedDirectory(path string) error {
	if !canonical(path) {
		return ErrHeld
	}
	f, err := fs.OpenTrusted(path, true)
	if err != nil {
		return err
	}
	return f.Close()
}
func platformRuntime(r Runtime) error {
	if err := trustedDirectory(r.Directory); err != nil {
		return err
	}
	allowed := map[string]bool{"HOME": true, "USERPROFILE": true, "PATH": true, "SYSTEMROOT": true, "WINDIR": true, "APPDATA": true, "LOCALAPPDATA": true, "TEMP": true, "TMP": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "NODE_EXTRA_CA_CERTS": true, "SSL_CERT_FILE": true}
	dirs := map[string]bool{"HOME": true, "USERPROFILE": true, "SYSTEMROOT": true, "WINDIR": true, "APPDATA": true, "LOCALAPPDATA": true, "TEMP": true, "TMP": true, "TMPDIR": true}
	seen := map[string]string{}
	for _, entry := range r.Environment {
		key, value, ok := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if !ok || !allowed[upper] || seen[upper] != "" || value == "" || strings.ContainsRune(entry, 0) || (upper == "PATH" && key != "PATH") {
			return ErrHeld
		}
		seen[upper] = value
		if dirs[upper] {
			if err := trustedDirectory(value); err != nil {
				return err
			}
		}
	}
	if seen["PATH"] == "" || seen["HOME"] == "" || seen["USERPROFILE"] == "" || seen["SYSTEMROOT"] == "" || !strings.EqualFold(seen["HOME"], seen["USERPROFILE"]) {
		return ErrHeld
	}
	for _, dir := range strings.Split(seen["PATH"], string(os.PathListSeparator)) {
		if err := trustedDirectory(dir); err != nil {
			return err
		}
	}
	pins := map[string]bool{}
	for _, pin := range r.Files {
		key := strings.ToLower(pin.Path)
		if pins[key] || pinned(pin.Path, pin.Digest, false) != nil {
			return ErrHeld
		}
		pins[key] = true
	}
	for _, key := range []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE"} {
		if seen[key] != "" && !pins[strings.ToLower(seen[key])] {
			return ErrHeld
		}
	}
	return nil
}
func platformSyncDir(path string) error { return fs.SyncDirectory(path) }
func platformExclusive(path string, data []byte) error {
	f, err := fs.CreatePrivateFile(path)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	return fs.SyncDirectory(filepath.Dir(path))
}
