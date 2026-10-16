# Opt-in native admission adapter

This separate Go module implements `nativejob.FinalStartGate` using only Factory's
exported `pkg/nativeadmission` API. Ordinary Kit builds do not require the private
Factory repository. This is source integration, not a new executable, daemon,
transport, enrollment or scheduler.

Use the exact accepted facade dependency in go.mod. Build with existing repository
access (`GOPRIVATE=github.com/hev/factory` scoped to the command); no credentials
are supplied by Kit. Private CI can replace Factory with its checked-out accepted
source. Never put a local replace or new credential into the maintained module.

Before native execution, query the original Pending job and prepare
`nativejob.StartCheck{Version:nativejob.StartVersion,JobID:job.ID,Request:job.Request}`.
Create authenticated `Provenance` from the original successfully installed native
Fence (including original expiry), workspace/parent enrollment and a durable
owner journal URI. Missing original expiry is a hold; neither renewed chef leases
nor an untrusted notification can supply it.

`MapCheck` verifies the original native RequestDigest and explicitly copies the
neutral DTO fields. Use that Check for the facade's separately reviewed
`DecideSpawn`. Construct this adapter with `New(facade, spawnIntent, provenance)`
and pass it to `Store.ExecuteWithGate`. Do not wrap Execute/Wait in the facade.

The facade acquires current chef, business and maintenance guards. This adapter
maps the exact receipt, invokes the existing native callback synchronously once,
and returns a digest/decision/revision-bound Started reference only after the
callback committed actual spawn and Running. It validates the returned Started,
propagates callback errors and blocks duplicate/late callbacks. It makes no native
Store calls inside the gate. Process Wait occurs outside every authority guard.

`JournalReference` is an authenticated owner's URI for the actual durable native
journal; fragment fields identify the exact retained job/start receipt. It is
not a bearer permit or proof the child is still alive. Birth may be unobserved
for very fast children. Native result/outbox/ACK remain authoritative for outcome;
Started is not completion, customer acceptance or a network supervisor protocol.

Original identity/intent/attempt/RequestDigest/StartAuthority never change.
Expired/transferred original fences remain unsupported; no native-dispatch/v1
adoption is implemented. A pre-claim refusal may retain Pending; refusal after
ownership commits remains Uncertain and cannot be reset/replayed.

Exact Linux and actual Darwin composite acceptance must prove the maintained
adapter with the accepted facade and native journal: real spawn/receipt, parent
cordon before spawn, guards released while child lives, current/old expiry and
fence failures, crash/ACK/reopen recovery and effect count one. Tests with a fake
facade only check mapping/fault handling. Windows remains separately disabled.
