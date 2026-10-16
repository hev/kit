# Final native start admission

Source extension: `native-start/v1` within the existing `native-jobs/v1`
protocol. This is opt-in source, with no shared daemon or node activation.

A controller-side reservation is insufficient: a native request can be Pending
when its node is cordoned. New managed requests bind immutable
`start_authority` in Request (optional, omitted for legacy requests). Its exact
fields are `version`, `node_uid`, `incarnation`, `admission_generation`,
`reservation_id`, `attempt_authority`. The payload digest covers the binding.
Do not reuse the chef `fence` for node generations. Existing Identity, intent
keys, duplicate/conflict behavior and unguarded request digests are unchanged.

The source API is:

```go
store.ExecuteWithGate(ctx, jobID, gate)
// gate implements:
WithStart(context.Context, nativejob.StartCheck,
    func(nativejob.StartReceipt) error) error
```

The exact versioned fields are declared in `pkg/nativejob/start_gate.go`.
StartCheck carries the complete immutable Request and native job ID. The
trusted enrolled receiver's gate adapter checks the authoritative node UID,
incarnation, generation, reservation, reviewed attempt authority and complete
Order/workflow/attempt/candidate identity. It must use the same cross-process
exclusion as cordon/drain, held throughout callback execution. A cached
observation, enqueue-time check, detached permit, controller lease alone or
arbitrary caller-supplied receipt does not meet this contract.

The gate invokes the callback synchronously exactly once with a receipt binding
job ID, payload digest, the exact authority, decision ID and authority revision.
The callback FULL-sync commits this receipt while Uncertain, then rechecks native
ownership, chef fence and context, starts the actual process, persists birth and
commits Running before returning to the gate. It does not wait for child exit
while holding the maintenance lock. Cordon wins first and denies the start, or
start wins first and cordon observes a started/uncertain reservation to drain.
Loss of either journal/authority never proves absence or permits replay.

Plain Execute and the existing wire `execute` action reject a guarded job without
a gate. No wire action accepts a receipt as launch authorization. A gateway must
embed the trusted gate implementation; the stock one-call CLI cannot activate
guarded jobs. Unguarded legacy requests remain possible only in legacy isolated
lanes. Managed admission must require start_authority and select this receiver
path; labels or a controller-only guard cannot enforce that requirement.

The job and durable outbox event retain `start_receipt` when one was granted.
Guarded event ID also binds its receipt digest. Result and ACK semantics remain
immutable/idempotent; a historical receipt or ACK cannot authorize another start.
A crash after ownership or receipt persistence remains Uncertain, including the
zero-effect window before spawn. Reconcile records inspection; it cannot clear
admission or reset the job to Pending. A fresh effectful attempt requires a new
reviewed identity/reservation/key.

The first committed guarded request marks the journal capability in metadata.
New readers accept legacy and guarded formats; old readers fail Open rather than
silently ignoring the new field. Rollout MUST drain/stop old writers before new
guarded admission: an old already-open process cannot be retroactively fixed by
a metadata marker. Rollback retains compatible readers, results/outbox and all
uncertainty; never downgrade or copy a journal as proof of fence transfer.

Windows: package cross-compilation is distinct from native readiness. The current
full `cmd/hev` Windows build is blocked by Unix-only redaction/daemon code outside
this package. Windows process birth, secure directory ownership, filesystem
flush behavior, native job/restart/ACK and VM snapshot/incarnation reconciliation
need platform-specific source and actual independently recorded Windows proof.
No Windows hardware, guest, credentials, enrollment or activation is implied.
