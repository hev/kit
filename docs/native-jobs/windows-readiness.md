# Windows native readiness: source only, not eligible

Current source provides a Windows process-birth adapter and native readiness
tests. Windows journal execution is explicitly disabled before any filesystem
mutation. A process-birth proof is not a Windows executor acceptance receipt.

The adapter queries a kernel boot GUID and process creation FILETIME, checks a
live process handle, and rejects missing identity or API failure. It never falls
back to PID alone or an estimated boot timestamp. The GUID buffer uses the
SystemBootEnvironmentInformation layout through pinned golang.org/x/sys. This
native API and layout need validation on the actual guest build: Microsoft says
[NtQuerySystemInformation](https://learn.microsoft.com/en-us/windows/win32/api/winternl/nf-winternl-ntquerysysteminformation)
is internal and subject to change. Creation time comes from
[GetProcessTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getprocesstimes).
An unavailable query holds ownership; it cannot establish a safe retry.

On Linux, produce native test artifacts without executing them:

```sh
GOOS=windows GOARCH=amd64 go test -c -o nativejob-windows-amd64.test.exe ./pkg/nativejob
GOOS=windows GOARCH=arm64 go test -c -o nativejob-windows-arm64.test.exe ./pkg/nativejob
```

On an explicitly owned Windows guest, verify source SHA and artifact hash, then
run the artifact matching the actual architecture in isolated scratch:

```powershell
Get-ComputerInfo | Select-Object WindowsProductName, WindowsVersion, OsBuildNumber, OsArchitecture
Get-FileHash .\nativejob-windows-amd64.test.exe -Algorithm SHA256
.\nativejob-windows-amd64.test.exe -test.v -test.run '^TestWindows(Birth|Journal)'
```

The proof exercises self birth stability, actual own child birth, mismatched
creation/boot identity, termination of only that child, and fail-closed journal
Open with no root creation. It does NOT prove durable job execution/ACK, and
cannot make a Node eligible. Preserve exact output, OS build, architecture,
service identity and source/artifact hashes. No new host access is created here.

Remaining source and platform acceptance obligations:

- Implement private owner-only DACL checks/creation; Unix mode 0700 is not a
  Windows security boundary. Avoid sharing personal or customer logged-in tokens.
- Implement and verify SQLite FULL/WAL plus file and directory-entry durability on
  the actual filesystem/volume/service account. Never swallow an unsupported
  directory flush or infer durability from cross-compilation.
- The unified hev CLI now has a Windows observer build in CI. The optional native
  receiver module remains a separate dependency: its pinned Factory admission
  implementation imports Unix-only chef file locks and has deliberately disabled
  Windows business/maintenance lock helpers. Those locks, safe file opens,
  config DACLs and retained supervisor lifecycle need native implementations and
  proof before Windows receiver admission. No whole-CLI fallback or parallel
  executor is implied by the SDK cross-build.
- Enable journal execution only after those implementations and independent native
  acceptance. Prove the actual guarded process, Pending-then-cordon no-effect,
  gate/spawn race, supervisor crash and own live child, process-birth mismatch,
  result/outbox/ACK loss and reopen, and effect count one on that exact candidate.
- Exercise reboot and guest restore/reclone with fresh authority. A VM snapshot
  can resurrect boot ID, process memory and an old journal together: local boot
  identity is not an anti-rollback oracle. Re-enrollment/incarnation and current
  admission generation/attempt authority must reconcile externally before any
  effect. Do not treat copied journals or checkpoints as effect rollback.

The actual host/guest, service identity, authorized route and independent native
executor are readiness dependencies owned by the rollout coordinator. No Windows
host/VM is assumed available, provisioned or activated by this source change.
Existing Linux trace capture and native-jobs/v1 remain on their current owner.

## Filesystem proof candidate (journal still disabled)

The Windows-only `filesystem_windows.go` helpers are a staged SDK implementation,
not an executor enablement. `Open` still returns before filesystem mutation.
Native helper tests run only in a new temporary directory under the current
Windows account; they do not change an existing enrollment, services, privileges
or credentials.

The private-directory descriptor selects the current token user as owner and
uses a protected DACL with one effective full-access allow ACE for that user,
inherited by files and directories. Existing objects are verified, never repaired.
Private artifacts are created exclusively with an explicit descriptor. SQLite's
DB, WAL and SHM inherit the private root ACL and their actual owner/effective ACL
are checked while WAL is active. NULL, empty, foreign, extra, inherit-only,
insufficient and non-inheritable permissions are not accepted by this narrow
contract. This does not restrict a separate process holding the same account's
full authority, or Windows administrative recovery privileges.

Only fixed local NTFS volumes with persistent ACLs are candidates. UNC/device
paths, alternate streams, observed reparse components and multiply linked files
are rejected. Enrollment must place the root under an owner-controlled parent;
these path checks do not create a sandbox against an authorized process swapping
that parent concurrently.

Run the exact candidate artifact with its recorded SHA:

```powershell
.\nativejob-windows-amd64.test.exe -test.v -test.timeout 60s -test.run '^TestWindows'
```

`TestWindowsFilesystemPrivacy` checks effective root/artifact/SQLite DB/WAL/SHM
privacy, exclusive creation and database reopen. `RejectsUnsafeSecurity` supplies
negative DACL, hard-link and path cases. `DirectoryDurability` requires a writable
NTFS directory handle and successful `FlushFileBuffers` on the directory and
parent. `AtomicReplacement` tests same-directory private temporary-file flush,
replacement, destination flush and directory barrier. A returned error is a
readiness failure, including an error after the replacement already took effect;
callers must reconcile before retry. These tests do not silently skip unsupported
barriers.

The replacement helper uses `MoveFileExW` without cross-volume copy and requires
additional file/directory barriers. Microsoft's
[MoveFileEx documentation](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw)
describes its write-through guarantee in terms of copy/delete; that flag alone
is not the journal's metadata durability proof.

There is also a separate test-only operation probe for NTFS write-through
creation and handle-based rename. Microsoft's
[CreateFile documentation](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
describes flushing metadata changes resulting from a write-through request,
including rename. This is stronger evidence for a rename issued on that handle
than reopening a file with write-through after a plain rename.
[NtCreateFile](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/nf-ntifs-ntcreatefile)
permits `FILE_WRITE_THROUGH` with `FILE_DIRECTORY_FILE`. The probe creates a
private directory with those options, then uses
[SetFileInformationByHandle](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-setfileinformationbyhandle)
to rename a file on its write-through handle and calls
[FlushFileBuffers](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers)
on the same handle. This is an alternative operation proof, not a fallback
that suppresses a failed directory barrier. It is not integrated into journal
execution until the exact operations and documented guarantees are accepted.
No volume-wide administrative flush or privilege adjustment is attempted.

Successful API calls prove availability under that actor/volume, not physical
power-cut persistence, universal rename atomicity, or VM snapshot anti-rollback.
Full native gate/receipt/birth, completion/outbox/ACK and real process-crash
reopen/no-replay proof on the integrated candidate remain required. The generic
Linux/Darwin shell tests cannot stand in for those Windows proofs.
