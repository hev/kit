---
title: Archive-wide prompt coverage can hide a fully backfilled mini
date: 2026-09-08
area: deploy
kind: environment
tags: [read-side, backfill, dashboard, coverage]
source: https://linear.app/hevmind/issue/FAC-11
---

## What happened

FAC-11 live acceptance showed incomplete archive-wide prompt coverage while
the worker was already running on the mini. Host-filtered stats revealed that
every missing prompt timestamp belonged to laptop rows. Mini coverage was
already complete, although its newly introduced preview fields still needed
the replay.

## What didn't work

- Treating the archive-wide deficit as proof that the mini lacked prompt times.
- Asking for SSH to the machine on which the worker was already running.
- Estimating replay duration from transcript bytes alone: thousands of small
  transcripts still make sequential hosted block/session writes expensive.

## What works

Check the current hostname and compare `/api/stats?window=30d&host=...` for
the host values actually present in the archive. Check preview coverage
separately from prompt-time coverage. A mini replay cannot read laptop-local
transcripts; record the remaining laptop command explicitly.

For a slow authorized read-side replay, `--workers 4` or `--workers 8` bounds
concurrent transcript writes while retaining the default sequential mode.
Keep the process supervised, measure throughput and machine health, and read
the final error and row-count report; progress alone is not success evidence.

## How to avoid it

Record per-host coverage before starting a backfill, then repeat the same
queries after it finishes. Distinguish the running dashboard binary from a
temporary branch binary, and never infer that a successful local replay also
ran on the other host.
