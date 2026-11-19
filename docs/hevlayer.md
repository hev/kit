# Capture and search with hev layer

kit feeds your traces into hev layer when it has a key. With no key (Starter
Kit), `hev up` starts nothing in Docker: it writes the config, installs the
capture daemon, the dashboard (`hev serve`, on 127.0.0.1:8099) and the
skills, and capture writes to a file archive on the host. Search over that
archive is grep; it matches words, not meaning. See
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
Hosted setup does not remove the file archive. On the hosted lane `hev down`
stops hevd; with no key it also stops the dashboard service.

## Retired: the keyless Docker stack

Older kit releases started a local Docker archive (Layer CE on Postgres) when
there was no key. That lane is gone, along with its `--store` choice. On a
host whose config still names it, `hev up` moves it to files: it exports
every row from the old gateway when the lane's containers start, and
otherwise re-indexes the transcripts still on disk and prints how many
sessions could not be recovered. See
[the trace archive](trace-archive.md#the-retired-local-docker-lane).
To keep traces in Layer instead, set `HEVLAYER_API_KEY` and use `hev backfill`.
kit never removes the old containers or volume. Stop them with
`docker compose -p hev-kit down`. Remove the volume with `docker volume rm` only
if you are sure you no longer need it.

Other kit settings use the same prefix: `HEVLAYER_EMBED_MODEL` chooses the
embedding model, and `HEVLAYER_API_KEY_REF` selects the optional key reference, read through
kit's secrets cache: `op://vault/item/field` (1Password) or
`azkv://vault/secret` (Azure Key Vault; see [secrets](secrets.md#choosing-a-provider)).
`HEVLAYER_API_KEY_OP_REF`, its older name, still works and takes either scheme;
the new name wins when both are set.
A key reference must be explicit; kit does not fall back to an implicit provider
credential in 1Password.

## Bring your own Turbopuffer account

Bring-your-own Turbopuffer is hosted-only now: kit no longer runs a local
gateway and dashboard in Docker in front of a provider account, and
`hev up --store turbopuffer` says so. A config that lane wrote (a loopback
endpoint with `store = "turbopuffer"`) is refused by `hev up` until you
choose: `hev up --store files` keeps traces in files (and drops the provider
key from the config), or `HEVLAYER_API_KEY=<hev layer key> hev up --store
hosted` moves to hev layer. The old containers are left alone. A hev layer
gateway of your own in front of Turbopuffer, configured with `hev init`, is a
Layer target like any other.

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
