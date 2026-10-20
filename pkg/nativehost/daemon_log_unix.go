//go:build linux || darwin

package nativehost

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func platformDaemonLog(root string) (*os.File, error) {
	fd, e := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return nil, e
	}
	name := "hevd-" + hex.EncodeToString(nonce[:]) + ".log"
	out, e := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(out), root+"/"+name)
	if e = errors.Join(f.Sync(), unix.Fsync(fd)); e != nil {
		return nil, errors.Join(e, f.Close())
	}
	return f, nil
}
