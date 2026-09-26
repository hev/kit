# Active RFCs

Index of active proposals, grouped by priority horizon. Update this
table when adding or archiving an RFC. Conventions follow the layer
repo (`../layer/docs/rfcs`): numbered files, Status header, Summary /
Motivation / Decision / Goals / Non-goals / Vocabulary / Proposed
surface / Open questions.

## Now

| RFC | Title | State | Why now |
|-----|-------|-------|---------|
| [0007](0007-collections.md) | Collections — one `hev query` over traces and what was kept from them | Draft; `pkg/search` landed | The factory board is a second Layer namespace searched the same way; one front door (`--in traces,board`) is what agents can be told once. |
| [0006](0006-up-command.md) | `hev up` — one command from install to backfilling traces | Implemented, amended 2026-09-23 | Closes the gap between `brew install` and a searchable archive. Amended: a Turbopuffer key is required, the gateway and dashboard run in Docker, the daemon stays on the host under launchd. |
| [0005](0005-marks.md) | Trace evaluations in Layer | Implemented | Generic eval ingestion, latest-grade joins, evaluator search, and store-filtered dashboard. |
| [0004](0004-read-side.md) | The read side — traces, timeline and stats on real data | Draft | The harness deletes transcripts after 30 days and nothing reads the archive; mocked against 1,643 real sessions, reviewed, and headed for the mini plus a Docker home lab. Adds `<ns>-blocks` and `<ns>-sessions` beside RFC 0003's chunks. |
| [0003](0003-trace-model.md) | The trace model — transcript-native capture and the namespace schema | Draft | Answers 0002's schema/chunking/embedding open questions; reads the transcripts the daemon currently ignores (382 sessions unread on the factory host); closes #8 and #1. |
| [0002](0002-direct-to-layer.md) | Direct-to-Layer capture — retire the warehouse-first architecture | Draft | Collapses the unshipped warehouse→iceberg→Layer path to one hop that works today; `hev find` on day one; every kit install becomes a Layer install (the OSS wedge). Durability moves to a future Layer WAL. |
| [0001](0001-frame-capture.md) | Frame capture — session-gated screenshots with on-device embeddings | Draft | The level-up for `hev d`: turns the trace archive into (observation, action, outcome) episodes; the session-gated/on-device privacy stance is the differentiator vs the 24/7-DVR screen-memory tools. |
