# Database-fenced native receiver

The database action claim and native journal issuance are separate authorities.
A database lease number never becomes `Identity.Fence`, a new native chef, or a
replacement request. The original job ID, request digest, attempt, enrollment
and native issuance stay unchanged.

`nativejob.FinalSpawnGate` extends the existing `FinalStartGate` with:

```go
WithSpawn(context.Context, StartCheck,
    func(StartReceipt, func(context.Context) error) error) error
```

The gate holds the authoritative database exclusion and existing business and
node/parent maintenance admission while invoking the callback once,
synchronously. It supplies a mandatory live checker using that same database
transaction. The native journal persists ownership as Uncertain before entering
the gate, commits the start receipt, reacquires its writer exclusion, verifies
original ownership and native issuance, then invokes the checker immediately
before `cmd.Start`. The callback commits Running before returning. Process Wait
and result collection occur after all admission guards are released.

A database gate must reject its legacy `WithStart` entry point: an older native
receiver must fail closed rather than omit the last checker. Checker rejection,
cancellation or connection failure leaves Uncertain and cannot authorize replay.
The checker must not call the native Store: the journal writer exclusion is held.
It must honor the bounded context and never renew authority.

Database row locks exclude authority mutations; time still passes. The checker
must revalidate against server time after the durable receipt gap, rather than
reuse a successful preflight or cached deadline. A database reply and an OS
process start cannot be one atomic operation. Expiry or transport loss can occur
between that final reply and `cmd.Start`; this API does not promise otherwise.
Original journal uniqueness, durable uncertainty and inspection of exact effects
remain necessary after a lost acknowledgement, supervisor crash or failed commit.
A durable receipt alone does not prove that a child started.

The existing result/outbox/ACK protocol remains authoritative. Historical result
ACKs are independent of current execution permission; a DB Started acknowledgement
is neither process completion nor customer acceptance. Never clear a journal,
reset Uncertain to Pending, reissue a request, or retry a start after uncertainty.
