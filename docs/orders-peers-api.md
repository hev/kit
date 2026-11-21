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
