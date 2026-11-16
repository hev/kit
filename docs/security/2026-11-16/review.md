# Network exposure and dispatch security review

Reviewed 2026-11-16 against `a7075f0d217c0dd29c5ca9c4cc2385b86ff5a4c1` on Linux.
Requested by local:hev.

## Scope and conclusion

This is a source review and synthetic test exercise, not a production penetration
test or a claim that the repository is free of vulnerabilities. No live services,
firewalls, credentials or access grants were changed. The deliverable is this
review; the findings below remain open. Dependencies, hosted Layer authorization,
operating-system firewall policy and external SSH server configuration were not
audited. Browser exploitation and a real network handover were not exercised.

The normal local stack is protected from direct LAN access by loopback binding.
Moving a laptop to public Wi-Fi does not itself turn a loopback listener into a
wildcard listener. However, opt-in wildcard listeners keep accepting connections
on newly attached interfaces. None of the reviewed listeners automatically
re-evaluates network trust after a handover. Explicit interface binding, an
authenticated encrypted transport and persistent host firewall policy must form
the boundary; the network's current designation as private is insufficient.

Dispatch authenticates restricted SSH callers and applies ownership and repository
checks, but its workers execute with the host account's authority. Admission is
permission to run an agent with that authority, not just permission to edit a repo.

## Exposure inventory

| Surface | Code/default | Handover implications |
| --- | --- | --- |
| Read-side dashboard | `cmd/hev/serve.go:56`, `127.0.0.1:8787` | Safe from direct LAN access by default; `--bind 0.0.0.0` removes that boundary. |
| Docker gateway/dashboard | `internal/local/docker-compose.yml:19,55`, host ports on `127.0.0.1` | Container's internal wildcard bind is not a host wildcard publish. Inspect compose overrides and proxies separately. |
| Board | `internal/board/cli/cli.go:79`, requires configured `board.addr` or `--addr` | No enforced loopback-only choice; read-only mode rejects writes but does not authenticate reads. |
| Optional peers HTTP ingress | `internal/peers/hevd.go:145`, off unless listen is configured | Configured TCP address persists across networks; bearer token with no native TLS. |
| Dispatch RPC | `internal/dispatch/rpc.go:142`, SSH | Host-key checking is explicitly strict; no new dispatch TCP listener. SSH exposure depends on sshd/firewall, outside this repository. |
| Linux secrets agent | `internal/secrets/ipc_linux.go`, Unix socket | Private directory/socket modes and same-UID peer credentials; no TCP listener. Same account is still trusted. |

## Findings

### NET-1 — High, conditional: unauthenticated dashboard and board on broad binds

Evidence: `internal/serve/server.go:68` registers trace/session/search/factory
handlers without authentication or Host validation. `cmd/hev/serve.go:33,47`
serves that handler directly on the configured bind. `internal/board/server/server.go:67`
and `:108` similarly expose read and mutation routes; `internal/board/cli/cli.go:104`
serves them directly. Layer credentials are used by the server and do not require
the inbound user to authenticate. Board poster fields are supplied by the caller.

An attacker who can reach a broad-bound dashboard can retrieve archived prompts,
tool results and operational metadata. A reachable writable board additionally
allows posting, identity spoofing and pin/lock operations through the server's
backend credentials. This is conditional on the operator exposing the listener;
the default dashboard does not expose traces directly to public Wi-Fi.

Reproduction: `TestSecurityReviewUnauthenticatedTrace` supplies only synthetic
archive data, a foreign Host and a non-loopback RemoteAddr. With no Authorization
header, `/api/session/audit` returns 200 and `synthetic-private-prompt`. Existing
`TestLegacyServerGoldenWire` demonstrates board reads and writes with no inbound
authentication while the server authenticates to its mock backend. Handler tests
do not establish production firewall reachability.

Mitigation: keep local listeners on loopback and use SSH forwarding or a narrowly
bound private overlay interface. Before permitting broad binds, require explicit
exposure consent plus authenticated TLS access (including all API routes), and
validate allowed Host values. A reverse proxy must prevent direct backend access.
Verify that connections from an untrusted interface fail after a network change.

### NET-2 — High, conditional: peers bearer token and instructions over plaintext

Evidence: `internal/peers/hevd.go:145-157` accepts arbitrary configured TCP bind
addresses and uses `srv.Serve`, not TLS. `:222-239` accepts `http` or `https`
endpoints, then attaches the bearer token. There is no restriction that HTTP be
loopback or travel within an authenticated encrypted tunnel. A 32-character
minimum token length does not encrypt the connection.

A network observer/on-path attacker on an unencrypted route can read the token
and message body. A stolen token can enqueue attacker instructions in any eligible
registered inbox; the listener's token is shared authority, not a per-sender grant.
`Handler` validates the syntax of client-supplied `From`, `Order` and `Run`, but
those values are not cryptographically bound to an individual identity. Do not
treat their appearance in a message as authentication. Instruction execution by an
agent remains a separate trust boundary; this endpoint is not direct shell execution.

Reproduction: existing `TestHEVDStatusesAndOutboundOnly` uses an HTTP test server
and checks receipt of Authorization and message data. `TestHEVDSenderAddresses`
shows accepted caller-selected sender labels. `TestStartListenerConfiguration`
checks disabled ingress and short-token rejection, but imposes no TLS policy.

Mitigation: disable this optional listener on roaming hosts until needed. Prefer
SSH transport, or authenticated TLS/overlay encryption with source restrictions.
Reject unencrypted non-loopback endpoints unless an explicit protected-transport
policy is configured. Bind sender authority to per-peer credentials, attach
receiver-derived provenance, and authorize inbox targets. Rotate tokens if they
have traversed an untrusted plaintext route. Preserve the existing no-redirect
credential forwarding and no-retry-on-ambiguous-delivery behavior.

### DSP-1 — High architectural trust risk: restricted dispatch reaches an unrestricted worker

Evidence: `internal/dispatch/sender.go:65` permits authenticated teammates to start
orders after discarding client launch authority. `internal/dispatch/run.go:255-257`
launches Codex with `--dangerously-bypass-approvals-and-sandbox` and Claude with
`--dangerously-skip-permissions`. At `:291`, the worker inherits the process
environment except configured plugin-only variables. There is no default OS user,
container or filesystem isolation in this launch path. A host-owned harness wrapper
can provide extra isolation; it is not required by default.

A malicious admitted requester, compromised repository instruction or prompt
injection can cause the worker to access the host account's files, credentials,
other worktrees and network. Restricting SSH to a forced RPC command and checking
run ownership does not contain the child agent's authority. This is a deliberate
execution design with security consequences, not a demonstrated shell quoting
bug or an unauthenticated remote exploit. Environment filtering for source plugins
protects those named values from ordinary inheritance, not every account secret.

Reproduction/evidence: inspect the launch argv and `child.Env` above; no real agent
was launched to steal credentials. `TestSenderBindingAndRunPermissions` verifies
RPC restrictions, not OS containment. Existing isolation tests concern process
ownership and cleanup, not filesystem/network sandboxing.

Mitigation: treat every admitted sender/repository as trusted with the worker
account until containment exists. Require a dedicated worker identity or mandatory
host-owned isolation wrapper, minimal credentials/environment, bounded mounts,
restricted network egress and no shared agent/secret sockets. Test attempted reads
outside the admitted workspace and sibling run mutation using synthetic files.
Do not describe the current worktree boundary as a security sandbox.

### DSP-2 — Medium policy footgun: omitted repository admission allows all

Evidence: `internal/dispatch/admission.go:44-49` returns success when `Repos` is nil.
An explicit empty list denies all; a missing key admits all, including for restricted
callers. This behavior is documented in `docs/dispatch.md` and does not bypass an
explicit allowlist. A host owner may nevertheless assume invitation limits a
teammate to a particular repo when no such default restriction exists.

Mitigation: configure an explicit target-owned allowlist before accepting teammates.
Consider failing closed for restricted callers when the policy is absent, with an
explicit compatibility opt-in for trusted unrestricted use. Admission remains a
selection control, not containment (DSP-1).

### WEB-1 — Medium: board form writes have no Origin/CSRF gate

Evidence: `internal/board/server/server.go:67-104` checks only read-only mode and
adds CORS response headers for selected GET/OPTIONS requests. Form write routes
(`/new`, `/t/{id}/reply`, `/t/{id}/useful`) consume form values without checking
Origin, Fetch Metadata or a CSRF token. CORS does not reject simple form POSTs.

A malicious page may submit a cross-origin form to a reachable writable board,
even if it cannot read the response. Browser private-network/loopback restrictions
vary and can reduce reachability; this review does not claim a browser exploit
on every browser. Foreign Host acceptance also leaves a potential DNS-rebinding
path, whose browser/DNS prerequisites were not tested.

Reproduction: `TestSecurityReviewForeignOriginMutation` sends a form POST with
`Origin: https://attacker.invalid` through the real middleware. A synthetic write
route receives it and returns 201. The stub deliberately avoids a real backend
mutation; actual form handlers were source-reviewed.

Mitigation: validate Origin/Fetch Metadata and CSRF tokens on browser mutations;
validate Host on local services. Retain authentication for direct clients and
read-only mode as a temporary write restriction. Read-only is not confidentiality.

## Verified protections and limits

Restricted node keys use `restrict,command=...` (`internal/nodes/nodes.go:235`),
validate the original command, and load the access record on every RPC (`:508`).
`authorizeSender` binds starts to authenticated identity, rejects delegation and
validation authority, checks ownership for existing runs, and converts teammate
say into inbox-only delivery. Target policy comes from target-owned configuration.
These are meaningful controls; no RPC ownership bypass was demonstrated.

Peers authenticates before parsing, caps bodies, rejects unknown fields, uses
constant-time bearer comparison, disables HTTP redirects and limits HTTP delivery
to registered inboxes. Archive redaction is defense in depth, not access control.
There was no inspection or publication of actual private prompts or secrets.

## Reproduce the synthetic evidence

The adjacent `.go.txt` files preserve the exact temporary probes. From the repo root:

```sh
cp docs/security/2026-11-16/serve_probe_test.go.txt internal/serve/security_review_probe_test.go
cp docs/security/2026-11-16/board_probe_test.go.txt internal/board/server/security_review_probe_test.go
go test -count=1 -v ./internal/serve ./internal/board/server -run '^TestSecurityReview'
rm internal/serve/security_review_probe_test.go internal/board/server/security_review_probe_test.go
```

These probes assert the observed insecure behavior to reproduce findings, not
security regressions that should stay green after remediation. They are kept out
of the normal test suite intentionally. Each was run once before the broader
package check; no failing test was rerun to obtain green. Dispatch run evidence
contains the exact commands and logs; the handoff report records final outcomes.

## Remediation order

1. On roaming machines, inventory actual listeners and compose/proxy overrides;
   remove wildcard publishes or enforce firewall/tunnel boundaries before handover.
2. Close NET-1/NET-2 with authenticated encrypted access and explicit bind policy.
3. Establish DSP-1 containment before treating teammates or repo content as untrusted;
   set explicit repo admission in the meantime.
4. Add board Origin/CSRF/Host protections and browser-level rejection tests.
5. Validate on Linux and macOS with a second isolated host/interface: default
   loopback remains unreachable, exposed routes require authentication, plaintext
   peers routing is refused, and unauthorized workers cannot read synthetic sibling
   files. No production network or firewall changes were made for this review.
