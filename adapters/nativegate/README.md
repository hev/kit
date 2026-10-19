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

## Database receiver command

The opt-in receiver uses the persisted database action as chef authority, reusing
business and node/parent maintenance guards without the file-chef DecideSpawn or
Apply path. DB lease/claim fences never replace original native issuance.
`DBGate.WithStart` rejects legacy receivers; `WithSpawn` passes a live checker
into the native journal after durable receipt persistence, immediately before
actual Start. See [the native boundary](../../docs/native-jobs/db-receiver.md).

Build and invoke from this optional module:

```sh
GOPRIVATE=github.com/hev/factory go build -o /absolute/hev-native-receiver ./cmd/hev-native-receiver
/absolute/hev-native-receiver --config /absolute/receiver.json < /absolute/call.json
```

The authenticated owner installs an explicit private regular configuration file
(mode 0600). It selects an existing enrolled journal and trusted provenance:

```json
{
  "version": "native-receiver/v1",
  "journal_root": "/absolute/existing-native-journal",
  "enrollment": {"host_id": "enrolled-host", "executor_id": "enrolled-executor"},
  "journal_reference": "native-journal://enrolled-host/original-store",
  "workspace": {"UID": "workspace", "Incarnation": "workspace-boot", "Reference": "workspace/node"},
  "parent": {"UID": "parent", "Incarnation": "parent-boot", "WorkspaceRef": "workspace/node"},
  "business_dir": "/absolute/existing-business-journal",
  "maintenance_dir": "/absolute/existing-maintenance-journal",
  "maintenance_owner": {"Owner": "maintenance-owner", "Fence": 1, "Expires": "2026-12-01T00:00:00Z"},
  "connection": {
    "host_ref": "op://vault/database/host",
    "database_name": "postgres",
    "user_ref": "op://vault/database/username",
    "password_ref": "op://vault/database/password"
  },
  "timeout_seconds": 30
}
```

Omit parent for a directly enrolled node. Optional `token_file` names an explicit
absolute existing account token file; no token is copied. Connection supports
`database_ref` instead of `database_name` and optional `port`, through Factory's
maintained DBReferences/DBOPResolver/OpenDB API. It requires verified TLS and
explicit references, never an ambient DSN or insecure local-socket fallback.
These labels/dates illustrate JSON shape only; use authenticated source facts.
Inspection/ACK require only version, journal_root, enrollment, timeout_seconds;
they do not resolve database credentials. Config is never accepted on stdin.

One strict JSON request (at most 64 KiB) selects one operation:

```json
{"version":"native-receiver/v1","operation":"inspect","job_id":"job-original"}
```

```json
{
  "version":"native-receiver/v1","operation":"start","job_id":"job-original",
  "claim":{"lease":"controller/order","holder":"peer-uuid","lease_fence":2,"action_id":"action-uuid","claimant":"peer-uuid","attempt":1,"claim_fence":2},
  "binding":{"order_uid":"order-uuid","generation":1,"scope_digest":"scope-digest","native_job_id":"job-original","request_digest":"sha256:original-request"}
}
```

ACK uses version/operation/job_id plus `identity` (the complete unchanged original
native Identity), `event_id`, and `result_digest` from the original completion.
All other fields must be absent. Unknown/case-aliased/duplicate/unused fields,
including unused null values, trailing data and executable payloads are rejected.
Job IDs cannot be intent-key aliases. There is no Submit, enrollment, issuance,
reconciliation or arbitrary argv/environment operation.

All successful operations return one Inspection JSON with exact keys `version`,
`job_id`, `intent_key`, `payload_digest`, `request_digest`, `identity`, `state`,
`acknowledged`, and optional `start_authority`, `start_receipt`, `completion`.
RequestDigest is recomputed from the complete original request and verified equal
to its retained PayloadDigest. Completion is the original nativejob.Event,
including StartReceipt when present. It remains available after historical ACK,
even though the outbox no longer lists it. No argv/environment is returned.
Inspection does not mutate recovery state or infer no effect from a missing
completion. Metadata is not start authority.

Start accepts only the existing Pending job. Its exact Check authority derives
from original journal/IssuedFence and trusted owner workspace/parent config.
SQL authenticates the enrolled request digest and canonical scope/generation;
business/maintenance guards verify real reservation and node/parent enrollment.
No model supplies replacement executable data or receiver configuration.

Start waits for completion outside all admission locks. This is a short-process
canary route, not a detached durable supervisor. Timeout must be 1–45 seconds;
timeout/signal exits 124, preserving the original journal for explicit recovery.
It does not kill or replay the child. Error exit 1 prints a stable redacted
message. Lost response, timeout or DB commit failure can follow actual spawn:
never blindly retry Start, reset Pending, or free capacity based on an error.
Stop invocation to roll back; retain native/DB journals, claims and evidence.

Validation from this optional module:

```sh
GOPRIVATE=github.com/hev/factory go test -race -count=1 ./...
GOPRIVATE=github.com/hev/factory go vet ./...
```

An exact-source composite uses an isolated temporary Go workspace containing Kit
root, this optional module and the nominated Factory checkout. Root Kit CI is
`ci` (jobs `go` and `windows-inventory`) and does not consume private Factory.
Independent Linux/PostgreSQL acceptance must exercise the actual DB facade/schema
with the maintained native adapter/command at the nominated source pair; injected
facade tests alone are not DB proof. Native Darwin and a real verified-TLS
production canary have separately scoped evidence.

`hev-native-receiver --config ABS --owner-proof` is an additive fixed read-only
mode, mutually exclusive with registration/supervision. Strict stdin is
`{"version":"native-owner-proof/v1","job_id":"…","request_digest":"sha256:…"}`.
Both job and digest may be omitted for enrollment authentication only; null,
duplicate, aliased and unknown fields are rejected. The request digest must equal
the authenticated original. Output embeds the exact `nativejob.OwnerProof` JSON
projection and adds `owner_birth_status` / `child_birth_status` for present saved
births, each `Alive`, `Reused`, `Gone`, or `Unknown`. Only a definite missing-process
error yields `Gone`; ambiguous exited/inspection errors yield `Unknown`. These
individual process observations do not prove an empty process tree or authorize
capacity settlement, PID adoption, replay or termination. Completed events require
recomputed original result digest. This mode uses read-only journal access and
never initializes, migrates, recovers or acknowledges it. Its context is ten
seconds, and the existing inspect/start/ACK wire remains unchanged.

The receiver's internal filesystem adapters use checked open handles for private
configuration/descriptor reads and executable hashes. Original journal checks
remain distinct: Unix owner-readable journal files may be0644 inside the private
journal root; configuration and descriptors require private file permissions.
Immutable descriptor, supervisor and readiness publication must expose complete
bytes once and refuse replacement, including under concurrent writers. Platform
adapters own directory creation and durability barriers. Unsupported adapters
remain held. These internal seams do not alter receiver DTOs, journal authority,
platform enablement or declared module pins; native platform acceptance and an
explicit exact dependency composition are required separately.

This source candidate explicitly composes Factory `2c88d1f2b647` with Kit SDK
`b4dfa9523f08`, including the Windows filesystem primitives and checked receiver
hooks. These dependency pins describe source composition, not Windows production
activation or acceptance. Controlled Windows execution still requires independent
native lifecycle, authority and completion/ACK validation.

The unified candidate includes the Darwin coalition capability observer and owned
workload boundary, Windows owned-command backend, and explicit same-daemon role
options. Factory model-runtime pins remain separate from the receiver runtime.
These exact pins require managed per-platform end-to-end acceptance; source checks
and cross-compilation do not establish a three-host delivery.

The final cohort pair adds the portable prepared caller/floor-owner slices and
sealed explicit LOW reasoning selection in Factory. The root SDK remains the
exact separately built `b4dfa9523f08` Windows artifact; this optional-module pin update
does not replace that root source identity or transfer platform acceptance.
Worker-local preparation precedes the unchanged remote start/inspect/ACK flow.

Mini and Oven accepted originals retain their recorded Root346/receiver30 pair.
This Windows composition includes the final read-only journal lease checks and
retained query-only service witness; older receiver binaries do not gain these
semantics from a separately updated root CLI.
