# Windows native readiness: source only, not eligible

Current source provides an unverified Windows process-birth adapter and native
readiness tests; Windows journal execution is explicitly disabled before any
filesystem mutation. This is not a Windows executor acceptance receipt.

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
- Make the existing full hev CLI portable. Its current Windows cross-build fails
  in Unix-only internal/redact and related host code; this package's successful
  cross-build is not a full CLI claim. No parallel/shadow hev executable is added.
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
