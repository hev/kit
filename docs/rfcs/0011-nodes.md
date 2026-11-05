# RFC 0011: File-based host invitations and dispatch identities

Status: Proposed; implementation in this PR

## Summary and motivation

Replace the retired coordination-backed `hev nodes` with SSH invitations. A
host owner grants a named teammate dispatch access without granting an
interactive SSH session. No daemon, control plane, listener or new port is
introduced. SSH remains the transport; invitations and requests move by paste.

## Decision and proposed surface

The owner runs `hev nodes invite NAME --address HOST --user USER`. The code
contains a random 256-bit token, expiration, SSH endpoint and the complete
host public key (and its SHA256 fingerprint). The owner verifies the endpoint
and sends the code privately. The teammate runs `hev nodes join CODE`, which
creates a dedicated Ed25519 key, pins the host key in a dedicated known-hosts
file, writes an SSH alias and a dispatch host entry, then prints a request.
The owner runs `hev nodes accept REQUEST`. Only the host's saved invitation
is authoritative for the name and endpoint. Accept consumes the invitation;
expired or replayed requests fail. Interrupted accepts fail closed and may
require a fresh invitation. Owner operations serialize through a file lock.

The installed key uses `restrict,command="... dispatch rpc --node ID"`.
The ID selects a host-owned access record, checked on every invocation.
SSH_ORIGINAL_COMMAND must be exactly `hev dispatch rpc`; shell requests,
commands appended to RPC, forwarding, PTYs and subsystems are refused.
The RPC overwrites sender metadata with the bound name. Teammates may start
runs and list, show, logs, say and stop their own runs. Other actions, forwarded
runs, orchestrators and access to other senders' runs fail closed. Owners
retain unrestricted local dispatch; their OS account identity is recorded with a `local:` prefix, which cannot
collide with an invitation name.
The inbox-only `peer-send` RPC is also allowed for the sender's own run,
so native peers delivery can share the same restricted SSH command. It holds
messages for ended runs without launching rework. Generic peers commands,
arbitrary session messaging, inbox reads, consume/ACK and caller-selected
sender identities are not admitted by this RPC grant.
List filters both names and rows. Last-used records successful authentication,
including denied RPC actions. Sender appears in metadata, inventory,
notifications and indexed trace tags alongside order attribution. Metadata also
records `requested_by` from the same authenticated sender, never a caller claim.
It is available in worker prompts, `HEV_REQUESTED_BY`, notification
`{requested_by}` substitutions, and source-order `{{.RequestedBy}}` templates
(the latter use the local source submitting identity). PR descriptions can
therefore state who requested the work.

`hev nodes ls` shows host grants (added and last-used times) and laptop joins.
`hev nodes remove NAME` first revokes the access record, then removes the exact
managed key line, preserving unrelated bytes. Revocation denies new RPCs even
if the line could not be removed. It does not terminate existing runs or SSH
connections already executing an RPC. Stop a run separately when required.

## Trust model and non-goals

A paste code is a bearer invitation: protect it, verify its provenance, and
review the public-key fingerprint before accepting. Host keys are pinned,
never learned with trust-on-first-use. Names are stable host-owned labels;
revoking and reinviting a name deliberately preserves that person's run access.

This is an SSH/RPC access boundary, **not an agent sandbox**. A dispatched
coding agent executes as the bot account and can read its files, use its
credentials, and execute programs. A hostile agent can modify access files.
Only invite people trusted with that execution capability. Repository admission
policy still applies. Isolating untrusted agents requires a separate OS account
or machine and is outside this proposal. Sender labels provide attribution,
not cryptographic attestation of work performed inside the account.

## Windows

Windows OpenSSH administrator accounts use
`C:\ProgramData\ssh\administrators_authorized_keys`; ordinary accounts use
`%USERPROFILE%\.ssh\authorized_keys`. The owner explicitly selects
`--administrators` for the former. The installer removes inherited ACLs and
grants only SYSTEM and Administrators for the administrator file; ordinary
files grant SYSTEM and the current account. Unix uses a 0700 directory and
0600 file. The forced command uses an absolute executable path; Windows paths
use forward slashes and double quotes, compatible with the default cmd shell.
Custom OpenSSH shells must support that quoting. ACL errors abort installation.
Windows CI exercises paths, command generation and ACL argument generation;
a real Windows SSH rollout remains an owner validation step.

## Compatibility and documentation

The old `hev nodes list/show` coordination inventory surface is replaced.
Use `hev get nodes` for machine resources. `hev overview nodes` continues
calling the configured coordination backend directly. `hev orders` and
`hev runs` retain their existing coordination backend and semantics. Private coordination
backends invoking the public command need migration; their own backend command
is unchanged. No dispatch wire action is renamed. Existing unrestricted keys
remain unrestricted; remove them manually after migrating.

The website branch should update share-a-host with invite/accept/revoke and
the agent-execution trust boundary, and join-a-factory with join, pinned keys,
request exchange and sender ownership. This PR does not edit site/.

## Open questions

A future version may support per-repository grants and isolated execution.
Neither is implied by this release's named access records.

## Platform references

Windows file selection and administrator ACLs follow Microsoft's
[OpenSSH server configuration](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh-server-configuration)
and [key management](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh_keymanagement).
