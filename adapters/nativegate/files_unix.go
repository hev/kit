//go:build linux || darwin

package nativegate

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func checkedFile(path string, private, executable bool) (*os.File, error) {
	if !canonicalPath(path) {
		return nil, ErrReceiverCall
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || (private && (!ownedByCurrentUser(st) || st.Mode().Perm()&0077 != 0)) || (executable && st.Mode().Perm()&0111 == 0) {
		f.Close()
		return nil, ErrReceiverCall
	}
	return f, nil
}
func openPrivate(path string) (*os.File, error) { return checkedFile(path, true, false) }
func openPinned(path string, executable bool) (*os.File, error) {
	return checkedFile(path, false, executable)
}
func platformPrivateDirectory(path string) error {
	if !canonicalPath(path) {
		return ErrReceiverCall
	}
	s, e := os.Lstat(path)
	if e != nil || !s.IsDir() || !ownedByCurrentUser(s) || s.Mode().Perm()&0077 != 0 {
		return ErrReceiverCall
	}
	return nil
}
func platformCreatePrivateDirectory(path string) error { return os.Mkdir(path, 0700) }
func platformSyncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func platformPublishExclusive(path string, data []byte) error {
	// Publish complete bytes exclusively: readers never decode an in-progress
	// readiness payload. A surviving temporary file is never executable intent.
	f, err := os.CreateTemp(filepath.Dir(path), ".native-publish-")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(temp, path); err != nil {
		return err
	}
	return platformSyncDirectory(filepath.Dir(path))
}

// The existing journal may use0644 inside its owner-private root. Keep that
// original-journal check distinct from strict0600 config/descriptor reads.
func privateFile(path string) error {
	f, e := openPinned(path, false)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !ownedByCurrentUser(st) {
		return ErrReceiverCall
	}
	return nil
}
