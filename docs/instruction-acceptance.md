# Cross-host instruction acceptance

Run from the capture implementation's checkout on both hosts. Use the existing
configured archive and authorized access. Do not copy credentials, alter daemon
configuration, or substitute the mini's hostname for the laptop's identity.
The live test writes two small synthetic versions to that archive and removes
its temporary local fixture afterwards. Archived fixture rows remain, by design.

## Actual laptop memory

On the laptop, use the implemented capture source through the authorized capture
process. Confirm that capture has indexed an actual existing memory entry. Choose
a nonsensitive phrase locally; keep the phrase and expected absolute path,
project, and laptop host in private evidence outside the repository.

Build the current CLI on the mini:

```sh
go build -o /tmp/hev-instruction-acceptance ./cmd/hev
```

Run `hev query` on the mini with the chosen phrase and save output privately
(umask 077). Use `--harness instructions --json` (the CLI does not yet support a host filter).
Check an exact matching hit with all three expected provenance fields; hybrid
search can return unrelated hits, so a successful exit alone is insufficient.
For example, after setting the variables privately:

```sh
umask 077
/tmp/hev-instruction-acceptance query "$MEMORY_PHRASE" \
  --harness instructions --top 100 --json > "$PRIVATE_HITS"
jq -e --arg path "$EXPECTED_PATH" --arg project "$EXPECTED_PROJECT" \
  --arg host "$EXPECTED_LAPTOP_HOST" \
  'any(.[]; .path == $path and .project == $project and .host == $host
    and .harness == "instructions" and (.version_id | length) > 0)' \
  "$PRIVATE_HITS" >/dev/null
```

Inspect the matching text privately to confirm it is the actual entry. Preserve
private evidence of the CLI head, both host identities, configured archive match,
and observation time. Publish only pass/fail counts.

## Isolated version edit

Run on the laptop, using its existing archive configuration:

```sh
HEV_LIVE_INSTRUCTIONS=1 go test ./cmd/hev \
  -run '^TestLiveInstructionVersions$' -count=1 -v
```

This uses the implemented source with a temporary home containing only a
synthetic memory fixture. It edits that file, checks two distinct hashes and
version IDs, verifies the old valid_to equals the new valid_from, and searches
both versions separately. It does not modify operator memory or shared daemons.
The test skips in ordinary CI and fails if configuration or store support is
unavailable. Running it on the mini is useful store validation, but does not
replace either laptop capture or the actual cross-host query above.

## Merge gate

Require the actual laptop-memory query on the mini, the isolated laptop edit
check, and the named `ci / go` workflow passing on the final PR head. Record
sanitized evidence privately. If authorized laptop access or laptop capture is
unavailable, report that operator-only blocker to the job's gaffer and leave
this PR unmerged.

## Acceptance record

The operator confirmed live acceptance on 2026-10-03 using acceptance head
`124cd35`:

- A query executed on the mini matched one actual laptop memory entry (1/1),
  with the correct absolute path, project, laptop host, instruction harness,
  and a nonempty version ID.
- The isolated laptop `TestLiveInstructionVersions` passed: two separately
  searchable retained versions and the exact old/new validity boundary.

Source text, identifying paths, and host names remain private. These are
operator-provided live results. The acceptance branch was then reconciled with
main's PR #33 projection fix, retaining the live test without duplicating the
implementation. The regression check now verifies every hosted query leg.
