# Final native-spawn admission facade

This internal package connects the existing chef, businessflow and maintenance
journals at an actual native launch callback. It imports no Kit code, creates no
shadow authority and chooses no ready work. Argo remains the sole workflow DAG
owner. It is opt-in source; no host enrollment, installation or cluster change is
performed.

## API for a native adapter

```go
func New(Config) (*Gate, error)
func (*Gate) DecideSpawn(context.Context, Lease, Check, Assessment) (SpawnIntent, error)
func (*Gate) WithStart(context.Context, SpawnIntent, Check,
    func(Receipt) (Started, error)) (Started, error)
```

All adapter arguments and interfaces are neutral exported DTOs. Callers import
the local `internal/nativeadmission` facade.
`Config` provides explicit existing ChefDir/BusinessDir/MaintenanceDir, the
maintenance owner's lease and a trusted canonical `Authority` interface:

```go
Current(context.Context, string) (OrderState, error)
Authorize(context.Context, Check) error
```

`Current` returns exact Order UID/revision/open/evidence. `Authorize` must verify
the full immutable native identity and reviewed attempt authority, current
canonical plan/scope/candidate/input/environment, executor enrollment and local
workspace access. These source methods must honor a bounded context, never wait for process
completion, and must not reenter any chef/businessflow/maintenance mutation or
advance DAGs. Directory selection or a peer message is not an authorization grant.

`Check` contains the existing native JobID, IntentKey, PayloadDigest, full native
Identity and StartAuthority, plus enrolled Workspace/Parent and the original
native receiver's `NativeFenceExpires`. The latter comes from the authenticated
native fence owner, not an untrusted job notification or an inferred chef renewal.
The successfully accepted `nativejob.AdvanceFence` issuance input can supply that
context while its authenticated owner retains it durably. Kit `55378df` exposes
no public fence-read/history API: the immutable native Request itself does not
contain the original expiry. After restart, missing trusted durable issuance
context remains Unknown/held. Do not query private SQL, read another owner's
journal, infer expiry from a newer lease, or manufacture replacement context.
The adapter must recompute/verify the original native RequestDigest and map every
field unchanged. This facade deliberately does not receive argv/environment or
create another native serialization/digest convention.

The existing business reservation must already bind the original native IntentKey,
Order/obligation, candidate, attempt, node and workspace. `Check.Workspace` binds
its UID/incarnation; Reference is a canonical opaque workspace reference. A guest
also binds Parent UID/incarnation/WorkspaceRef, exactly matching its maintenance
enrollment. Parent ancestry, cordon state and fresh resource evidence are checked
again by the maintenance guard. Native HostID/ExecutorID to NodeUID mapping is
verified by `Authority.Authorize`; these identifiers must not be guessed equal.
Workspace Reference and parent/enrollment provenance are likewise trusted source
facts, not authority inferred from the native request, argv, environment, peer
notifications or a caller-supplied annotation. The facade rechecks the exact
approved Check through `Authority.Authorize` while all admission guards are held
immediately before the native launch callback. The adapter must preserve that
trust boundary when mapping the original native StartCheck.

## Actual spawn, not submission or completion

An admitted assessed action supplies `Assessment{Evidence, Reason, Action}`.
Action uses the existing `admit-workflow` or `request-validation` chef vocabulary.
`DecideSpawn` persists a **separate actual-spawn** attention/decision. It neither
submits a native Pending job nor reports that job as executed. The native job and
business reservation keep their original immutable identities.

Call `WithStart` **inside** the native receiver's accepted `FinalStartGate`:

1. Read/query the original native Pending request under its existing protocol;
   do not rewrite identity or manufacture another attempt.
2. Map `StartCheck` to the exact approved `Check` and use its prepared SpawnIntent.
3. Inside the facade callback, map Receipt to the accepted native StartReceipt,
   then synchronously call the receiver's native launch callback.
4. Only after that callback returns nil (its accepted contract commits receipt,
   actual spawn and Running) return a bound `Started` acknowledgement.
5. Return from the facade/gate. The native receiver may then wait for process
   completion outside all these authority locks.

`Receipt` contains only the current accepted `native-start/v1` fields:
Version, JobID, PayloadDigest, Authority, DecisionID and AuthorityRevision. It is
evidence, not a detached bearer permit. `AuthorityRevision` binds the actual
maintenance and business published Starting snapshot revisions, plus the
business reservation policy revision, read under both writer guards.
The adapter must preserve them in the native durable receipt/result reader.

`Started` must explicitly have State `NativeSupervisorStarted`, matching JobID,
PayloadDigest, DecisionID and AuthorityRevision, plus bounded nonempty durable
Evidence. A nil callback error, Pending, queued, submitted or process completion
label is not this acknowledgement. The native callback must commit Running
before returning it; do not query the native Store from inside its gate (single
SQLite connection can deadlock). In accepted Kit, a nil native callback result
means `cmd.Start` succeeded and Running was committed; it does not prove that the
child is still alive when the acknowledgement is consumed. Birth capture can be
unavailable or a short-lived child may already have exited. Preserve the original
native owner/birth/result reader semantics rather than inventing a live-process
claim from this acknowledgement.

A durable StartReceipt alone is also insufficient: Kit commits it before spawn,
so interruption can leave a receipt on an Uncertain job without a spawned child.
Receipt existence never permits replay, proves completion or frees capacity.
The facade cannot independently prove a trusted adapter's evidence content;
exact real-native integration acceptance is required. This package implements
only an in-process synchronous callback/DTO acknowledgement. It provides no
network or detached supervisor ACK transport; those must not be advertised as
implemented by this facade or its isolated fixture.

Lock order is **actual-spawn chef Guard → business reservation guard → maintenance
start guard → native launch callback**. Each Starting state persists before the
next effect boundary. Never put native ExecuteWithGate or its process Wait inside
this facade: that would hold locks for the whole job and may double-consume the
same reservation. The older generic controllerruntime Service is not used here.

## Restart, handoff and uncertainty

Supported execution requires the original native Identity.ChefID/Fence to match
the current live chef lease **and** its original native fence expiry still be
live. Expiry or ownership transfer returns `ErrUnsupportedPendingHandoff` before
launch. Current Kit cannot adopt an old Pending identity under a new fence
without an additional accepted protocol. This facade implements no such protocol,
changes no native Request.Identity/RequestDigest and invents no retry attempt.
An unknown original native expiry is not a permit (zero expiry is invalid).

The native state depends on where rejection happens. Expired preflight **before**
calling Kit ExecuteWithGate leaves the original job Pending. Once Kit has claimed
ownership, it durably records Uncertain before calling FinalStartGate. If the
original expiry elapses or chef handoff occurs between DecideSpawn and that gate,
admission returns `ErrUnsupportedPendingHandoff`, but native state remains
**Uncertain**, not Pending. Do not rewind/retry the attempt or interpret a failed
final gate as proof that no native ownership was claimed. Rejection before
admission's guards consume admission leaves business Reserved; if those guards
were entered, Starting/Unknown may also require original-effect inspection.

The facade rechecks chef ownership/expiry under the actual chef Guard and after
potentially blocking authorization. The native receiver must independently enforce
its original local fence at the actual spawn, as its current protocol requires.
Cancellation, lost acknowledgement or a crash conservatively retains original
chef Applying/Held and business/maintenance Starting/Unknown records. No automatic
replay occurs, including after a new assessment/fence. Explicit original native
inspection/settlement must precede any capacity release or reviewed new attempt.
No result or customer acceptance is implied by Started; the original durable
native result/outbox and canonical Acceptance obligation still determine delivery.

## Source acceptance and limitations

Tests prove the public facade's boundaries with real durable journals and an
actual subprocess crash: all three records retain the uncertain original effect,
callbacks do not replay, parent maintenance can proceed after start ACK, malformed
ACKs fail closed, and cancellation/handoff/identity drift reject launch. These are
facade tests, **not real Kit/P2 integration evidence**. The separately owned Kit
adapter must map the accepted API and independently run an exact-source composite
with an actual native launch, callback receipt, delayed cordon, expiry, lost ACK,
restart and compatible native result readers on each claimed platform. Until that
candidate passes, do not advertise the facade as connected to live native starts.


The actual Kit fixture in `validation/native-admission/actual_kit_test.go.fixture`
uses only published Kit source and the successfully installed original fence
issuance input. It separately tests expiry preflight (native Pending), expiry
inside FinalStartGate after native claim (Uncertain), and chef handoff after that
claim (Uncertain). Queries occur outside the native gate. Each failed case proves
unchanged original native request/attempt/digest, no spawn/receipt, and retained
business Reserved when admission's guards were never entered. These are bounded
Linux isolated-source results; no new handoff protocol or live deployment is
implied.
