package platformops

import (
	"os"
	"path/filepath"
	"runtime"
)

// SecureFile and SecureDir preserve Unix modes and use protected private DACLs
// on Windows. Call before writing sensitive bytes, including temporary files.
func SecureFile(path string) error { return securePath(path, 0600) }
func SecureDir(path string) error  { return securePath(path, 0700) }
func MkdirAll(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return SecureDir(path)
	}
	return nil
}
func OpenFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	if runtime.GOOS != "windows" {
		return os.OpenFile(path, flags, mode)
	}
	// Do not truncate until the existing file has been secured.
	f, err := os.OpenFile(path, flags&^os.O_TRUNC, mode)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		err = SecureFile(path)
	}
	if err == nil && flags&os.O_TRUNC != 0 {
		err = f.Truncate(0)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func WriteFile(path string, data []byte, mode os.FileMode) error {
	f, err := OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func CreateTemp(dir, pattern string) (*os.File, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		err = SecureFile(f.Name())
	}
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	return f, nil
}

// WriteFileAtomic preserves the previous complete version on write/flush or
// replacement failure. The temporary file lives on the destination filesystem.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := CreateTemp(filepath.Dir(path), ".hev-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return SyncDir(dir)
}
