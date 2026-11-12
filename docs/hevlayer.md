# Capture and search with hev layer

kit feeds your traces into hev layer when it has a key. With no key, `hev up`
starts nothing in Docker: it writes the config, installs the capture daemon and
the skills, and capture writes to a file archive on the host. Search over that
archive is lexical; it matches words, not meaning. See
[the trace archive](trace-archive.md). The board follows the same rule
(see [the board's storage modes](board.md#two-storage-modes) and
[storage modes](design/storage-modes.md)).

A key saved in `[layer].api_key` counts as a key. A key that is configured but
cannot be read, such as a failing `op read` or a blank variable, is an error;
kit does not fall back to files.

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
to the new archive. Data does not move between the file archive and Layer on
its own; after adding a key, `hev backfill` sends the sessions still on disk.
Hosted setup does not remove local containers, volumes or archives. Stop a
previous local stack with `hev down` before switching if it is no longer needed.

## Retired: the keyless Docker stack

Older kit releases started a local Docker archive when there was no key. That
lane is gone, along with its `--store` choice. On a host whose config still
names the old store, `hev up` says so: traces now go to the file archive, and
hevd re-reads every transcript still on disk into it.
To keep traces in Layer instead, set `HEVLAYER_API_KEY` and use `hev backfill`.
kit does not stop, start or delete the old containers or volume. Stop them with
`docker compose -p hev-kit down`. Remove the volume with `docker volume rm` only
if you are sure you no longer need it.

Other kit settings use the same prefix: `HEVLAYER_EMBED_MODEL` chooses the
embedding model, `HEVLAYER_TELEMETRY=off` disables local gateway telemetry,
and `HEVLAYER_API_KEY_OP_REF` selects the optional 1Password key reference.
A key reference must be explicit; kit does not fall back to an implicit provider
credential in 1Password.
The local gateway retains its upstream environment contract internally; kit
translates its settings at that boundary.

## Advanced: bring your own Turbopuffer account

A provider account is an explicit store choice, separate from hosted hev layer:

```sh
export HEVLAYER_API_KEY=<your-gateway-key>
hev up --store turbopuffer
```

This starts a local gateway and dashboard in Docker, with the gateway writing
to your provider account. You can also use `hev init` to configure a direct provider endpoint
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
