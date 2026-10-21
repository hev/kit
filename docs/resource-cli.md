# HEV resource CLI source contract

The existing `hev` executable now accepts resource verbs alongside trace commands:

```sh
hev api-resources -o json
hev apply -f resource.yaml --dry-run -o yaml
hev apply -f resource.yaml -o json
hev get orders --selector team=platform --limit 100 -o json
hev get order/example -o yaml
hev describe order/example
hev query 'previous validation'
```

The backend is the `factory` executable on PATH; `HEV_RESOURCE_BACKEND` optionally
selects an executable path, never a shell command. It must implement the matching
resource contract. The bridge preserves argument boundaries, stdin, stdout,
stderr and exit code. Factory owns transport, host identity, admission, storage
and capability discovery; hev does not copy state or credentials and does not
start a daemon or a controller. No private overlay dependency is introduced.
An absent backend returns structured `ResourceBackendUnavailable`; an old backend
must be upgraded explicitly, never replaced by a silent local database.

The current Factory candidate serves staged `factory/v1alpha1` Order resources:
apply/get/list/describe, exact UID/resourceVersion CAS/generation, labels/selectors,
JSON/YAML, read-only conditions/observedGeneration. Dry-run has zero effects;
apply does not activate execution or import live orders. `logs`, `watch`, `wait`
are routed but explicitly unsupported for Order declarations until real durable
log identity/observation/cursor APIs are integrated. Discovery states these limits.
No Host/Line resources or destructive imitation verbs are supplied. Bare
`hev watch` retains the existing interactive trace browser; a resource target
selects resource observation. `hev browse` and `hev tui` are unchanged.

Use neutral tooling vocabulary: resource, workflow, execution, executor,
controller/reconciler and event. Preserve Order API kinds and persistent IDs.
`factory order resource` aliases and old live-order commands remain compatible.
This is technical implementation guidance; final narrative/site copy is separate.

## Packaging proposal (not published)

The maintained Kit release builds the same `cmd/hev` into its existing Darwin
archives. `hev/homebrew-tap` currently has `Formula/kit.rb`, generated from
`.github/kit.rb.tmpl`, and installs `bin/hev`. There is currently no `hev` formula
or alias. The smallest compatible proposal is `Aliases/hev -> ../Formula/kit.rb`
in the existing tap, preserving existing `hev/tap/kit` upgrades. After explicitly
adding the tap, `brew install hev` can resolve that alias; an untapped machine
still needs `brew tap hev/tap` or qualified `brew install hev/tap/hev`.

A release must provide the matching Factory backend, either an explicit
`hev/tap/factory` formula dependency or a documented separate install for clients
using a preconfigured backend. This candidate changes neither tap publication
nor generated release automation and makes no installation claim. Installer owner
must resolve this proposal against the exact accepted Factory/Kit pair, preserve
rollback binaries, and verify discovery and trace commands after installation.
No tag, public release, global install, daemon replacement or shared rollout is
part of this source change.

Run the executable example against an exact locally built pair:

```sh
go build -o /tmp/hev-candidate ./cmd/hev
python3 scripts/test-resource-cli.py /tmp/hev-candidate /path/to/factory-candidate
```

The script owns and removes one scratch store and uses a shell-script backend
adapter solely to select Factory's explicit `--local` mode. It never retargets
shared configuration, starts daemons, or touches live resource records.

`hev controller status --store /absolute/existing/journal [-o json|yaml]` reads
an explicitly selected, existing P4 opt-in controller journal. No default store
or activation is implied. It reports the accepted canonical owner/fence/deadline,
handled attention cursor, sequence and attention/observation counts; machine
output preserves the P4 snapshot. Missing state fails without initializing a
lock/authority. This controller API is not a new CRD, a health check of installed
services, or an execution watch. Order logs/watch/wait remain unsupported.

## Operational orientation

Bare `hev` shows a read-only overview of the existing `hev nodes`,
`hev orders` and `hev peers` surfaces; `hev --json` emits versioned section
data and errors. It reuses configured coordination and peer executables, with
one concurrent, bounded read per section. Selected-node scope, order list-window
limits and peer snapshot freshness are explicit. Missing or failed observations
are unknown; other sections still render. Exit 1 signals any partial failure,
and exit 0 means all reads succeeded. Help stays offline. See the
[README](../README.md#commands) for the schema and bounds. Use `hev browse`
for the interactive trace browser.
