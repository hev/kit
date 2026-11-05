# Capture and search with hev layer

kit feeds your traces and board into hev layer. With no key, `hev up` runs a
free local hev layer gateway, Postgres archive, embedding service and dashboard.

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
hosted dashboard sign-in link. The hosted service must recognize your key;
a provider account key does not grant a hosted dashboard session.

On macOS capture uses launchd; on Linux it uses the user systemd service when
available, otherwise run `hev d`. Windows capture setup is described in the
[Windows guide](windows.md).

The config remains `[layer] endpoint`, `api_key`, `namespace`, and `store`.
`hev config show` displays effective `HEVLAYER_ENDPOINT`, `HEVLAYER_API_KEY`
(redacted), `HEVLAYER_NAMESPACE`, and `HEVLAYER_STORE` fields. Environment
values override config. `hev init` prompts for the target and store choice.
An exported key selects hosted `hev up`; an explicit `--store` selects a local
stack. With no exports, a saved hosted target or local store is reused.
Changing archives resets the capture index so existing transcripts are sent
to the new archive. Hosted setup does not remove local containers, volumes or archives. Stop a
previous local stack with `hev down` before switching if it is no longer needed.

Other kit settings use the same prefix: `HEVLAYER_EMBED_MODEL` chooses the
embedding model, `HEVLAYER_TELEMETRY=off` disables local gateway telemetry,
and `HEVLAYER_API_KEY_OP_REF` selects the optional 1Password key reference.
The local gateway and embedding container retain their upstream environment
contract internally; kit translates its settings at that boundary.

## Advanced: bring your own Turbopuffer account

A provider account is an explicit store choice, separate from hosted hev layer:

```sh
export HEVLAYER_API_KEY=<your-provider-key>
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

Aliases still resolve, with one warning per alias per process naming its
replacement. Equal values are allowed. A canonical name takes precedence in
lookup, but conflicting nonempty values fail before startup and name both
variables without disclosing their values. Conflicting aliases also fail.
Key aliases select the same hosted lane as the canonical key; users retaining
a provider account should select `--store turbopuffer` explicitly. Launchers
using the old names remain readable; migrate names together rather than
leaving different credentials under old and new names.
