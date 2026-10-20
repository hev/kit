# Direct execution adapter

This optional module builds `hevd` without coupling core Kit to coordination
storage. Core `hev nodes`, `hev orders`, and `hev runs` delegate to `hev-control`;
trace compatibility commands share their original handlers. Existing native-job
and trace capture remain intact. No scheduler or background service is activated.

Build in this directory: `go build -o /absolute/output/hevd ./cmd/hevd`.
Private-module deployments use existing Git access and process-scoped GOPRIVATE;
no token is embedded. The module pins the exact Factory protocol and native Kit
implementation. Overrides for validation must identify exact archived sources.

Configuration is owner-selected JSON:

```json
{"node":"NODE","incarnation":"ENROLLMENT","journal":"/private/local/journal",
 "transport":"/absolute/sql-transport","codex":"/absolute/codex",
 "models":["EXPLICIT_MODEL"]}
```

`hevd observe --config FILE` publishes a short-lived capability observation.
`hevd execute --config FILE --run RUN` accepts only that assigned node/incarnation,
model allowlist and an exact clean source revision. The local FULL-sync journal
records identity before process creation. A bounded subprocess supervisor gives
each selected Codex invocation its recorded deadline, including after caller
loss. Credentials are not copied into assignments or the journal environment.
The model uses the host's existing authenticated account.

Normal calls go through `hev orders assign`. The configured backend executor map
selects this command and config; a lost response uses the same dispatch key. An
existing intent never authorizes another process. Successful completion records
a durable source-candidate receipt before cloud publication. Uncommitted output
is reported as failure. Recovery reads that receipt, never a later working tree.
A missing receipt or journal requires inspection, not an inferred success.

`hev runs recover` invokes the explicitly selected `hevd recover --config FILE`.
A dead supervisor yields Unknown even if a child still exists. It does not replay
effects or claim that a disconnected child stopped. Original stdout/stderr and
result digests stay in the native journal. Cloud loss prevents new admission;
completed bounded work retains a reportable local result. Publication is retried
explicitly by the owner. Reports cannot confer independent acceptance.

Limits: Linux/macOS execution only. No automatic checkout, distributed scheduler,
provider fallback, automatic reconciliation verdict, live peer identity bridge,
follow or cancel operation is supplied. Existing standalone peers is the single
communication backend; its native identity remains authoritative. A run's native
runtime ID and source identity are exposed without inventing a second peer ID.
A shared service installer, credentials and transport authentication remain
explicit deployment responsibilities. Source success is not installed readiness.
