# Upgrading an existing transcript archive

The first enabled `hevd` scan or `hev index` run after this upgrade removes old
content-addressed chunks, whole blocks, session prompts and summaries for each
session owned by the selected transcript source. It then indexes all available
turns through the scrubber. This also happens with `--read-side` and
`--summarize`: the initial upgrade writes embeddings and every tier, regardless
of `--limit`, saved signatures or the requested tier selection. Later runs use
the ordinary options. Dry runs do not delete or mark anything complete.

Stop older daemons before upgrading (as `hev up` does). Current CLI, daemon and
session-schema migration writers share a local file lock. This cannot stop an
older binary, another installation using a different config, or a remote writer
from adding raw rows. Upgrade all writers before treating a shared archive as
scrubbed. Search and dashboard readers may see a temporarily incomplete session
while cleanup and reindexing run; there is no multi-namespace transaction.

Cleanup uses exact session IDs, never a blanket namespace delete. Chunk
ownership is read from source paths, harness and host; present source turns also
supply session IDs. Other source roots and hosts stay intact. A session ID that
also has chunks attributed to another source is rejected before deletion.
Unattributable orphan chunks, blocks or session rows cause an error, rather than
a completion marker that silently leaves raw rows behind. Source ownership
cannot be recovered from a lone legacy block or summary: those rows lack a
source path. Run each source root that contributed to the archive, including
custom roots, to cover the entire archive.

## Missing and unrecoverable transcripts

If a transcript has aged off disk, but its archived chunks still identify its
source root, the upgrade **removes its archived chunks, blocks and summary**.
It cannot reconstruct the original turns and therefore does not recreate that
session. This applies when the entire source root has disappeared, too. Keep
source backups if historical sessions must survive an upgrade.

An existing file that cannot be read or parsed halts migration before cleanup.
Restore or repair it, then retry. For an orphan summary/block/chunk that cannot
be attributed, restore the original transcript under its original source root
and rerun. If that source is unrecoverable, explicitly remove the identified
session's rows from all three namespaces (`<archive>`, `<archive>-blocks` and
`<archive>-sessions`) with an exact `session_id` filter, then retry. Verify that
ID's ownership before deleting: blocks have no host or source-path attributes.
Do not delete an entire shared namespace to clear an orphan error. The error
names the session; the upgrade remains incomplete until ownership is restored
or those rows are explicitly removed.

## Retry and completion state

Mode-0600 `archive-redaction-<identity>.json` files beside the capture config
contain only ownership coordinates and completion state, never transcript text.
The identity hashes endpoint, credential/account, namespace, store, model,
host and absolute source root. Policy and fingerprint-salt identities are also
checked. Changing an archive target, credential or salt triggers another
migration; it does not inherit completion or unchanged signatures.

The journal is synced and atomically replaced **before** deletion. Every
session's chunks, blocks and summaries are deleted and queried again to verify
absence before scrubbed rows are written. Completion is synced only after every
write succeeds. An interrupted or failed run keeps the journal incomplete; a
retry deletes the same IDs again and rebuilds them, including IDs whose source
ownership rows were removed during the previous attempt. A failed completion
save also retries cleanup. Leave these journals in place until a retry succeeds.

`[capture] redact = false` preserves explicit opt-out: no upgrade cleanup is
performed. An opted-out write invalidates prior completion, so enabling
scrubbing later runs cleanup again. Restoring an old raw archive at the same
target requires deleting its matching completion journal before indexing;
there is no server-side archive-generation identifier. Do this only with
writers stopped. Source transcripts remain raw and detector limitations still
apply. Interrupted session-schema backups are scrubbed before replay, so they
cannot restore raw prompts or summaries after the upgrade.

CLI output and daemon logs name historical sessions removed because their
sources were missing. Reported redaction counts describe available turns read
in the upgrade, not the contents of sessions that could not be recovered.
