# One native execution contract across hosts

The target path is one controller/intake/job contract into a registered hevd node,
then the same original native issuance, DB final spawn exclusion, business and
maintenance admission, postcommit readiness, owned Wait, terminal completion and
exact original ACK. A baseline coding run measures execution capability; it does
not establish this unified path. OS adapters must not select replacement argv,
remap fences, introduce another controller or start another production daemon.

The shared nativehost Profile, Dispatch, registration and owner-proof DTOs remain
unchanged. Platform seams are kernel peer authentication, private endpoint/journal
filesystem operations, process birth, managed descendant ownership and restart
cleanup, and terminal-tree evidence. Each enabled role needs actual native proof.

Darwin's credential adapter uses LOCAL_PEERCRED and LOCAL_PEERPID through the
connected Unix socket in both directions. A malformed, closed, unsupported or
foreign-UID peer is held. PID is never execution authority. Darwin role capability additionally requires a successful exact-live-birth
resource-counter observation on the native-tested arm64 kernel build25G83.
Other builds and failed probes remain held. Credentials and this capability
probe do not establish launchd descendant containment or production readiness.
The existing hevd LaunchAgent is the production ownership target. launchd's same
process-group cleanup differs from a Linux control group; a harness that creates
another process group may escape. Conversely the daemon's inherited group cannot
be required empty while the daemon is serving. The harness and terminal producer
must establish the exact managed composition's lifecycle boundary before
production admission. Controlled candidate tests do not transfer production
acceptance or permit adoption of a previous daemon incarnation.

Windows has a controlled candidate on native-proved amd64 kernel 10.0.26300,
with production checked-journal operations and nonzero user-session hosting.
Other kernels/architectures and Session0 remain held. Managed public-caller,
process-tree/restart and final spawn/ACK acceptance remain required. Previously tested Windows journal DACL/durability primitives do
not establish model execution or receiver hosting. The Windows adapter must reuse
the same job/authority protocol, rather than a legacy CLI fallback or a guard bypass.

Linux source/controlled-smoke acceptance is scoped to its exact candidate and
route. Cross-compilation, inventory and baseline jobs never transfer platform
acceptance. Unknown or surviving descendants keep the original uncertain and
capacity charged; no arbitrary PID adoption or replay is permitted.

The Unix listener/lock/stale-socket implementation is shared by Linux and Darwin.
The platform capability guard runs before any filesystem mutation. Darwin
requires its native-tested, birth-bound kernel probe; transport alone is
insufficient. Registration, final DB/native admission and managed terminal
producer checks remain separate. Actual Darwin stale
socket tests preserve live/unknown/replaced endpoints. The existing SDK export
`nativejob.ProcessBirth(pid) (Birth, error)` supplies `boot_id`, `pid`, `start` on
Darwin from boot-session UUID and kernel creation timeval. An owned harness can
capture birth, read PGID, and recheck birth without changing the DTO. Its independent
process group must remain distinct from daemon PID/identity. Unknown restart or
escaped descendants remain uncertain and charged, never adopted or replayed.

## Internal transport backend seam

The shared loop calls `validateEndpoint(path)`, `openEndpoint(ctx, configPath,
config)`, `verifyListener(listener)`, `dialEndpoint(ctx, path)`,
`readRequest(conn)`, `writeRequest(conn, payload)` and `peer(conn)`. These are
internal OS adapters; public job DTOs and authority do not change. The listener
owns its endpoint-lock lifetime. Configuration verification is static; opening
or dialing is separate. Kernel peer proof is required on both sides before a
trusted payload, and cancellation closes only the owned connection.

Unix framing remains bounded JSON followed by write-half-close. A Windows backend
must provide a bounded native pipe-message boundary rather than pretend to be a
Unix connection. Incoming bytes are limited to64KiB before JSON decoding; trailing
JSON is rejected. One connection carries one request and its existing literal
`accepted` reply. The Windows backend uses SID-authenticated private pipes and checked DACL/no-reparse
operations. Its capability guard permits the controlled candidate only; it does
not establish managed model/lifecycle acceptance.

The fixed receiver spawn hook is `startReceiver(Profile, descriptorRef)`, returning
an owned Wait function and error. A successful start returns its waiter. An error
with no waiter is permitted only when no unresolved live receiver effect remains;
an uncertain creation/cleanup retains and returns a real handle-backed waiter.
The shared loop owns that waiter even after an uncertain start reply. A waiter
wrapping `ErrHeld` retains the local physical receiver slot. A known terminal
result releases only that local slot, never business/maintenance capacity or a
coding success claim. An invalid nil-waiter/nil-error backend remains held. The
Wait function is not the original job completion/ACK protocol.
