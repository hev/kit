# Active RFCs

Index of active proposals, grouped by priority horizon. Update this
table when adding or archiving an RFC. Conventions follow the layer
repo (`../layer/docs/rfcs`): numbered files, Status header, Summary /
Motivation / Decision / Goals / Non-goals / Vocabulary / Proposed
surface / Open questions.

## Now

| RFC | Title | State | Why now |
|-----|-------|-------|---------|
| [0024](0024-workspaces.md) | Workspaces | Proposed | Scope host routing, harness subscriptions and storage to a workspace. |
| [0024](0024-licensing.md) | Offline Kit Pro licenses | Implemented | Distinct Ed25519 verification and local entitlement for Kit Pro. |
| [0023](0023-climb.md) | `hev climb` — multi-day hill climbing on a scored goal | Proposed | A meta-harness can spend agent-hours for days on its own hosts and plans; the missing piece is the loop: a pinned numeric objective (an RFC 0021 `metric` scorer), a population of branches, selection, paced budgets, and control rules so the fleet cannot dispatch, merge or grade itself. |
| [0022](0022-evidence.md) | Evidence — multimodal proof packages, folders in OSS, EmbeddingGemma 2 retrieval and federated search in Pro | Draft | Validators, browser-use, computer-use and spikes need a first-class multimodal proof primitive (text, image, video): a plain folder of files in OSS, `/v1/evidence/*` + federated `/v1/search` + Pro UI viewer in Pro. |
| [0022](0022-pro-workspaces.md) | Pro dispatch workspaces | Implemented | Bound build caches and reclaim clean, recoverable worktrees. |
| [0021](0021-cron.md) | Cron in hevd | Phase 1 implemented | Cross-platform schedules share the daemon secrets cache, attention pipeline and optional SWIM placement. |
| [0020](0020-harness-registry.md) | Harness registry | Implemented | Gives new harnesses one place to declare identity, discovery and subsystem capabilities. |
| [0019](0019-gates.md) | Gates — checks that fail closed, set on the host and answered by a person, a command or a validator | Proposed | Kit has six one-off gates; validation covers only the browser; attention goes to a host, not a person; host TOML is writable by the agents it governs. One fail-closed primitive in hevd-owned host config unifies them for Club Kit. |
| [0018](0018-skills-shelf.md) | The skills shelf — a few pinned popular packs, enabled per run, searchable in Layer | Implemented | Board memo #500: welcome popular and personal skills through a curated shelf, not a marketplace; per-run enablement and commit-joined skill loads are what let kit measure which packs work. |
| [0016](0016-cluster.md) | Encrypted hevd membership and replicated usage facts | Implemented | Shared liveness and bounded counters before account-wide secrets budgets. |
| [0014](0014-release-train.md) | Release train — a train branch, docs first, a draft release as the ledger, publishing as the gate | Proposed; runbook and workflows in PR | 0.7 is the first release on a train branch (`v0.7`), with docs reviewable before it ships, one act (publish) to cut it, and `main` as the latest release. |
| [0013](0013-hooks-and-steering.md) | Hooks and steering — one hook layer across harnesses, and steering that reaches a busy agent | Implemented (Claude Code, Codex) | Board #292: a scope correction sat unseen for 4.5 hours in a single long Codex turn. Urgent steering lands at the next tool call, holds mutations until acknowledged, and receipts replace "queued N". |
| [0013](0013-swarms.md) | Swarms: one file that puts an orchestrator and its workers on a problem across hosts | Phase 1 implemented (single host) | Swarming agents on one problem today means a hand-written orchestrator spec plus N orders, with no healing when a worker or host is lost. One file, a code reconcile tick and fencing make it declarative and self-healing over orchestrate and dispatch. |
| [0011](0011-nodes.md) | SSH host invitations and bound dispatch senders | Proposed; implementation in PR | Share a host with named dispatch access through existing SSH. |
| [0008](0008-orchestrators.md) | Orchestrators — optional, declarative agents that keep a goal moving | Phase 2 implemented | Follow-through after dispatch is the bottleneck; bespoke expediter and intake loops are invisible and unbounded. Rounds with required checkpoints make long-lived orchestrators safe. |
| [0007](0007-collections.md) | Collections — one `hev traces search` over traces and what was kept from them | Draft; `pkg/search` landed | The factory board is a second Layer namespace searched the same way; one front door (`--in traces,board`) is what agents can be told once. |
| [0006](0006-up-command.md) | `hev up` — one command from install to backfilling traces | Implemented, amended 2026-11-10 | Closes the gap between `brew install` and a searchable archive. A hev layer key selects hosted capture and dashboard; keyless setup captures into a file archive and starts nothing in Docker. Provider accounts are an advanced explicit store choice. |
| [0005](0005-marks.md) | Trace evaluations in Layer | Implemented | Generic eval ingestion, latest-grade joins, evaluator search, and store-filtered dashboard. |
| [0004](0004-read-side.md) | The read side — traces, timeline and stats on real data | Draft | The harness deletes transcripts after 30 days and nothing reads the archive; mocked against 1,643 real sessions, reviewed, and headed for the mini plus a Docker home lab. Adds `<ns>-blocks` and `<ns>-sessions` beside RFC 0003's chunks. |
| [0003](0003-trace-model.md) | The trace model — transcript-native capture and the namespace schema | Draft | Answers 0002's schema/chunking/embedding open questions; reads the transcripts the daemon currently ignores (382 sessions unread on the factory host); closes #8 and #1. |
| [0002](0002-direct-to-layer.md) | Direct-to-Layer capture — retire the warehouse-first architecture | Draft | Collapses the unshipped warehouse→iceberg→Layer path to one hop that works today; `hev traces search` on day one; every kit install becomes a Layer install (the OSS wedge). Durability moves to a future Layer WAL. |
| [0001](0001-frame-capture.md) | Frame capture — session-gated screenshots with on-device embeddings | Draft | The level-up for `hev d`: turns the trace archive into (observation, action, outcome) episodes; the session-gated/on-device privacy stance is the differentiator vs the 24/7-DVR screen-memory tools. |

- [0012: Receiver FIFO dispatch queue](0012-dispatch-queue.md) — proposed
