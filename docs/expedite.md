# Optional follow-through with `hev expedite`

Expediting is an opt-in practice. Ad hoc follow-through remains useful; dispatch
never requires an expediter. From your front-of-house terminal session:

```sh
hev expedite --host worker --authority review --every 10m --max-lifetime 24h
hev expedite status
hev expedite stop
```

The command resolves your current peer address and snapshots dispatch runs whose
`reply` exactly matches it. Set `--host`, or configure dispatch's `default_host`.
The selected host must support headless dispatch. The expediter runs there as
`HOST/ORDER-NAME`, surviving your laptop sleeping. It uses a worktree in the
first followed run's repo; `--repo` selects another repository on that host.
All inventory hosts must be reachable when collecting the snapshot. New runs are
not automatically enrolled. One active expediter per reply session is allowed.

The embedded, version-stamped `hev-expedite` skill and order template prescribe
reading the inbox and chats before work, reviewing completed work at high effort,
and an independent verification pass (a different implementer model when
configurable). Findings go to `hev dispatch amend`, preserving the existing run,
branch and PR. Missing review tools or an unavailable verification model must be
reported as `[needs you]`; they do not count as a successful verification.

The default `review` authority permits review and rework, and never merges.
`merge-on-green` additionally permits merging reviewed and independently verified
PR heads with green CI. No authority permits deployment, tags, releases or live
configuration changes. The skill records review evidence by head SHA and invokes:

```sh
hev expedite round ../order.md --evidence evidence.json
```

Evidence is an object keyed by `HOST/NAME`, e.g.
`{"worker/task":{"head":"SHA","reviewed":true,"verified":true}}`.
An optional `amend` string contains the complete amended order. The helper reads
the inbox first, re-reads run state and GitHub checks, sends one line per run to
the reply session, and gates merge on the recorded head. GitHub's match-head
condition rejects a changed head at merge time. Every reported check must succeed;
empty, pending, skipped, neutral and failed check sets block readiness. Missing
`gh` or access errors are reported, never treated as green. Review evidence is
an agent attestation, not a sandbox boundary against a malicious agent.

The repeating supervisor stops on all runs merged or explicitly abandoned,
`hev expedite stop [NAME]`, or maximum lifetime. A closed unmerged PR requires a
person's explicit abandonment instruction in the inbox; the expediter must
record that disposition in its round notes. **On reopening your session, run
`hev expedite stop` explicitly**; there is no automatic laptop wake detector.
Status shows the owner, state, round count, last/next round and followed snapshot.
Use `--host HOST` and an explicit NAME to inspect or stop from another session.
Ordinary `hev dispatch show NAME` retains detailed round notes and reports.

If the owning host has `[board].url` in its HEV config or `FACTORY_BOARD_URL`,
the supervisor posts a handoff on stop/completion/expiry containing the snapshot
and latest round notes. Failed handoffs leave `handoff.md` and `handoff-error`
in the run directory and a notification error. No endpoint is guessed.

Native `hev peers chat new MEMBER... --title TITLE -m MESSAGE`, `chat ls`,
`chat show ID`, and `chat say ID MESSAGE` support terminal and dispatched peers.
Chat IDs carry the title and membership so remote recipients can reply through
normal peer inbox transport. Keep chat IDs intact; inbox provenance identifies
the sender. Configured peers backends retain their own chat commands.
