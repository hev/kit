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
The selected host runs the common orchestrator runner in tmux session
`o-ORDER-NAME` (Windows uses a detached process), surviving your laptop sleeping.
It uses the fixed snapshot run names, `say,rework,notify,validate` authority and a once preset; a compatibility
scheduler retains `--every` and `--max-lifetime` from the saved snapshot.
`--repo` is retained in that snapshot; the orchestrator itself needs no worktree.
All inventory hosts must be reachable when collecting the snapshot. Before starting
its supervisor, the preset applies admission-only copies of the spec on every
followed host and the configured validator host, with the durable owner route.
These copies cannot launch supervisors. A refused/unreachable host (including an
older host without admission-only RPC support) aborts startup and is named in the
error; previously registered copies are rolled back. Stop removes these copies. The saved
snapshot identifies followed work. One active expediter per reply session is allowed.

The embedded, version-stamped `hev-expedite` skill and order template prescribe
reading the inbox and chats before work, reviewing completed work at high effort,
and an independent verification pass using `hev dispatch validate NAME --host HOST`.
Inside orchestrator rounds HOST identifies the followed run host; the validator
host/harness/model comes from `[validation]`. It must differ from the implementer.
Explicit non-browser validation uses a read-only acceptance reviewer; automatic
browser validation retains its browser-only trigger. Read validation.json and
verdict.json (the verdict also lives in the validation record) next round and
require the current revision. Never substitute a worker success report. In the orchestrator preset, findings go through `hev dispatch rework`,
preserving the existing run, branch and PR. The explicit legacy evidence helper
still supports `amend` outside an orchestrator. Missing review tools or an unavailable verification model must be
reported as `[needs you]`; they do not count as a successful verification.

The default `review` authority permits review and rework, and never merges.
`merge-on-green` is retained for the explicit legacy evidence helper and permits
merging reviewed and independently verified PR heads with green CI there. New
orchestrator presets report merge-ready results to the owner and have no merge
authority. No authority permits deployment, tags, releases or live
configuration changes. The skill records review evidence by head SHA and invokes:

```sh
hev expedite round ../order.md --evidence evidence.json
```

Evidence is an object keyed by `HOST/NAME`, e.g.
`{"worker/task":{"head":"SHA","reviewed":true,"verified":true}}`.
An optional `amend` string contains the complete amended order, tied to the
reviewed SHA in `head` (the run’s `head_sha` when no PR exists). The helper only
sends it when that SHA matches the current head and the run is not running.
Drop `amend` from subsequent evidence once successfully sent; review the new
head after rework finishes. Stale amendments report that review is awaited.
The helper reads
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
Unchanged evidence and an empty inbox skip the model at no model cost (`idle:
skip`). Polls still refresh run state and PR head/checks. Three consecutive
unchanged polls with an accepted checkpoint are a stall: send one lifecycle
notice with its first `[needs you]` line, show `stalled` and that line in status,
and keep polling without launching a harness. A changed run, PR head/check,
validation result or inbox message clears the stall and runs a fresh model round.
The previous accepted checkpoint (including named waits) stays unchanged during
skipped polls. Status shows state, round count, last/next round, followed snapshot,
and undelivered notice count with newest message and error.
Use `--host HOST` and an explicit NAME to inspect or stop from another session.
Ordinary `hev dispatch show o-NAME` retains detailed round notes and reports.

If the owning host has `[board].url` in its HEV config or `FACTORY_BOARD_URL`,
or the retained board endpoint configuration used by the board CLI,
the supervisor posts a handoff on stop/completion/expiry containing the snapshot
and latest round notes. Failed handoffs leave `handoff.md` and `handoff-error`
in the run directory and a notification error. No endpoint is guessed.

Native `hev peers chat new MEMBER... --title TITLE -m MESSAGE`, `chat ls`,
`chat show ID`, and `chat say ID MESSAGE` support terminal and dispatched peers.
Chat IDs carry the title and membership so remote recipients can reply through
normal peer inbox transport. Keep chat IDs intact; inbox provenance identifies
the sender. Configured peers backends retain their own chat commands.

Owner replies require `notify` authority and are sent only after checkpoint
acceptance. Lifecycle notices are always durable. Native notices use the same
incarnation-bound peers beacon outbox as interactive messages when a cached host
exists, including historical laptop host aliases. Relay enqueue is still queued;
a recipient inbox receipt makes it delivered. When no cached host exists, an unresolvable SSH destination falls back to pull.
Stale snapshots and absent recipients remain queued and recheck the local relay
on each flush; transient SSH failures retry with backoff. From the
owner laptop, deliver and acknowledge queued notices with:

```sh
hev dispatch notices pull --host oven --deliver
hev dispatch notices pull --host mini --deliver
```

This drains both member-run and orchestrator notices; `hev orchestrate notices
pull --host HOST --deliver` remains available for orchestrator notices alone.
Only successful inbox delivery is acknowledged; failures remain visible for retry.

Configure the round harness/model in the dispatch config:

```toml
[expedite]
harness = "claude"
model = "claude-fable-5-2"
```

`hev expedite --harness HARNESS --model MODEL` overrides these values. Each
field resolves independently: flag, then config, then the defaults `claude` /
`claude-fable-5-2`. Both are saved in the spec and shown by `hev expedite status`.
The independent validator is separately configured under `[validation]`; an
identical implementer/validator model is rejected and reported `[needs you]`,
including when their harness names differ.
