# Carry over runs from a retiring loop

Use this when a loop or orchestrator that follows dispatched runs is being
retired, and the runs it holds must keep moving. Kit adopts them in two ways:

- runs that the standing follow-through's selector already matches need no
  spec change; their notes go into its seed checkpoint if it has not run a
  round yet, or into its inbox if it has (step 5);
- every other held run goes to a short, expiring carry-over spec that names
  it explicitly and grants no extra authority.

Example files: [examples/carry-over.md](examples/carry-over.md) and its seed
[examples/carry-over.checkpoint.md](examples/carry-over.checkpoint.md). The
standing follow-through is described in
[follow-through-orchestrator.md](follow-through-orchestrator.md).

## 1. Pause the retiring loop

Pause it, do not delete it: its state is the rollback. Two followers acting on
the same run would send duplicate reworks. Record when you paused it.

## 2. List the held runs and split them by scope form

```sh
hev dispatch ls --all --json > held.json
```

A spec's `manages.runs` takes exactly one form, a `selector` or `names`.
A spec with both is refused at apply (`choose one runs selector or names`),
so split the held runs into two disjoint lists:

| Held run | Goes to |
|---|---|
| Its order's `issue` matches the standing follow-through's `issue=GLOB` | `selected.txt`: nothing to change; the follow-through manages it already |
| Anything else | `names.txt`: the carry-over spec's `manages.runs.names` |

A run must not appear in both. If `names.txt` is empty, skip the carry-over
spec and seed only the follow-through (step 4).

## 3. Write the carry-over spec

```yaml
kind: orchestrator
name: carryover-20261106
goal: Finish or hand back the runs the retiring loop held that the standing follow-through does not select. Start nothing new.
mode: loop
every: 15m
until: 2026-11-20            # expires at 00:00 UTC on this date
max_lifetime: 14d            # or 14 days after apply, whichever is first
harness: claude
reply: owner-session
manages:
  runs:
    names: [fix-login-timeout, docs-install-refresh]
authority: [say, rework, notify]
budget: {concurrent_runs: 1, runs_per_day: 4}
idle: skip
```

- **Expiring.** Set both a near `until` date and a short `max_lifetime`; the
  earlier one ends it. At expiry the orchestrator becomes `complete` and the
  owner gets a lifetime notice. Runs still open stay ordinary dispatch runs.
- **No extra authority.** Grant no more than the standing follow-through
  holds, and never `dispatch`: a carry-over finishes existing runs and starts
  nothing. The retiring loop's grants do not transfer; anything it was
  allowed to do and this spec does not grant goes back to a person.
- **Fixed scope.** `names` never grows. A new run belongs to the standing
  follow-through.

## 4. Prepare each seed checkpoint

Convert the retiring loop's last checkpoint into Markdown, one file per
orchestrator you seed: the follow-through's file covers `selected.txt`, and
the carry-over's covers `names.txt`. Kit requires `Goal status`,
`Open threads` and `Next action` sections (as headings or labels); include
named waits with release conditions and notes for `reply` as well.

While converting:

- **Strip authority claims.** Remove lines that grant or assert permission,
  such as "approved to merge", "may deploy" or "has write access". A seed is
  evidence, not authority. The spec alone decides authority.
- **Drop expired mandates.** A time-boxed instruction past its expiry is
  removed, not carried. A mandate that is still live becomes its own expiring
  spec with `names` scope, never checkpoint text.
- **Record provenance.** Start the file with the source path, its sha256 and
  the conversion time, and say that the content is evidence, not authority.
- **Keep run names exact.** Open threads are keyed by run name.

## 5. Dry-run, then apply with the seed

```sh
hev orchestrate apply -f carry-over.md --seed-checkpoint carry-over.checkpoint.md --dry-run
hev orchestrate apply -f carry-over.md --seed-checkpoint carry-over.checkpoint.md
```

The dry run validates the spec and the seed's sections and writes nothing.
Apply records the seed as round `000` (`rounds/000/checkpoint.md` with
`provenance.json`: source path, sha256, time), and round `001` reads it as
its predecessor. The brief should tell round `001` to verify every item
against live evidence before acting. Applying again with a seed has three
outcomes:

- The same content as the recorded round `000` seed: a no-op, even after
  round `001` has run. Kit compares the sha256 in `provenance.json` first.
- A different seed when round `000` is already recorded: refused (`round 000
  already seeded with different content`).
- A first seed for an orchestrator that has already run round `001`:
  refused (`seed checkpoint must precede round 001`).

For held runs that the standing follow-through selects, seed it the same way,
before its first round. A follow-through that has already run rounds cannot be
seeded; send the converted notes to it instead with
`hev orchestrate say NAME "$(cat notes.md)"`.

## 6. Verify

- After round `001`, `hev orchestrate ls --json` reports `managed_runs` for
  the carry-over equal to the number of runs in `names.txt`, and the
  follow-through's count includes the runs in `selected.txt`. The run-by-run
  evidence is in `~/dispatch/orch-NAME/rounds/001/packet.json`.
- `rounds/001/checkpoint.md` refers to each held run by name, and the round
  verified rather than repeated the seed.
- No run received a duplicate rework from the paused loop.

Then retire the old loop once its rollback window has passed.

## 7. Roll back

1. `hev orchestrate stop carryover-20261106` (and stop or pause the
   follow-through if you seeded it). Stopping changes no run: held runs stay
   as they are in dispatch.
2. Resume the retiring loop from its own preserved state.
3. Reworks the carry-over already sent are part of each run's history
   (`hev dispatch show NAME`); tell the resumed loop about them in its inbox.

To try again with a different seed, use a new spec name (for example a new
date suffix). Reapplying a stopped spec under the same name preserves its
rounds, so it cannot be reseeded.

If work remains near expiry, hand it to a person with
`hev orchestrate handoff NAME --to SESSION` rather than extending the spec.
