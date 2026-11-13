# Share a dispatch host

Install the same current `hev` on the host and each laptop. SSH must already
reach the host; this feature starts no listener and opens no port. Keep a
stable absolute path for the host executable: authorized keys refer to it
directly.

Dispatched agents execute as the host account, including its file and
credential access. Invite only teammates trusted to run agent work as that
account. The forced SSH command prevents interactive shells and restricts the
RPC surface; it does not sandbox an agent's programs. Existing unrestricted
SSH keys keep their existing access until the owner removes them.

## Owner

On the host, `hev skills setup --role host` installs the owner skills and
selects the host half of team skill sources. On a machine that runs a dispatch
host (`local = true` in dispatch.toml) and has no `[skills] role` yet, setup
refuses to guess and asks for `--role`. Make an invitation with a
reachable address and bot account:

```sh
hev nodes invite alex --address 100.64.0.10 --user bot --expires 1h
```

It prints a short code and the command the teammate runs:

```text
Invite code for alex: 7K-Q4M2-XPWD (single use, expires 2026-11-09 15:00 UTC, locks after 5 wrong tries)
On alex's machine: hev nodes join bot@100.64.0.10 7K-Q4M2-XPWD
Then check the fingerprint with them and approve: hev nodes accept alex
```

Read or send the code privately. Use `--host-key FILE.pub` when the host's
ed25519 public key is not at the default path, `--allow repos` to grant the
capability on accept, and `--expires` up to `24h`. The code is single use:
it stops working once a teammate's request is recorded, after 5 wrong codes
(the invitation locks), or at expiry. One invitation per name is open at a
time; `hev nodes remove alex` withdraws an open, locked or pending one.

When the teammate has joined, their request is pending:

```sh
hev nodes pending      # also shown by hev nodes ls
# alex invite=pending expires=... key=SHA256:... (accept: hev nodes accept alex)
```

Compare the `key=` fingerprint with the one `hev nodes join` printed on their
machine (by voice or chat), then approve by name:

```sh
hev nodes accept alex
hev nodes ls
```

Accept installs exactly the key the teammate proved with the code, with the
same forced command as before. A pending request must be accepted before the
invitation expires. See [How short codes work](#how-short-codes-work) for the
security design.

`hev nodes invite --long` still prints the older pasted code (a base64url
invitation carrying the endpoint, pinned host key and a one-use token). The
teammate runs `hev nodes join CODE` and sends back a request, which the owner
pastes into `hev nodes accept REQUEST`. Use it where the enrollment key below
is unwanted.

On Windows, run as the SSH account. Administrator accounts must use
`--administrators` on both `hev nodes invite` and `hev nodes accept`, which chooses
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
hev nodes accept alex --allow repos
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
that requires owner cleanup. `nodes` commands serialize on an OS file lock
(`~/.config/hev/nodes/nodes.lock`) that is released when the holding process
exits, so an interrupted command leaves nothing to clean up. A `lock`
directory left by an older `hev` is no longer used and can be deleted.
Invitation files contain bearer tokens; protect the nodes directory and never
commit it.

## Teammate

Run the command the owner gave you:

```sh
hev nodes join bot@100.64.0.10 7K-Q4M2-XPWD
```

The host part can be an address or an SSH host name your SSH configuration
resolves; keep the `bot@` account. Case, dashes and spaces in the code do
not matter. Join reaches the host over SSH, checks that the host knows the
code, learns and pins its host key, and submits a new dedicated public key.
It prints that key's fingerprint: read it to the owner so they can match it
before accepting. A wrong code, or a machine pretending to be the host, fails
with "the host could not prove it knows this code", and nothing about your key
is sent. Each wrong try counts toward the invitation's lockout.

Join creates a dedicated SSH key, a pinned known-hosts file and SSH alias in
`~/.ssh`, and appends a host to `~/.config/hev/dispatch.toml`. Use
`--config FILE` for another dispatch configuration. Dispatch becomes
available after the owner accepts. (For a `--long` code, run
`hev nodes join CODE` and send the printed request back to the owner.) Find
the host alias with `hev nodes ls`, then:

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

Then give your agents the member skills (any time; it is idempotent):

```sh
hev skills setup
```

`hev nodes join` prints this step but does not run it: join edits SSH and
dispatch configuration, and setup writes into your harness homes
(`~/.claude/skills`, `~/.codex/skills`, and `~/.agents/skills` for pi and
OpenCode), which join should not do without asking. Setup starts no daemon or
capture. When every host in your dispatch configuration is a joined node, it
installs `hev-member`, which teaches only what your key can do: send runs and
orders, list, show, logs, say and stop your own runs, and check `ls` for
finished ones, and `hev admission ls` (plus `add`/`remove` with the `repos`
capability). `hev-peers`, `hev-board`, `hev-handoff` and `hev-traces` are
installed for members too; they cover local sessions, the board and reports.
The owner-only `hev-dispatch` and `hev-expedite` are not installed, and earlier
unedited copies are removed; locally edited copies stay unless you pass
`--force`, and same-name skills hev never installed are always kept. If you also
dispatch to hosts you own, you get both sets. Rerun setup after joining or
removing hosts. On Windows the same paths live under `%USERPROFILE%`.

A teammate can start runs and list, show, logs, say and stop only runs bearing
their bound sender name. Inbox-only `peer-send` is also allowed for their
own runs so native peers delivery can use this SSH transport; it holds messages
for ended runs without starting rework. `member-notices` and
`member-notice-ack` read and acknowledge only the teammate's own run notices
(below). Generic peers commands and inbox
read/consume/ACK operations are denied. Other RPC actions (including attach, clean, amend,
validation, orchestrators and `update`) are unavailable with this key: the host
refuses a member's `hev dispatch hosts update` as an owner action. Owners can
perform those actions locally. Shell, PTY, forwarding and arbitrary remote commands are
unavailable. The owner controls installed software and repository admission, unless
they grant the `repos` capability described below.
The teammate cannot change their sender name in a dispatch request.

### How run notices arrive

The host cannot reach a teammate's laptop (shares are often one-way, laptops
accept no SSH and run no listener), so it does not push notices to them.
Instead, when a teammate's run finishes, fails, stops or receives a validation
result, the host keeps the notice a push would have carried in a per-sender
store (`~/dispatch/.member-notices/`), and the laptop pulls it over the same
forced-command key:

```sh
hev dispatch notices pull                          # every joined host
hev dispatch notices pull --host hev-node-EXAMPLE  # one host
hev dispatch notices pull --watch                  # keep pulling every minute
hev dispatch notices pull --watch=5m
```

Each notice goes into the local peers inbox of the run's `reply:` session with
a stable message ID, then is acknowledged on the host. A lost acknowledgement
redelivers the same ID, which the inbox deduplicates, so a session never sees
a notice twice. Notices for runs without `reply:`, or whose session has no
registered inbox on this machine (`hev peers inbox on SESSION`), are printed
instead and acknowledged. A delivery failure leaves the notice queued for the
next pull.

The laptop treats pulled notices as host data. Each one arrives from
`ALIAS/d-RUN`, where `ALIAS` is the local host alias from `hev nodes ls`, so a
host cannot pose as a local address. The reply must be a plain local session
name: a notice addressed to `HOST/NAME`, a pane or anything else is printed
instead. Notices with a malformed ID or empty text are reported as rejected and
acknowledged, so they cannot block the queue.

A teammate only ever sees notices for runs bearing their own sender name;
validator children and forwarded runs produce none. The host keeps at most 100
notices per teammate for 14 days and returns them in batches of 50; message
text is capped at 1000 characters (`hev dispatch show` has the rest).

For a teammate's runs the host skips notify templates addressed to `{reply}`,
because that session lives on the laptop (and could collide with an unrelated
session on the host). Templates without `{reply}`, such as an owner's own
alert sink, still run. Owner and unrestricted runs push exactly as before.
Polling from the laptop daemon (`hev up`) is a follow-up; until then, run
`pull --watch` in a spare terminal or a scheduler.

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
   case-insensitive, path case-sensitive, `.git` ignored, explicit ports
   significant) on whole path segments, with at least one segment beyond the
   prefix: `https://github.com/ExampleOrg/` admits
   `https://github.com/ExampleOrg/widget` but not
   `https://github.com/ExampleOrg.evil/widget`,
   `https://github.com/exampleorg/widget` or the organization itself
   (`https://github.com/ExampleOrg`, `https://github.com/ExampleOrg.git`).
   The transport must match the entry too, because it decides which host
   credential the clone uses: an `https://` entry admits only HTTPS URLs (the
   host's HTTPS credential helper), and an SSH entry such as
   `git@github.com:ExampleOrg` admits only SSH URLs (`git@github.com:ExampleOrg/widget`,
   `ssh://git@github.com/ExampleOrg/widget`) with the same SSH user (the host's
   SSH keys). List both forms to allow both.
3. Clones it into the host workspace as the last path segment without `.git`
   (spaces become `-`), using the host account's own git configuration and
   credentials. The teammate supplies only the URL: no git arguments, path,
   branch or ref. The clone runs with transports limited to https and ssh
   (`GIT_ALLOW_PROTOCOL`), hooks disabled (an empty `core.hooksPath`, no
   templates), no submodules, no Git LFS filters (a repository's `.lfsconfig`
   cannot redirect object fetches or credential lookups; run `git lfs pull` in
   the clone if a run needs the objects), no HTTP redirects (including
   URL-scoped `http.<url>.followRedirects` in host config), terminal,
   askpass and SSH password prompts disabled (`ssh -o BatchMode=yes`), at most
   10 minutes and 4 GiB. Git and everything it starts (transport and
   credential helpers) run in their own process group, or job object on
   Windows, which is killed on timeout and when the clone ends. It clones into
   a temporary directory first, so a failed or bounded clone leaves nothing
   behind. When git fails, a teammate gets a short refusal; git's own output,
   which can name host paths and helpers, goes to `~/dispatch/admission-clone.log` on the
   host for the owner.
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
credential helper. Credential helpers and anything else git starts run inside
the clone's job object and are terminated when the clone ends, so a helper
that expects to linger as a cache daemon starts afresh on the next clone.

## Compatibility and website follow-up

`hev nodes` replaces the old coordination-backed node registry CLI. `hev orders`
and `hev runs` have been removed. Consumers
of the former `nodes list/show` registry shape need to call their backend or
migrate; `nodes list` now aliases access listing and has a different shape.

On the website branch, share-a-host should document invite/accept/remove,
Windows administrator paths and the agent execution trust boundary.
Join-a-factory should document pinned host keys, join/request exchange, host
aliases and ownership restrictions. No website sources change here.

Short invite codes change these website pages (not edited here):

- share-a-host: `hev nodes invite NAME --address A --user U [--allow repos]
  [--administrators]` prints a short code and join line; `hev nodes pending`
  (or `ls`) shows the teammate's key fingerprint; approve with
  `hev nodes accept NAME`; the enrollment key exists only while an invitation
  is open; `--long` keeps the pasted flow.
- join-a-host: `hev nodes join USER@HOST CODE`; read the printed key
  fingerprint to the owner; no request to paste back; the "could not prove"
  error means a wrong code or an impostor.
- invited: replace the long code and request exchange with the code and the
  fingerprint check; mention lockout after 5 wrong codes and single use.

## How short codes work

The teammate has no key yet, so the exchange cannot ride a teammate's
forced-command key, and the host should not open a port. `hev nodes invite`
instead appends one more line to the owner's `authorized_keys`:

```text
restrict,command="\"/path/to/hev\" nodes enroll" ssh-ed25519 AAAA... hev-nodes-enroll
```

The key on that line is the same in every `hev` build and its private half is
public: `hev nodes join` uses it to open an SSH session that can only run
`hev nodes enroll` (no shell, PTY or forwarding). Anyone who can reach the
host's sshd can therefore start an enrollment while an invitation is open,
which is why everything rests on the code. The line is removed as soon as no
invitation is open (request recorded, locked, withdrawn), and on the next
`hev nodes` command after the last one expires; enrollment refuses expired
invitations in any case. It works wherever SSH already reaches the host,
including Windows OpenSSH, with no listener, firewall rule or daemon.

The code has a 2-symbol public slot (which invitation) and an 8-symbol
secret: 40 bits from the Crockford base32 alphabet (no I, L, O or U). Both
sides run a CPace-style PAKE on X25519: each derives a generator from the code
(Elligator2), sends an ephemeral public value, and derives a session key that
only a party knowing the code can compute. The host then sends the invitation
details (account, host key, expiry, access ID) with a key-confirmation MAC.
The member checks it before trusting anything, so a machine that does not
know the code (a spoofed address, a man in the middle on the bootstrap
connection, whose host key is deliberately not checked) cannot get its host
key pinned. Only then does the member send its new public key with its own
confirmation MAC, which the host checks before recording the request.

A recorded exchange does not let anyone test codes offline: checking a guess
needs the discrete log of an ephemeral value. Each online exchange tests one
guess and is counted before the host answers, so disconnecting early still
counts; after 5 the invitation locks. A guesser succeeds with probability at
most 5 in 2^40. Someone who can reach sshd while an invitation is open can
also lock it on purpose (a denial of service; the owner invites again).

Every change to an invitation record (the attempt count, the pending
request, consumption) is written to a sibling file, fsynced, renamed over the
record and followed by a directory sync before the host sends anything, so a
crash or power loss after an offer cannot forget the attempt. A record that
does not parse fails closed for that invitation only: `hev nodes ls` shows it
as `unreadable` with its file, it matches no code, and the owner deletes the
file. The member's bootstrap SSH session has a two-minute total deadline,
after which ssh is killed; only the first 8 KiB of its error output is kept;
and it switches off the member's own forwarding, agent, X11, local-command and
connection-sharing settings, since the server is unauthenticated until the
code checks.

When `authorized_keys` lacked a final newline, `hev` inserts one before its
first line and records that; whichever owned line (enrollment key or grant)
is removed last takes the newline out again, so the file returns to its
original bytes however invitations and grants overlap.

The pending request lives in the invitation's record (name, expiry, access
ID) with the member's key and fingerprint; `hev nodes accept NAME` installs
exactly that key with the usual `dispatch rpc --node ID` forced command, and
only before the invitation expires. Invitation records hold the code; protect
the nodes directory as before.

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

The separate [hevd cluster](cluster.md) listener is opt-in and uses one private TCP/UDP port. Node invitations and grants continue to use SSH and start no listener of their own; cluster membership grants no access.
