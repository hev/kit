# Never-started Pending retirement

`(*Store).RetirePending(id string, current Identity, in PendingRetirement) (Job, error)`
accepts `PendingRetirement{ID, Evidence, Inspector}`. It does not issue SDK fences,
execute, reconcile processes or authorize upstream business settlement.

The caller authenticates enrollment and never-started evidence. Under SQLite
writer exclusion, the SDK requires the original Chef, an identity identical to
the original except for a strictly higher currently issued live SDK fence, and
an intact request digest. This is SDK issuance, not a PG or maintenance fence.

Only pristine Pending with no owner, child birth, start receipt, result, event,
ACK, inspections, retirement receipt or outbox may transition. The original
request is unchanged. Result is fixed exit125 with an empty artifact list and the
provided evidence. Completion and original-identity outbox commit atomically.
The live fence is rechecked immediately before commit. Execute and Reconcile
are unchanged.

`Job.pending_retirement` is an additive durable discriminator. Reentry requires
that exact retirement proof, terminal result/digest/event and matching original
outbox/ACK state; an unrelated Completed125 is not retirement reentry. Historical
ACK still uses the original request identity. Reentry itself requires a live
higher issuance of the same original Chef. Missing evidence, changed proof,
claimed/uncertain/running jobs and any conflicting terminal state hold.

After a lost reply, query the original job/event and use exact retirement
reentry, never reset or execute the old attempt. This source API adds no live
CLI, receiver operation, migration, fence issuance or installation.
