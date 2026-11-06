# Transactional coordination client

This package calls a supplied PostgreSQL `kitchen` API at version 1. It supplies the opt-in bounded PostgreSQL controller and does not schedule work. The database is responsible
for claims, server-clock leases, monotonic fences, deduplication and recovery.
Apply compatible numbered schema migrations with a separate trusted migration
role before using the client. Enrollment requires the enrollment role; ordinary
runtime connections have SELECT and function EXECUTE, without table writes.

`New(ctx, pool)` checks `api_version()` before returning a store. The caller owns
and closes the pool. For a reference-configured connection, `Open` accepts explicit
`op://` references and a resolver. An explicit nonsecret `DatabaseName` may replace
the database reference only; host/user/password still require references. It resolves values only in memory, enforces
verified TLS with system roots and rechecks the API on new connections. The
returned pool must be closed. `OPResolver` uses existing `op` authentication; an
optional token-file path is supplied by the caller. No default host, vault,
credential file, managed service or environment configuration is assumed.
Errors expose SQLSTATE only; `errors.Is` recognizes `ErrStaleFence`,
`ErrStaleGeneration`, `ErrNotFound`, `ErrConflict`, and `ErrInvalidState`.
Connections use parameterized queries without prepared-statement/session state.

`Transaction` calls its callback once. Use it to atomically record an intent,
event and outbox entry. Do external work only after the transaction returns
successfully. `ClaimAction` commits a claim before returning it, and includes
the attempt number required by `FinishAction`. When called inside `Transaction`,
the claim is committed by the enclosing transaction instead.

No calls transparently retry. A connection error, timeout or lost commit response
can leave an unknown outcome. Replay only calls whose schema contract supplies
a stable dedupe key (same complete content), or inspect durable state first.
Acquiring a lease, claiming work, enrollment, and finishing an external effect
must not be blindly replayed. Enrollment mints a new immutable identity on every
successful call; reconcile a lost enrollment response before enrolling again.
Expired action/outbox claims become uncertain; inspect the external target and
record reconciliation evidence. Outbox finalization retains the v1 SQL contract:
it takes an ID, not a claimant token; callers must arrange exclusive relay
ownership. This client does not strengthen the external target's guarantees.

Run the unit checks with `go test ./internal/controlstate`. Integration tests
start and destroy their own local PostgreSQL cluster and use synthetic data:

```sh
CONTROL_PG_BIN=/path/to/postgresql/bin \
CONTROL_SCHEMA_DIR=/path/to/compatible/numbered/sql \
CONTROL_PG_REQUIRED=1 go test -race -v ./internal/controlstate
```

The directory must contain both the v1 base schema and enrollment/grant migration.
No existing DSN is accepted by the tests. Missing integration prerequisites cause
a visible skip, or a failure with `CONTROL_PG_REQUIRED=1`. Coverage includes
concurrent claims, delayed attempt reports, stale fences, restricted roles,
transaction rollback, killed-connection retry, messages/acks, outbox, enrollment,
assignment and version mismatch. Local PostgreSQL evidence does not establish
managed-service failover, real transaction-pooler compatibility or live cutover.
