# Directory filters on existing archives

`workdir` keeps full-text search and now declares `filterable: true`.
`harness` and `plan` are scalar strings without FTS and default to filterable;
the existing shared archive explicitly reports both as filterable.

Before each hevd scan, reconciliation reads the configured namespace schema.
This covers `hev-traces` and the hosted daemon's `kit-<slug>-traces` without
cross-tenant namespace discovery. It sends a schema-only v2 write containing
only affected attributes, copying each existing definition and changing only
`filterable`. It does not send rows, vectors, distance metrics, or a newly
chosen embedding model. Missing namespaces are created by ordinary ingestion.
Read/write failures skip the scan, are logged, and retry on the next tick.
An incompatible attribute type fails safely; this is not the sessions type
migration from board #65 and never deletes or rebuilds an archive.

Turbopuffer's [write documentation](https://turbopuffer.com/docs/write#updating-attributes)
explicitly supports online filterability changes and schema-only writes.
Queries depending on the new index can return 409 until indexing completes.
The request completing does not establish readiness. Enabling filtering adds
an inverted index. With both FTS and filtering, an attribute is billed at 200%
of its logical size, versus 100% with FTS alone. Only the workdir attribute's
index footprint increases; native embeddings are not regenerated.

## Safe provider probe, 2026-10-02

The configured endpoint was `https://aws-us-east-1.hevlayer.com`, backed by
Turbopuffer. Two synthetic rows in `kit-lyr244-v2probe-zw2yab` used `/lyr` and
`/other` workdirs. Eq failed with 400 under the original FTS schema, then a
schema-only `POST /v2/namespaces/<probe>` enabled filtering. The response
reported `rows_affected: 0` and `billable_logical_bytes_written: 0`.

The request took 0.306 seconds. The first Eq query succeeded 0.446 seconds
after request start and returned only the `/lyr` row; no 409 was observed.
This bounds observed readiness for two rows, not internal index build duration
or readiness for a larger archive. All other schema settings and both rows
were verified unchanged. Probe queries reported the provider's 1.28 GB minimum
billable logical bytes queried; do not describe polling as free.

[Redacted provider evidence](evidence/lyr244/provider-probe.txt) contains no
credentials or archive text. The recorded schema in
`internal/layer/testdata/lyr244-provider-schema.json` seeds Eq-evaluating
regressions for shared and hosted namespace names, with extra failure/retry,
409, custom schema preservation, incompatible-type and missing-namespace cases.
Both tiny synthetic namespaces are retained; no namespace was deleted.

Python's default User-Agent initially received Cloudflare 403 `error code:
1010`; the same authenticated read with Go's User-Agent succeeded. Native
`hev` also reached the provider. No credentials were changed.

## Existing shared archive acceptance

Before the change, the actual installed command
`hev query "x" --workdir /Users/hev/workspace/lyr --json` exited 1 with 400:
`attribute isn't filterable; likely because bm25 is enabled`.
The schema read confirms `workdir.filterable=false` and retains the full FTS
object (`word_v4` tokenizer); `harness.filterable=true`, `plan.filterable=true`.
Foreman, gaffer `mfk6q6`, and kit owner `9g9ghn` coordinated the change;
`zw2yab` was the sole executor after the benchmark reservation was explicitly
withdrawn. Existing launchd writer `com.hev.hevd` PID 95604 and serve PID 95608
were kept running. The actual reconciliation method succeeded in 1.015 seconds.
A complete before/after schema comparison showed only `workdir.filterable`
changed. Aggregate rows increased from 1,043,637 to 1,043,793 with ongoing
ingestion; this is not a frozen full row/vector census. The first actual CLI
query returned the documented 409 while indexing builds. Final readiness and
scoped-hit acceptance are pending below.

## Hosted coverage

Regression requests execute against an existing `kit-example-traces` schema
using the recorded provider settings and Eq semantics. The production path
uses the daemon's configured namespace and credential, without listing other
customers. This is hosted reconciliation coverage, not a claim that every
live hosted archive has been migrated. Existing daemons must run the new
binary; activation and shared restarts stay with their owners.
