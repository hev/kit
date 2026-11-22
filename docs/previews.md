# Private previews

`hev preview` builds and serves static site previews strictly on loopback or
Tailscale private network addresses. It replaces per-site shell and Python
preview scripts with one cross-OS command configured in `~/.hev/config.toml`
(or `HEV_CONFIG`).

## Configuration

Add one or more `[[preview]]` entries to `~/.hev/config.toml`:

```toml
[[preview]]
name = "kit-docs"
bind = "100.104.233.125"
port = 4370
repo = "https://github.com/hev/kit.git"
ref = "origin/v0.7"
subdir = "site"
out_dir = "dist"
keep_builds = 2
timeout = "10m"
env = { PUBLIC_KIT_APP_ORIGIN = "https://app.example.ts.net" }

[[preview]]
name = "local-static"
bind = "127.0.0.1"
port = 4380
root = "~/sites/preview"
```

### Fields and validation rules

Configure up to 64 previews per host. Unknown fields inside `[[preview]]` are
refused.

| Field | Required | Rules and defaults |
|---|---|---|
| `name` | yes | Unique; matches `^[a-z0-9][a-z0-9-]*$`; at most 128 bytes. |
| `bind` | yes | Literal IPv4/IPv6 address that must be loopback (`127.0.0.1` / `127.0.0.0/8`, `::1`) or Tailscale CGNAT (`100.64.0.0/10`). Wildcards (`0.0.0.0`, `::`), RFC 1918 LAN IPs (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), link-local IPs, IPv4-mapped IPv6 addresses, hostnames and public IPs are refused in both config validation and `serve`. |
| `port` | yes | TCP port in `1024..65535`. |
| `repo` | one of `repo` or `root` | Git remote URL or local repository path. Cannot start with `-` or contain NUL bytes. |
| `ref` | repo mode | Git ref to follow (default `"origin/main"` when `repo` is set, e.g. `"origin/v0.7"`). Refused when `root` is set. |
| `subdir` | optional | Relative path inside the repository (e.g. `"site"`). Cannot be absolute or traverse above the repo root. Refused when `root` is set. |
| `build_cmd` | optional | Explicit command argv (`[]string`). When omitted in repo mode, `sync` runs `pnpm install --frozen-lockfile` followed by `pnpm build`. When set, `sync` executes only `build_cmd` directly with no shell. Refused when `root` is set. |
| `out_dir` | optional | Relative output directory inside the build working directory (default `"dist"`). Cannot be absolute or escape the build directory. Refused when `root` is set. |
| `root` | one of `repo` or `root` | Static directory served directly without Git sync. Leading `~` or `~/` expands against the user's home directory. |
| `keep_builds` | optional | Number of completed builds retained under `builds/`. Default `2`; must be `1..5`. |
| `timeout` | optional | Go duration bounding the entire `sync` (fetch, checkout and build). Default `"10m"`; must be positive and at most `"30m"`. |
| `env` | optional | `map[string]string` of non-secret public build variables (at most 64 entries; keys match `^[A-Za-z_][A-Za-z0-9_]*$` and are case-insensitively distinct). Never store secrets here. |

## Commands

- `hev preview ls [--json]` lists configured previews, their listen address,
  mode (`repo` or `static`), state (`ready`, `not_built`, `missing_root`),
  current commit SHA and build timestamp.
- `hev preview show <name> [--json]` inspects one preview's configuration,
  active `preview.json` metadata, configured environment fingerprint and
  retained build count.
- `hev preview sync <name> [--force] [--json]` fetches `repo`/`ref`, builds if
  `(sha, env_fingerprint)` changed (or `--force` is set), atomically swaps
  `current`, and prunes old builds. Returns an error on static `root` previews
  or when another sync already holds the preview lock.
- `hev preview serve <name>` serves `~/.hev/previews/<name>/current` (or
  `root`) strictly on `<bind>:<port>` until interrupted (`SIGINT`/`SIGTERM`).

## Storage layout and atomic builds

Git-backed previews store their state under `~/.hev/previews/<name>/`:

```text
~/.hev/previews/<name>/
├── sync.lock           # OS file lock preventing concurrent syncs
├── source.git/         # Bare Git mirror for fetching ref updates
├── staging/            # Ephemeral isolated build workspace (cleaned after sync)
├── builds/<build-id>/  # Immutable build outputs, each containing preview.json
└── current             # Atomically swapped link to builds/<build-id>
```

On macOS and Linux, `current` is a relative symlink swapped with a single
atomic `rename(2)`. On Windows, `current.target` is written atomically and
`current` is updated without requiring elevated symlink privileges.

Each completed build contains `/preview.json`:

```json
{
  "schema": "hev.private-preview/v1",
  "site": "kit-docs",
  "repo": "https://github.com/hev/kit.git",
  "ref": "origin/v0.7",
  "sha": "0123456789abcdef0123456789abcdef01234567",
  "env_fingerprint": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "built_at": "2026-11-21T16:55:00Z"
}
```

### Sync guarantees

- **Skip unchanged builds.** When `current/preview.json` matches the resolved
  `sha` and `env_fingerprint`, `sync` exits without rebuilding.
- **No partial publishes.** A failed fetch, timed-out build or missing `out_dir`
  cleans up `staging/` and leaves `current` pointing at the previous good build.
- **Process-tree timeout.** Exceeding `timeout` terminates the entire build
  process group (Unix) or Job Object (Windows).
- **Bounded retention.** After a successful swap, old directories in `builds/`
  are pruned to at most `keep_builds` (never deleting the active build).
- **Redaction.** `preview.json` strips URL credentials, replaces local repo
  paths with `local:<name>`, and records only `env_fingerprint` (never raw
  environment values). Build and Git error output is bounded and scrubbed of
  secrets and local paths.

## Serving behavior

`hev preview serve <name>` resolves `current` on every request, so `sync`
updates take effect immediately with no server restart.

- Answers any `Host` header (Tailscale MagicDNS names, tailnet IP, loopback).
- Serves `GET` and `HEAD` only (`405 Method Not Allowed` for other methods).
- Serves `index.html` cleanly for directory paths (redirecting `/dir` to
  `/dir/`) and returns `404 Not Found` for directories without `index.html`
  (directory listings are disabled).
- Sets `X-Content-Type-Options: nosniff` and
  `X-Robots-Tag: noindex, nofollow` on every response.
- Refuses path traversal (`..`, URL-encoded `%2e%2e`, backslashes, NUL bytes)
  and symlinks pointing outside the active build root with `403 Forbidden`.
- Returns `503 Service Unavailable` when a Git-backed preview has not been
  synced yet.

## Running continuously

Pair `hev preview serve <name>` under your host service manager (launchd,
systemd or a Windows service) with a `[[cron]]` entry to poll for updates:

```toml
[[cron]]
name = "sync-kit-docs"
every = "5m"
cmd = ["hev", "preview", "sync", "kit-docs"]
timeout = "12m"
```
