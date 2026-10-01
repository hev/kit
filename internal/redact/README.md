Secret scrubbing is available in CE and enabled unless `[capture] redact = false`.
`Load()` follows `HEV_CONFIG`, otherwise `~/.hev/config.toml`. It persists a random
32-byte hex `capture.redact_salt` in a mode-0600 file through a locked, synced,
atomic replacement. Existing comments and settings are preserved for ordinary
`[capture]` tables; dotted or inline capture tables are normalized as TOML.
Invalid config or salt stops ingestion. Opt-out returns a nil scrubber, whose
methods leave text unchanged.

`New(salt)` builds an immutable scrubber. `Text` returns replacements and counts;
`Turns` mutates all turn strings, including blocks, titles and metadata.
`Counts.Add` merges replacement counts. `Version` identifies this policy for the
following archive migration. `index.Report.Redactions` and daemon status count
replacements in units read during the scan, including failed writes; skipped
units are not counted again. Counts are not an inventory of the archive.

The gitleaks v8.24.3 embedded text rules retain keyword and entropy thresholds,
with the same secret-group extraction. Trusted file paths, example allowlists
and `gitleaks:allow` comments do not exempt transcript text. Additional rules
cover Slack xox tokens, sk and tpuf tokens, PEM private keys and certificates,
JWTs, URL user/password credentials, Authorization values and high-entropy
assignments (at least 20 characters and Shannon entropy 3.5).

Replacement markers contain a 16-hex-character HMAC-SHA256 fingerprint, keyed by
the installation salt. The hash does not include the rule name, so identical
secret spans retain the fingerprint across rules. Overlapping spans are merged
and counted once under the first, longest rule; partially overlapping matches
hash the complete merged span. Existing markers are left alone. Keep the salt
stable: deleting or changing it changes future fingerprints.

This detector cannot guarantee coverage of arbitrary passwords, encoded secrets,
split tokens or unrecognized formats. Source transcripts remain raw. Scrubbing
starts before content-addressed chunks and read-side rows are constructed, and
model prompts are scrubbed before truncation. Stored summaries carried into a
new write and generated model output are scrubbed too.

Existing archives are rewritten by the first enabled `index.Run` or
`index.Summarize` for each Claude/Codex source root. See
[archive upgrade handling](../../docs/archive-redaction.md) for cleanup,
completion, retries, source loss and ownership errors. `Version` and
`Scrubber.Identity()` identify the policy and salt in migration journals.
