//go:build windows

package nativejob

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func windowsPrivateTestRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private-journal")
	if err := windowsCreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated private root: %s", root)
	return root
}

func TestWindowsFilesystemPrivacy(t *testing.T) {
	root := windowsPrivateTestRoot(t)
	if err := windowsCreatePrivateDir(root, true); err != nil {
		t.Fatal("safe reopen", err)
	}
	if err := windowsCreatePrivateDir(root, false); !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatal("exclusive directory creation", err)
	}
	output := filepath.Join(root, "job-proof")
	if err := windowsCreatePrivateDir(output, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stdout", "stderr"} {
		path := filepath.Join(output, name)
		f, err := windowsCreatePrivateFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.WriteString("private artifact\n"); err != nil {
			t.Fatal(err)
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			t.Fatal(err)
		}
		if err = windowsVerifyPrivate(path, true, false); err != nil {
			t.Fatal(name, err)
		}
		if f, err = windowsCreatePrivateFile(path); err == nil {
			f.Close()
			t.Fatal("artifact overwritten")
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "CREATE TABLE proof (value TEXT)", "INSERT INTO proof VALUES ('private')"} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	var mode string
	var sync int
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatal(mode, err)
	}
	if err = db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil || sync != 2 {
		t.Fatal(sync, err)
	}
	for _, name := range []string{"journal.sqlite", "journal.sqlite-wal", "journal.sqlite-shm"} {
		if err = windowsVerifyPrivate(filepath.Join(root, name), false, false); err != nil {
			t.Fatal(name, err)
		}
		t.Log("verified effective owner-only ACL:", name)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filepath.Join(root, "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM proof").Scan(&value); err != nil || value != "private" {
		t.Fatal(value, err)
	}
}

func TestWindowsFilesystemRejectsUnsafeSecurity(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	cases := map[string]string{
		"unprotected":     "O:" + sid + "D:(A;OICI;FA;;;" + sid + ")",
		"foreign":         "O:" + sid + "D:P(A;OICI;FA;;;WD)",
		"extra":           "O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FR;;;WD)",
		"inherit-only":    "O:" + sid + "D:P(A;OICIIO;FA;;;" + sid + ")",
		"not-inheritable": "O:" + sid + "D:P(A;;FA;;;" + sid + ")",
		"insufficient":    "O:" + sid + "D:P(A;OICI;FR;;;" + sid + ")",
	}
	for name, sddl := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			sd, err := windows.SecurityDescriptorFromString(sddl)
			if err != nil {
				t.Fatal(err)
			}
			p, err := windows.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
			if err = windows.CreateDirectory(p, &sa); err != nil {
				t.Fatal(err)
			}
			if err = windowsCreatePrivateDir(path, true); err == nil {
				t.Fatal("unsafe existing root accepted or silently repaired")
			}
		})
	}
	root := windowsPrivateTestRoot(t)
	file := filepath.Join(root, "file")
	f, err := windowsCreatePrivateFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = windowsVerifyPrivate(file, true, true); err == nil {
		t.Fatal("file accepted as directory")
	}
	link := filepath.Join(root, "linked")
	if err = os.Link(file, link); err != nil {
		t.Fatal(err)
	}
	if err = windowsVerifyPrivate(file, true, false); err == nil {
		t.Fatal("multiply linked file accepted")
	}
	for _, path := range []string{`\\server\share\journal`, `\\?\C:\journal`, `C:\journal:stream`, `relative`} {
		if err := windowsLocalPath(path); err == nil {
			t.Fatal("unsafe path accepted", path)
		}
	}
}

// This is deliberately a failing readiness test when the actual actor/filesystem
// cannot provide a directory metadata barrier. Do not change failure to Skip or
// infer native acceptance from source cross-compilation.
func TestWindowsFilesystemDirectoryDurability(t *testing.T) {
	root := windowsPrivateTestRoot(t)
	if err := windowsSyncDirectory(root); err != nil {
		t.Fatalf("Windows journal MUST remain disabled: directory metadata flush unavailable: %v", err)
	}
	if err := windowsSyncDirectory(filepath.Dir(root)); err != nil {
		t.Fatalf("Windows journal MUST remain disabled: parent metadata flush unavailable: %v", err)
	}
	t.Log("NTFS FlushFileBuffers directory and parent returned success; this is not power-cut proof")
}

func TestWindowsFilesystemAtomicReplacement(t *testing.T) {
	root := windowsPrivateTestRoot(t)
	temp, target := filepath.Join(root, "next"), filepath.Join(root, "current")
	for _, body := range []string{"first", "replacement"} {
		f, err := windowsCreatePrivateFile(temp)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.WriteString(body); err != nil {
			t.Fatal(err)
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			t.Fatal(err)
		}
		if err = windowsReplacePrivateFile(temp, target); err != nil {
			t.Fatalf("replacement/barrier failure (may already have replaced; no blind retry): %v", err)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != body {
			t.Fatal(string(data), err)
		}
		if _, err = os.Stat(temp); !os.IsNotExist(err) {
			t.Fatal("temporary name retained", err)
		}
	}
	if err := windowsReplacePrivateFile(target, filepath.Join(t.TempDir(), "elsewhere")); err == nil {
		t.Fatal("cross-directory replacement accepted")
	}
}

// Alternative NTFS operation proof, separate from production helpers. A
// write-through creation/rename request has a documented NTFS metadata contract;
// merely opening a new handle after a plain rename does not establish it.
func TestWindowsFilesystemWriteThroughRequests(t *testing.T) {
	root := windowsPrivateTestRoot(t)
	sd, err := windowsPrivateDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "write-through-directory")
	name, err := windows.NewNTUnicodeString(`\??\` + dir)
	if err != nil {
		t.Fatal(err)
	}
	oa := windows.OBJECT_ATTRIBUTES{ObjectName: name, SecurityDescriptor: sd, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, &oa, &iosb, nil, windows.FILE_ATTRIBUTE_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_WRITE_THROUGH|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		t.Fatal("NtCreateFile directory write-through request", err)
	}
	if err = errors.Join(windowsVerifyPrivateHandle(h, true, true), windows.CloseHandle(h)); err != nil {
		t.Fatal(err)
	}
	t.Log("NtCreateFile FILE_DIRECTORY_FILE|FILE_WRITE_THROUGH created private directory")
	temp, target := filepath.Join(dir, "temp"), filepath.Join(dir, "target")
	f, err := windowsCreatePrivateFile(temp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("same write-through handle\n"); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	ptr, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		t.Fatal(err)
	}
	h, err = windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_WRITE_THROUGH|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if err = windowsVerifyPrivateHandle(h, true, false); err != nil {
		t.Fatal(err)
	}
	if err = windows.FlushFileBuffers(h); err != nil {
		t.Fatal("pre-rename file flush", err)
	}
	type renameInfo struct {
		Replace uint8
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	utf16, err := windows.UTF16FromString(target)
	if err != nil {
		t.Fatal(err)
	}
	var layout renameInfo
	offset := int(unsafe.Offsetof(layout.Name))
	size := offset + 2*(len(utf16)-1)
	buf := make([]byte, size)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.Replace = 1
	info.Length = uint32(2 * (len(utf16) - 1))
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[offset])), len(utf16)-1), utf16[:len(utf16)-1])
	if err = windows.SetFileInformationByHandle(h, windows.FileRenameInfo, &buf[0], uint32(len(buf))); err != nil {
		t.Fatal("rename on original write-through handle", err)
	}
	if err = windows.FlushFileBuffers(h); err != nil {
		t.Fatal("same handle post-rename flush", err)
	}
	if err = windowsVerifyPrivate(target, true, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "same write-through handle\n" {
		t.Fatal(string(data), err)
	}
	t.Log("SetFileInformationByHandle rename + FlushFileBuffers on same FILE_FLAG_WRITE_THROUGH handle returned success; not a power-cut/atomicity receipt")
}
