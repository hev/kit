# Local native jobs v1

This opt-in source bridge runs isolated Linux processes. It does not activate
execution in the capture daemon, schedule a DAG, install a service, authenticate
a remote caller, or supply a macOS process adapter. Native macOS and live
integration acceptance remain outstanding. Existing capture paths are unchanged.

`hev native-job --root /absolute/private/journal --host HOST --executor EXECUTOR`
reads one JSON call on stdin and writes `{ "version":"native-jobs/v1",
"result": ... }`. Nonzero exit means query the journal before any retry.
`pkg/nativejob` is the same API for an embedded authenticated gateway.
Only the trusted local enrollment owner may access the root or run these calls.
The root must be mode 0700 on durable local storage with working fsync. Do not
put it on ephemeral/container-local/network storage. Enrollment and format are
bound on first open; another enrollment or version fails closed. Deployments
must provision upstream authority/authentication and disk quotas before use.
No shell interpolation is performed by the bridge; explicitly submitting a shell
is an execution decision by the trusted caller. The child receives only the
submitted complete environment. Use absolute executable and working directory.

## Example and protocol

Run `python3 docs/native-jobs/example.py /path/to/hev /absolute/scratch` for one
real isolated Linux operation, lost submit ACK retry, journal reopen, identical
outbox replay, ACK retry and effect count one. It creates only its scratch root.

Every call has `version: "native-jobs/v1"` and one `action`. Unknown fields,
unsupported versions/actions and unused action fields are rejected.

| action | fields | result |
| --- | --- | --- |
| fence | fence | null |
| submit | request | Job |
| query | job_id (ID or intent key) | Job |
| execute | job_id | Job after synchronous process completion |
| recover | job_id | Job; orphaned Running becomes Uncertain |
| outbox | none | unacknowledged Event array |
| ack | job_id, identity, event_id, result_digest | null |
| reconcile | job_id, identity, inspection | Job |

The Go JSON tags in `pkg/nativejob/types.go` define the versioned wire fields.
Request requires `version`, `intent_key`, `payload_digest`, `identity`, `argv`,
`directory`, `environment`. The digest is `sha256:` plus lowercase hex of Go
`encoding/json.Marshal(Request)` with `payload_digest` set to the empty string.
HTML characters and U+2028/U+2029 use Go JSON escaping. Field ordering follows the Go declaration; use `RequestDigest` rather than
reimplementing serializer assumptions. Requests are immutable and exact duplicate
submissions return the same ID. A conflicting key is an error. Intent keys beginning `job-` are reserved and rejected. Query accepts
both durable ID and intent key; transport failures never imply permission to
resubmit a changed operation.

Identity requires every field: `order_uid`, `scope_digest`, `plan_digest`,
`authority_digest`, `execution_epoch`, `workflow_uid`, `node_id`, `step_id`,
`attempt_id`, `candidate_digest`, `input_digest`, `environment_digest`,
`operation` (only `exec`), `host_id`, `executor_id`, `chef_id`, `fence` (>0).
Digests are opaque exact identities supplied by the authenticated adapter;
this bridge does not reinterpret Order scope or independently grant authority.
P3 must bind node_id to its exact native task/wait correlation and preserve all
fields in its callback receipt. New effectful rework needs a new intent and
attempt, never a retry of execute. Cross-order scheduling is not implemented.

Fence is `{order_uid, chef_id, epoch, expires}` (RFC3339 UTC). Provisioning a
fence is a privileged local-owner operation, not evidence of upstream authority.
Epochs increase; identical replay is allowed; same-epoch lease/owner changes
and lower epochs fail. Submit and execute check current unexpired fence inside
the same serialized writer transaction as admission. A new chef must inspect
existing jobs before dispatch. Historical results retain their original identity.

## Durable states and recovery

SQLite WAL synchronous FULL commits persist intent (`Pending`) before submit
returns. Execute commits `Uncertain` and supervisor boot/PID/birth before spawn.
An interruption after this point never authorizes another execute. On spawn,
record child boot/PID/start ticks if observable and state `Running`. Fast child
exit can prevent birth observation; this never implies a safe replay. Execute
waits for exit, syncs stdout/stderr files and their directory, hashes both, then
atomically commits `Completed`, immutable Result and outbox Event. Failure to
commit any outcome leaves effects held. There is no automatic operation retry.

Query is a historical journal read. `recover` explicitly probes supervisor birth:
a missing/reused PID or changed boot moves Running to Uncertain. Matching live
supervisor ownership stays Running. A child still alive with a missing supervisor
is Uncertain; no new launch. The Linux adapter reads boot_id and /proc stat
starttime (including correct parsing of parenthesized process names), not PID
alone. Other platforms reject execute until a supported adapter is accepted.

An inspection `{id, evidence, inspector, result}` resolves only Uncertain jobs.
The caller must inspect host/process/effect evidence and supply the final result;
reconciliation cannot reset to Pending or trigger a spawn. A still-live supervisor or recorded child blocks reconciliation. Its identity must
match all original execution fields, except chef/fence may advance to the current
owner. Inspection IDs are immutable/idempotent. Retain external inspection
receipts and artifact evidence; the bridge cannot verify an operator's assertion.

Result is `{exit_code, artifacts:[{name,digest,size}], evidence?}`. Normal
execution produces stdout/stderr artifacts inside the job directory. Event is
`{version,event_id,job_id,identity,result,result_digest}`; digest is `Digest(Result)`.
The outbox has no transport sender: the gateway polls and sends identical events
until exact receipt ACK. Lost result ACK never reruns a process. ACK binds the
complete original identity, event ID and digest and remains valid for historical
results after fence transfer. It marks delivery only and cannot satisfy current
acceptance. P3 must check current scope/fence before resuming its exact wait.
A conflicting result/ACK is rejected. Results and ACKs remain in the journal.

The filesystem and local owner are trusted. This source does not promise
cross-host exactly-once execution, unbounded disk safety, process cancellation,
or survival of dishonest storage. Crash windows trade availability for safety:
manual inspection can be required even when no effect happened. Keep the journal,
output files and version-compatible readers throughout rollback; never downgrade,
delete or replay uncertain jobs.
