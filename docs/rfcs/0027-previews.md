# RFC 0027 — Private previews

Status: Implemented

## Summary

Provide a cross-platform, private-network-only static site preview primitive in
`hev` (`[[preview]]` in `~/.hev/config.toml` and `hev preview {ls,show,sync,serve}`),
replacing per-site shell and Python preview scripts with one bounded, atomic Go
implementation.

## Motivation

Previewing internal documentation and staging sites across macOS, Linux and
Windows previously relied on bespoke per-repo scripts (`hev-preview.py`,
`docs-preview.sh`, `twc-stage`). Each script reimplemented git polling, atomic
swaps, retention pruning, locking and static HTTP serving with different bind
checks, platform assumptions and failure modes. A single native command in `hev`
enforces private-only binding structurally, works across operating systems,
bounds build execution and disk retention, and composes directly with `[[cron]]`
or host service managers.

## Decision

Configure previews with `[[preview]]` entries in `~/.hev/config.toml` (or
`HEV_CONFIG`) and manage them with `hev preview {ls,show,sync,serve}`.

- **Private-only bind guard.** Every preview must specify a literal loopback
  (`127.0.0.1` / `127.0.0.0/8`, `::1`) or Tailscale CGNAT (`100.64.0.0/10`) IP
  address and an unprivileged port (`1024..65535`). Wildcard addresses
  (`0.0.0.0`, `::`), LAN RFC 1918 addresses, link-local addresses, hostnames
  and public IPs are refused both during config validation and inside `serve`.
- **Atomic bounded sync.** `hev preview sync <name>` acquires a per-preview OS
  file lock, fetches the configured Git ref into `~/.hev/previews/<name>/source.git`,
  hashes `(sha, env_fingerprint)` to skip unchanged builds, builds into an
  isolated staging directory under a strict process-tree timeout, writes
  `preview.json` (`hev.private-preview/v1`), atomically switches
  `~/.hev/previews/<name>/current`, and prunes old builds to `<= keep_builds`.
- **Safe static serving.** `hev preview serve <name>` resolves `current` (or a
  static `root`) per request so swaps need no restart, answers any `Host`
  header (including MagicDNS names), serves clean directory `index.html`
  without directory listings, emits `X-Content-Type-Options: nosniff` and
  `X-Robots-Tag: noindex, nofollow`, and refuses path traversal (`..`,
  backslashes, NUL bytes) and symlink escapes outside the active build root.
- **No secret or local-path leaks.** `preview.json` carries a sanitized repo
  identifier and an `env_fingerprint` digest rather than credentials, raw
  environment values or local paths. Subprocess error output is bounded and
  scrubbed before surfacing.

## Goals

- Unify private site previews into one cross-OS `hev preview` command surface.
- Prevent accidental LAN or public internet exposure structurally.
- Guarantee zero half-built states on failed or concurrent syncs and cap disk
  retention.
- Work cleanly with `[[cron]]`, systemd, launchd and Windows services.

## Non-goals

- Public hosting, TLS termination, ACME certificates or reverse proxying.
- Dynamic application servers or server-side rendering runtimes.
- Managing secrets for preview builds (preview `env` is for non-secret public
  build variables only).
- Built-in daemon supervision of preview listeners (use `[[cron]]` for periodic
  `sync` and the host service manager for `serve`).

## Vocabulary

- **Preview**: one `[[preview]]` entry naming a static site built from a Git
  ref or served from a local directory.
- **Env fingerprint**: the SHA-256 hex digest of the sorted `env` key-value
  pairs configured for a preview.
- **Active build**: the build directory referenced by
  `~/.hev/previews/<name>/current` (or `root` for static-directory previews).

## Proposed surface

See [private previews](../previews.md) for `[[preview]]` configuration,
validation rules, `preview.json` schema, storage layout and CLI behavior.

## Open questions

- Whether a later phase should let `hevd` optionally supervise configured
  `[[preview]]` listeners alongside `[[cron]]` schedules.
- Whether optional fallback refs (such as following a train branch after a
  temporary docs branch is deleted) should be added to `[[preview]]`.
