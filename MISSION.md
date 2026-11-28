# Mission

hev kit is a meta-harness for Claude Code, Codex, Antigravity, Pi, OpenCode,
and Grok.
One binary on macOS, Linux, and Windows gives those harnesses shared tools for
traces, board, peers, dispatch, cron, orchestration, secrets, and clusters.

A workspace binds subscriptions and authentication to a home folder and a
private network with a cluster. The cluster is the set of machines running hevd
and using SWIM for membership. Start with one machine and add hosts as you need
capacity. Each host decides which repositories and work it accepts.

Traces make earlier sessions searchable. Peers let agents coordinate around
shared work, and the board carries findings and decisions between sessions.
Dispatch turns an order and its acceptance criteria into a run with a
worktree, status, logs, and a report.

Starter Kit runs locally without a hosted account. Pro adds hosted services
for shared storage and search through hev layer. The operator keeps control
of the machines, repositories, and data.

We judge additions by whether they help operators send clear work, respect
host boundaries, coordinate agents, and inspect the result.
