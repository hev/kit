# Share a dispatch host

Install the same current `hev` on the host and each laptop. SSH must already
reach the host; this feature starts no listener. Keep a stable absolute path
for the host executable: authorized keys refer to it directly.

Dispatched agents execute as the host account, including its file and
credential access. Invite only teammates trusted to run agent work as that
account. The forced SSH command prevents interactive shells and restricts the
RPC surface; it does not sandbox an agent's programs. Existing unrestricted
SSH keys keep their existing access until the owner removes them.

## Owner

On the host, make an invitation with a reachable address and bot account:

```sh
hev nodes invite alex --address build.example.test --user bot --expires 1h
```

Use `--host-key FILE.pub` when the host's public key is elsewhere. Send the
printed code privately by paste. It contains the endpoint, pinned host public
key, fingerprint, expiry and a one-use random token. Verify the endpoint before
sharing. A code can be used until the owner accepts one request or it expires.

The teammate sends a request back. Review their identity and public key out of
band (the request is base64url JSON), then paste it into:

```sh
hev nodes accept REQUEST
hev nodes ls
```

On Windows, run as the SSH account. Administrator accounts must use
`hev nodes accept REQUEST --administrators`, which chooses
`C:\ProgramData\ssh\administrators_authorized_keys` and restricts its ACL to
SYSTEM and Administrators. Standard users use `%USERPROFILE%\.ssh\authorized_keys`
with SYSTEM and their own account. Unix uses `~/.ssh/authorized_keys` with mode
0600 in a 0700 directory. The installer preserves other key lines. Windows
OpenSSH must support restricted forced commands and the default cmd-compatible
quoted executable syntax. Validate SSH access before removing old credentials.

Revoke a teammate with:

```sh
hev nodes remove alex
```

This revokes their host-owned grant and removes exactly its managed key line.
Other keys remain intact. It denies future RPCs; it does not cancel an already
running command or agent. Stop outstanding work separately. If removing the
key file fails, the grant is already revoked; the error identifies the file
that requires owner cleanup. Interrupted `nodes` operations may leave a
`~/.config/hev/nodes/lock` directory: check no operation is running before
removing that directory. Invitation files contain bearer tokens; protect the
nodes directory and never commit it.

## Teammate

Verify the invitation came from the owner through a trusted channel, then:

```sh
hev nodes join CODE
```

Join creates a dedicated SSH key, a pinned known-hosts file and SSH alias in
`~/.ssh`, and appends a host to `~/.config/hev/dispatch.toml`. Use
`--config FILE` for another dispatch configuration. It prints a request to
send back to the owner. Dispatch becomes available after acceptance. Find the
host alias with `hev nodes ls`, then:

```sh
hev dispatch run project task --host hev-node-EXAMPLE
hev dispatch ls --host hev-node-EXAMPLE
hev dispatch show task --host hev-node-EXAMPLE
hev dispatch say task 'Please check the failing case' --host hev-node-EXAMPLE
hev dispatch logs task --host hev-node-EXAMPLE
hev dispatch stop task --host hev-node-EXAMPLE
```

A teammate can start runs and list, show, logs, say and stop only runs bearing
their bound sender name. Inbox-only `peer-send` is also allowed for their
own runs so native peers delivery can use this SSH transport; it holds messages
for ended runs without starting rework. Generic peers commands and inbox
read/consume/ACK operations are denied. Other RPC actions (including attach, clean, amend,
validation and orchestrators) are unavailable with this key. Owners can perform
those actions locally. Shell, PTY, forwarding and arbitrary remote commands are
unavailable. The owner controls installed software and repository admission.
The teammate cannot change their sender name in a dispatch request.

Sender is stored in `meta.json`, displayed in inventory and show, included in
notification summaries (`{sender}` is also available), and indexed as `sender`
on trace chunks and sessions with `order`. Unrestricted local starts use the
local account identity (`local:ACCOUNT`, a separate namespace from teammate names); old runs may have no sender field.

## Compatibility and website follow-up

`hev nodes` replaces the old coordination-backed node registry CLI. `hev orders`
and `hev runs` keep their existing backend. `hev overview nodes` queries that
backend directly; `hev get nodes` remains the machine resource view. Consumers
of the former `nodes list/show` registry shape need to call their backend or
migrate; `nodes list` now aliases access listing and has a different shape.

On the website branch, share-a-host should document invite/accept/remove,
Windows administrator paths and the agent execution trust boundary.
Join-a-factory should document pinned host keys, join/request exchange, host
aliases and ownership restrictions. No website sources change here.
