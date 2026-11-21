# RFC 0024 — Workspaces

A workspace associates subscriptions and authentication, a home directory, and
a private network with a cluster. Resolution happens before routing an order
or choosing a data store. An explicit unknown workspace is an error. Nested
directory matches choose the most specific canonical directory.

The host allowlist limits routing and inventory. Target hosts provision their
own workspace config and resolve credentials locally. Dispatch carries the
workspace name across the transport. A workspace's Layer configuration selects
its archive and board. Starter Kit uses separate directories for client data.
The default workspace retains the legacy bucket configuration.

The network and seed fields describe the workspace's cluster. Provision the
private network and configure its daemon independently on each member. See
[Workspaces](../workspaces.md) for configuration and command usage.
