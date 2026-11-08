# HEV resource CLI

## Resource verbs: removed pending a native implementation

`hev apply`, `hev get`, `hev describe`, `hev logs`, `hev wait`,
`hev api-resources` and `hev controller` have been removed. They passed every
argument to the external `factory` executable, which has been retired, so all
they did was print its end-of-life notice. They are now unknown commands.

The get/apply/describe verb shape is still wanted. It will return on top of
kit-native resources (RFC 0014) that read kit's own records, with no external
backend. Until then, use the native commands:

- `hev dispatch` for orders and runs
- `hev nodes` and `hev admission` for hosts and access
- `hev peers` for agent sessions
- `hev watch` for source watches
- `hev traces` for captured sessions

The hidden `hev _resource-observe node` probe stays. It is implemented natively
(`pkg/nodeinventory`) and the Windows CI smoke test uses it.

## Operational orientation

Bare `hev` shows a read-only overview of the `hev peers` surface; `hev --json`
emits versioned section data and errors (version 2 has only `peers`; version 1
also carried nodes and orders read from a now-removed coordination backend).
It reuses the configured peer executable (`HEV_PEERS_BACKEND`, native peers by
default) with one five-second read and no retries. Bounded peer task titles make
current work visible, and peer snapshot freshness is explicit. A missing or
failed observation is reported as unknown. Exit 1 signals a failed read and
exit 0 means it succeeded. Help stays offline. Use `hev browse`
for the interactive trace browser.

## Packaging

Kit releases build `cmd/hev` for macOS and Linux (arm64/amd64) and Windows
(amd64). The tap generates `Formula/hev.rb` and a dependent compatibility
`Formula/kit.rb`. Install with `brew install hev/tap/hev`; existing users run
`brew upgrade hev/tap/kit`. Bare `brew install hev` requires homebrew-core.
`hev` no longer needs a companion backend binary.
