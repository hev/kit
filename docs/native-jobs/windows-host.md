# Windows host adapter controlled candidate

The target is the same registered-hevd Profile/Dispatch, original native request,
DB/business/maintenance final spawn gate, Ready, owned Wait, completion and exact
historical ACK. Windows hooks do not introduce an alternate controller, command
selection, scheduler, admission token or a PowerShell policy workaround.

`pkg/nativefs/windowsfs` shares the existing journal security implementation with
nativejob. Private state uses the explicit current-token-user owner and protected
owner-only DACL, local NTFS, no observed ancestor reparse points and no file hard
links. Existing state is checked before mutation and never repaired. OpenExisting
returns the same checked handle that consumers read. AcquireExisting locks that
same existing handle and never creates a missing lock. Lock.Publish retains the
ownership exclusion through temp-file durability, a final ownership/expiry check
immediately before replacement, target flush and directory metadata flush. Errors
after replacement require reconciliation, not blind replay.

Trusted runtime files are a different policy from private state: existing OS
ownership/writes by the selected user, SYSTEM, local Administrators or
TrustedInstaller are allowed; effective untrusted writers, unknown allow ACE
forms, absent DACLs and reparse paths hold. This provisions no accounts or rights.
A PATH directory itself must forbid untrusted additions; an ancestor's ability
to add a new subdirectory does not authorize replacing the already checked child.
The actual handle used for hashing/reading must satisfy the policy.

The private local pipe namespace binds the current SID and canonical config
path hashes. Creation requires FIRST_PIPE_INSTANCE and a protected owner DACL;
remote clients are rejected. Clients verify the actual server process token SID
and pipe DACL before sending. The server checks kernel client identity and an
identification-only impersonation token on a locked OS thread, reverting before
payload processing. One bounded message contains one existing JSON request; one
message contains `accepted\n`. This is an OS transport frame, not a wire DTO
change. Unknown listeners and extra queued request messages hold. Future messages
are never dispatched on the same connection. Deadlines/close cancel and drain the
owned overlapped operations before their buffers/handles are released.

Fixed receiver creation uses the original CreateProcess process and primary
thread handles. PROC_THREAD_ATTRIBUTE_JOB_LIST establishes membership during
CREATE_SUSPENDED creation; the original primary thread is resumed only afterward.
The unnamed owner-private noninherited Job Object has KILL_ON_JOB_CLOSE and no
breakaway flags. Only explicit NUL stdio handles are inherited. Root exit is not
tree exit: the retained job is queried and any surviving owned descendants are
terminated and observed empty before local transport-slot release. Unknown
cleanup returns ErrHeld with retained original handles and keeps the local slot
charged. This is not coding success, original completion/ACK or business capacity
settlement. Host death closes its last job handles; process-crash/restart proof
and terminal-producer integration remain mandatory before managed activation.

This source requires Windows 10+/Server 2016+ process job-list attributes. An
unsupported API, job-assignment restriction or identity query fails closed.
See Microsoft's [UpdateProcThreadAttribute](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute),
[Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
and [named pipe client](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-client)
contracts. API success is not physical power-cut or VM rollback acceptance.

The controlled candidate permits production Open on native-proved amd64 kernel
10.0.26300 and nativehost only in a nonzero current user session. Other builds,
architectures and Session0 remain held. Every original checked filesystem,
identity, Job and admission boundary remains in force.
Cross-builds and isolated helper tests do not establish actual managed lifecycle
or model-backed coding capability. The standalone cohort attempt is separate:
its Codex process exited zero but its sandbox/tool runner failed, produced no
code or commit, and failed the owner Go test. No native ACK or successful latency
sample follows from that exit status. Do not downgrade sandbox or system policy
to obtain a passing sample.

The later standalone recovery in the existing selected user's interactive session
produced Double and passing Go tests. Its reviewed commit was made by the owner
after the restricted agent's Git operation was denied. This is separate from the
failed original cohort and establishes no managed native ACK. A Windows host
rejects Session0 before creating endpoint ownership state. Configuration checking
and clients remain usable from Session0. A nonzero server session alone does not
establish readiness: the maintained user-host lifecycle and exact runtime still
need actor acceptance. No per-job scheduled-task fallback is implemented.
