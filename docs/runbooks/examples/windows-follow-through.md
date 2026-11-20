---
kind: orchestrator
name: winhost-follow
goal: Every run on this Windows host for an ACME issue is reviewed, reworked until clean, and reported to the owner as ready to merge with green CI at its current head.
host: winhost                # placement label; apply on this host
mode: loop
every: 15m
until: never
max_lifetime: 30d
harness: claude
reply: owner-session
notify_route: pull           # this host cannot reach the owner; the owner pulls
manages:
  runs: {selector: "issue=ACME-*"}
authority: [say, rework, notify]
budget:
  concurrent_runs: 1
  runs_per_day: 6
idle: skip
round_timeout: 45m
---
Follow every run in scope until a human can merge it. Read the inbox first.
Review each new PR head with this harness, check CI at that head yourself,
send findings with `hev dispatch rework`, and record named waits with their
release conditions. Report ready-to-merge results in `reply.md` with the head
SHA and evidence. Never merge, and never touch the issue tracker.
