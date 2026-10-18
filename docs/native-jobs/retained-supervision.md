# Retained native supervision contract

The additive `Store.ExecuteWithGateObserved(ctx, id, gate, onRunning)` SDK keeps
the original journal and process ownership. Existing ExecuteWithGate delegates
with a nil observer. The observer receives a `StartedObservation` only after
the final gate returns successfully, including its facade transaction and
business/maintenance transitions, and the native Running commit boundary passes.
It runs before Wait, outside all launch guards. Raw journal Running can precede
the facade's final commit; it must not substitute for this notification.

StartedObservation contains `job_id`, `payload_digest`, actual supervisor birth
`owner`, optional actual process birth `child`, and optional `start_receipt`.
Pointer fields are copies, not references to retained state. It contains no argv
or environment. Notification is evidence, never permission or completion.
An observer must return promptly even if readiness publication fails, so the
supervisor continues Wait, artifact sync and original result/outbox collection.
A failed gate or interrupted Running boundary emits no notification. Supervisor
death retains original recovery uncertainty; another process cannot adopt Wait.

The following retained receiver contract is agreed for implementation; its
launcher/configuration route is not an installed capability. Independent exact
source/platform acceptance and an owner-installed lifecycle remain necessary.
The existing native-receiver/v1 Inspection wire remains unchanged.

The owner selects private `ReceiverConfig.retained` with exact fields `version`
(`native-supervisor/v1`), `launcher`, `supervisor`, `state_root`, and
`workload_timeout_seconds`. Paths are canonical absolute owner-installed choices.
The workload deadline is separate from the bounded start-client deadline. No
model or stdin operation selects a launcher, executable or arbitrary arguments.
The fixed interoperability is:

```text
PINNED_LAUNCHER --config ABS_RECEIVER_CONFIG --descriptor ABS_PRIVATE_DESCRIPTOR
PINNED_HEV_NATIVE_RECEIVER --config ABS_RECEIVER_CONFIG --supervise ABS_PRIVATE_DESCRIPTOR
```

The launcher reports accepted or unknown; accepted is neither native Running
nor completion. It must start the supervisor outside caller/scheduler kill scopes.
Setting a longer timeout or detaching within a Linux service cgroup is inadequate.
Estate ownership must prove the installed lifecycle on each claimed platform.

The proposed `NativeSupervisorDescriptor` stored as `descriptor.json` has JSON
fields `version` (`native-supervisor/v1`), `config_ref`, `config_digest`,
`supervisor_ref`, `supervisor_digest`, `start_call`, `workload_timeout_seconds`,
`descriptor_digest`. `start_call` is the exact existing ReceiverCall for start,
including original job ID and DBClaim/NativeBinding, with no unused ACK fields.
There are no duplicate job/claim/binding fields outside that call. Require its
job_id to equal binding.native_job_id and its binding.request_digest to equal
the original computed digest. Config and
supervisor digests are SHA256 of exact file bytes. Descriptor digest uses the
existing SHA256 JSON encoding of the typed descriptor with its omitempty digest
field cleared. No credential values, argv or environment enter this descriptor.

Private `ready.json` uses `NativeSupervisorReady` fields `version`
(`native-supervisor-ready/v1`), `descriptor_digest`, `observation`. The retained
supervisor exclusively persists and fsyncs it after the post-gate observer.
Readiness must bind the original descriptor, receipt and kernel births. A start
reply returns unchanged Inspection Running only with verified post-gate readiness
and live owner/child, or original Completed for a fast finished child. A missing
or lost reply is inspection-only, never permission to start another supervisor.
Descriptors are exclusively created and durably bound, not scanned for replay.

The retained supervisor itself calls ExecuteWithGateObserved from before native
ownership claim and owns the child, open artifact descriptors and Wait through
completion. No transfer or extra scheduler is introduced. Original DB action,
native issuance and business/node/parent guards remain distinct and unchanged.
Native admission covers actual cmd.Start of the trusted single-use foreground
Factory entrypoint. Subsequent harness startup uses existing host admission;
this does not claim the PG gate remains held through that later spawn. Factory
must reject prepared-session legacy launch/resume/runner bypasses before and
after single-use consumption. Session creation and a clean process exit do not
establish a trustworthy coding turn or independent product acceptance.

Platform proof must include a workload longer than 45 seconds, caller/scheduler
termination with the same retained process births surviving, post-facade readiness,
completion/ACK once, lost reply/reentry and supervisor death with an uncertain
original journal. Windows remains disabled until private DACL, durability and
actual native lifecycle/restart/ACK acceptance are implemented and passed.

## Retained implementation and workload bounds

`ExecuteRetainedWithGate(ctx, id, gate, RetainedExecutionOptions{WorkloadTimeout},
onRunning)` explicitly adds a positive workload timeout of at most 24 hours.
Its independent clock targets only the root `exec.Cmd` process owned by that
supervisor. Existing ExecuteWithGate/ExecuteWithGateObserved context semantics
are unchanged: client cancellation after Running does not kill the workload.
A deadline requests termination through the owned process handle, waits for the
actual root exit, and preserves the original result/artifacts. Nonzero root exit
following the deadline termination request carries fixed evidence
`native-workload-deadline/v1`. This does not prove descendant termination, a
successful coding turn, or permission to release capacity. Factory's prepared
entrypoint owns harness timeout/cleanup/terminal receipt; the configured root
budget must leave cleanup grace beyond its turn deadline. `ProcessBirth(pid)`
is a read-only local kernel query, never adoption or kill permission.

The receiver implements the fixed `--config CFG --supervise DESC` entrypoint.
Normal start selects retained behavior only when private config contains:

```json
{
  "retained": {
    "version": "native-supervisor/v1",
    "launcher": "/absolute/pinned/launcher",
    "launcher_digest": "sha256:EXACT_LOWERCASE_FILE_HASH",
    "supervisor": "/absolute/pinned/hev-native-receiver",
    "supervisor_digest": "sha256:EXACT_LOWERCASE_FILE_HASH",
    "state_root": "/absolute/existing-private-retained-state",
    "workload_timeout_seconds": 1900
  }
}
```

Paths/digests and budget are authenticated owner choices; illustrative hashes
are not valid values. State root, descriptor directories and files must be
private and owned by the executing account. Launcher/supervisor are pinned
regular executable files without group/other write access. Config digest covers
its exact private file bytes. Descriptors use one strict start_call, reject raw
unused null/ACK fields, and verify all config/binary references. Duplicate or
changed descriptors never launch again. The launcher must return exactly
`accepted` on stdout with exit zero; every other result is uncertain. A launch
acceptance is not readiness. Launcher diagnostics are discarded and output is
bounded. Its independent lifecycle is a deployment-owner obligation, not a
fallback shell detach performed by the receiver.

The supervisor exclusively commits `supervisor.json` (descriptor digest and real
owner birth) before execution. A duplicate supervisor cannot consume it again.
It publishes readiness after the observer without blocking process Wait on
notification storage. JSON is written/fsynced in a private temporary file and
linked exclusively to its final name; readers also sync the readiness directory
before treating it as durable. Temporary/partial evidence never authorizes replay.
A fast completion may have no ready file: its original durable completion is
sufficient. Ready Running requires exact retained owner/child births and the
same original journal receipt; missing birth is not converted to PID-only proof.
Inspection in retained mode applies existing orphan recovery, holding dead-owner
Running jobs Uncertain. Supervisor interruption never adopts a surviving child.

The command's normal start-client deadline remains 1–45 seconds. The supervisor
mode has its own root workload budget and retains Wait independently. SIGKILL or
host failure can still lose supervision; the original uncertainty protocol is
required. No claim of retained executor readiness is made for Windows, and no
installed launcher is selected or activated by source tests.

Isolated tests use real native children with an injected facade/launcher receipt
fixture to verify post-gate readiness, duplicate/lost start replies, single-use
supervisor consumption, notification failure with original completion retained,
strict descriptors, binary-pin drift and owned-root deadline behavior. These do
not prove actual PostgreSQL composition, process/cgroup isolation, production
credentials, descendant cleanup or installed platform readiness. Exact independent
composite and per-platform lifecycle proofs are still required before rollout.
