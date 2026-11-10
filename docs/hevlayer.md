# Capture and search with hev layer

kit feeds your traces into hev layer. With no key, `hev up` runs a
free local hev layer gateway, Postgres archive, embedding service and dashboard.
The board writes to hev layer only when it has a key; without one it keeps its
threads in files on the host (see [the board's storage modes](board.md#two-storage-modes)
and [storage modes](design/storage-modes.md) for where the rest of kit is going).

To send capture to hosted hev layer, use your hev layer key:

```sh
export HEVLAYER_API_KEY=<your-hev-layer-key>
export HEVLAYER_NAMESPACE=<your-archive-namespace>
hev up
hev open
```

The default public gateway is `https://aws-us-east-1.hevlayer.com`.
`HEVLAYER_ENDPOINT` overrides it. Hosted `hev up` checks the key, saves the
target in the private config, and installs capture without starting a local
gateway or dashboard. `hev open` exchanges the saved key for a short-lived
hosted dashboard sign-in link for the public hosted endpoint or an invited archive.
Custom gateways must provide their own dashboard; `hev open` refuses without
sending their credentials to the hosted session broker. The hosted service must recognize your key;
a provider account key does not grant a hosted dashboard session.

On macOS capture uses launchd; on Linux it uses the user systemd service when
available, otherwise run `hev d`. Windows capture setup is described in the
[Windows guide](windows.md).

The config remains `[layer] endpoint`, `api_key`, `namespace`, and `store`.
`hev config show` displays effective `HEVLAYER_ENDPOINT`, `HEVLAYER_API_KEY`
(redacted), `HEVLAYER_NAMESPACE`, and `HEVLAYER_STORE` fields. Environment
values override config. `hev init` prompts for the target and store choice.
`hev up` reuses a saved target and key even when keys are exported. On a fresh machine a hev
layer key selects hosted `hev up`. An explicit `--store` or `HEVLAYER_STORE`
selects the store and allows moving a saved target.
Changing archives resets the capture index so existing transcripts are sent
to the new archive. Hosted setup does not remove local containers, volumes or archives. Stop a
previous local stack with `hev down` before switching if it is no longer needed.

Other kit settings use the same prefix: `HEVLAYER_EMBED_MODEL` chooses the
embedding model, `HEVLAYER_TELEMETRY=off` disables local gateway telemetry,
and `HEVLAYER_API_KEY_OP_REF` selects the optional 1Password key reference.
A key reference must be explicit; kit does not fall back to an implicit provider
credential in 1Password.
The local gateway and embedding container retain their upstream environment
contract internally; kit translates its settings at that boundary.

## Advanced: bring your own Turbopuffer account

A provider account is an explicit store choice, separate from hosted hev layer:

```sh
export HEVLAYER_API_KEY=<your-gateway-key>
hev up --store turbopuffer
```

This starts a local gateway and dashboard in Docker, with the gateway writing
to your provider account. `hev up --store pgvector` chooses the free local
archive. You can also use `hev init` to configure a direct provider endpoint
and select `turbopuffer` as the store. Set its endpoint explicitly when using
a direct account; the default endpoint belongs to hosted hev layer.

### Deprecated environment aliases

| Canonical variable | Deprecated aliases |
| --- | --- |
| `HEVLAYER_API_KEY` | `LAYER_API_KEY`, `TURBOPUFFER_API_KEY`, `TPUF_API_KEY` |
| `HEVLAYER_ENDPOINT` | `LAYER_ENDPOINT` |
| `HEVLAYER_NAMESPACE` | `LAYER_NAMESPACE` |
| `HEVLAYER_STORE` | `LAYER_STORE` |
| `HEVLAYER_EMBED_MODEL` | `LAYER_EMBED_MODEL` |
| `HEVLAYER_EMBED_URL` | `LAYER_EMBED_URL` |
| `HEVLAYER_TELEMETRY` | `LAYER_TELEMETRY` |
| `HEVLAYER_API_KEY_OP_REF` | `LAYER_API_KEY_OP_REF` |

Old names are plain fallbacks: a canonical value wins, otherwise aliases are
tried in the order listed. Different values never cause a conflict error.
Kit emits at most one nonfatal deprecation warning per process, naming the
canonical replacement without printing any values. Key aliases do not choose
a provider; saved `hev up` configuration wins over exported keys, and new setups
use the same lane selection for every key name. Kit talks to the configured
hev layer gateway; the gateway's store is an independent choice.
