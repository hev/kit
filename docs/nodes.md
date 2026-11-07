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

A teammate is accepted without extra capabilities. To let them add and remove
repositories themselves (see [Teammate-added repositories](#teammate-added-repositories)),
accept with `--allow repos`, or change it later without re-inviting:

```sh
hev nodes accept REQUEST --allow repos
hev nodes allow alex repos
hev nodes deny alex repos
```

`allow`/`deny` rewrite only the host-owned grant record; the SSH key line is
untouched and the change applies to the teammate's next RPC. `hev nodes ls`
shows `allow=repos` (or `allow=-`). Grants made before this option existed
carry no capabilities.

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

Every teammate can list the host's admitted repositories:

```sh
hev admission ls --host hev-node-EXAMPLE
```

With the `repos` capability, a teammate can also add or remove one:

```sh
hev admission add https://github.com/ExampleOrg/widget.git --host hev-node-EXAMPLE
hev admission remove https://github.com/ExampleOrg/widget.git --host hev-node-EXAMPLE
```

`add` clones it into the host workspace (directory `widget`) and admits it;
dispatch it with `repo: widget`. `remove` stops admitting it. See below for the
limits.

A teammate can start runs and list, show, logs, say and stop only runs bearing
their bound sender name. Inbox-only `peer-send` is also allowed for their
own runs so native peers delivery can use this SSH transport; it holds messages
for ended runs without starting rework. Generic peers commands and inbox
read/consume/ACK operations are denied. Other RPC actions (including attach, clean, amend,
validation and orchestrators) are unavailable with this key. Owners can perform
those actions locally. Shell, PTY, forwarding and arbitrary remote commands are
unavailable. The owner controls installed software and repository admission, unless
they grant the `repos` capability described below.
The teammate cannot change their sender name in a dispatch request.

Sender and its human-facing alias `requested_by` are stored in `meta.json`, displayed in inventory and show, included in
notification summaries (`{sender}` and `{requested_by}` are also available), and indexed as `sender`
on trace chunks and sessions with `order`. Unrestricted local starts use the
local account identity (`local:ACCOUNT`, a separate namespace from teammate names); old runs may have no sender field.

Workers receive the requester in their prompt and `HEV_REQUESTED_BY` environment
variable, so an order can include it in PR descriptions. Source-order Go
templates can use `{{.RequestedBy}}` for the local submitting identity. The
host always derives `requested_by` from the authenticated sender, overwriting
client-supplied metadata; orders cannot select a different requester.

## Teammate-added repositories

The owner decides where teammates may add repositories from, in the host's own
dispatch configuration (`~/.config/hev/dispatch.toml` or its
`HEV_DISPATCH_CONFIG`):

```toml
[admission]
repos = ["https://github.com/ExampleOrg/kit"]
add_from = ["https://dev.example.test/ExampleOrg/", "https://github.com/ExampleOrg/"]
```

A teammate can add a repository only when their grant has `repos` **and** the
URL is under an `add_from` prefix. A missing or empty `add_from` means nobody
can self-add. The two refusals are distinct: "your access does not include
adding repos" (no `repos` grant) and "URL is not in an allowed organization"
(no `add_from` prefix matches). `hev admission add URL --host H` then:

1. Validates the URL: `https://` or `ssh://` URLs, or `user@host:path`. Local
   paths, `file://`, `http://`, `git://`, `transport::` helpers, user names or
   passwords in HTTPS URLs, queries, fragments, `.`/`..` segments, empty
   segments, percent-encoding other than `%20`, leading `-` and shell or quote
   characters are refused, not rewritten.
2. Matches the normalized origin the same way `[admission] repos` does (host
   case-insensitive, path case-sensitive, transport, SSH user and `.git`
   ignored, explicit ports significant) on whole path segments:
   `https://github.com/ExampleOrg/` admits `git@github.com:ExampleOrg/widget`
   but not `https://github.com/ExampleOrg.evil/widget` or
   `https://github.com/exampleorg/widget`.
3. Clones it into the host workspace as the last path segment without `.git`
   (spaces become `-`), using the host account's own git configuration and
   credentials. The teammate supplies only the URL: no git arguments, path,
   branch or ref. The clone runs with transports limited to https and ssh
   (`GIT_ALLOW_PROTOCOL`), hooks disabled (an empty `core.hooksPath`, no
   templates), no submodules, no HTTP redirects, terminal prompts disabled,
   at most 10 minutes and 4 GiB. It clones into a temporary directory first, so
   a failed or bounded clone leaves nothing behind.
4. Appends the origin to `[admission] repos` under the dispatch lock. The file
   is edited in place: comments, ordering and all other keys are kept, and the
   edit is checked by decoding before it atomically replaces the file. Only a
   `repos = [...]` array under an `[admission]` header is edited; other
   spellings are refused with a request to add the entry by hand. A missing
   `repos` key already admits everything and is left missing.
5. Records the teammate (the same identity as `requested_by`) and time in
   `admission-log.jsonl` next to the configuration; `hev admission ls` shows it.

Repeating an add, or adding an equivalent spelling, changes nothing. A
different repository under an existing workspace name is refused. Owners run
the same commands locally without `--host` (or over their own unrestricted
SSH); they skip the grant and `add_from` checks but keep the URL and clone
safeguards.

`hev admission remove URL --host H` (alias `rm`) needs the same `repos` grant
("your access does not include removing repos" otherwise) and:

1. Accepts only a URL that `add` would accept, under an `add_from` prefix.
   Admitted repositories outside `add_from` are owner-only and refused with
   "URL was not added from an allowed organization; ask the owner".
2. Drops every `[admission] repos` entry with the same normalized origin, under
   the dispatch lock, with the same in-place, decode-checked, atomic edit as
   add. An entry alone on its line is removed with its line and comment.
   Removing the last entry leaves `repos = []`, which admits nothing.
3. Leaves the clone in the workspace and does not stop runs already started.
   New dispatches to the repository are refused from then on. The output says
   so.
4. Succeeds without a change when the repository is not admitted. On a host
   with no `repos` list (which admits every workspace repository) there is
   nothing to remove from, so it is refused; the owner must add a list first.
5. Records the remover and time in `admission-log.jsonl`, the same log as adds.
   `hev admission ls` attributes only additions still in effect.

Owners run `hev admission remove` locally too, without the grant or `add_from`
checks, and may name any entry exactly as it appears in the list (including
local paths).

Trust boundary: a teammate with `repos` can make the host's bot account clone
any repository its credentials reach under `add_from`, and then dispatch agents
against it. They can also withdraw admission for any repository under
`add_from`, including ones the owner added, which blocks new dispatches to it
for everyone until it is added again. Keep prefixes as narrow as the work needs. The capability grants no
other RPC action.

On Windows hosts the clone runs non-interactively inside the SSH session.
Credential helpers that need a desktop session or prompt (for example Git
Credential Manager's interactive sign-in) will fail rather than wait; test
`hev admission add` over the teammate's SSH path once after configuring the host's
credential helper.

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

### Restricted messages and validation

Teammate `say` always appends to the run inbox, including after the run ends;
it never starts rework. Validator children are excluded from teammate inventory
and all teammate run actions, even when their attribution names the teammate.
Teammate starts cannot request `validation: browser`: the owner must initiate
validation with their own authority. This prevents an invitation from implicitly
authorizing execution on the owner's separate validation host.

Authentication reads the grant without the owner mutation lock. Last-use updates
are advisory, atomic, separate files; interrupted owner locks do not block RPCs.
Accept appends to authorized_keys. Removal prepares a secured sibling and renames
it into place. Windows applies a complete protected DACL in one operation using
the process user's SID, or SYSTEM and Administrators for the administrator file.
Owners should serialize unrelated external edits to authorized_keys with removal.
