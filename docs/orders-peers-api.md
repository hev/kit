# Orders and peers backend adapters

An application serving the versioned orders and peers APIs passes its
authenticated team to the backend. The operator configures the team-to-host
routes. Request fields never supply an SSH destination or grant access to a
host. Give each team an isolated execution account and peers inbox store.

Order operations use dispatch RPC. Repository admission and launch settings
come from the receiving host. Peer deliveries target registered inboxes and
bind each message to the recipient incarnation.

The hosted transport can use an outbound host connection with a separately
enrolled machine identity. A host still runs the same local operations as a
self-hosted adapter. Network access to its SSH server is unnecessary.

Run `hev dispatch relay --host HOST` under the enrolled execution account.
The command performs one outbound poll using that account's Club Kit
sign-in. An explicitly provisioned key can be supplied through
`HEV_API_KEY` with `--origin`. The host must have a non-empty
`[admission].repos` allowlist. Use `--sync-peers` on a periodic poll to publish
registered local inbox peers and their consume and acknowledge receipts.

Relay operations use the execution account's selected workspace. For an
isolated workspace, pass the API origin and provision its dedicated key
instead of relying on the default Club Kit sign-in.

The API refuses a stop on an ended run and keeps its terminal result readable.
An explicit API purge removes the ended run record and its worktree, subject
to Git's worktree removal checks. The branch stays available. Ordinary
`hev dispatch clean` keeps the receipt for an admitted run.

The orders API reports the requested validation mode separately from the
validation result and exposes the verdict label. A native harness outside
the v1 enum is omitted from the structured summary.

The self-hosted sent box reads outgoing held messages from current local
recipient inboxes in the selected workspace, most recent first. Its inbox
counts still describe the queried peer's inbound inbox.
