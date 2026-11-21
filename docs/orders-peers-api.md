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
