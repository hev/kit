# GitHub Copilot CLI

Use `harness: copilot` in a dispatch order, orchestrator or swarm. The worker
runs `copilot` in prompt mode with tool approvals enabled and passes the order's
model and reasoning effort when specified. Dispatch links the result to the session when Copilot
creates its local event log. Copilot can use saved credentials or a token from
`[harnesses.copilot.env_refs]` in dispatch configuration. GitHub documents
`COPILOT_GITHUB_TOKEN`, `GH_TOKEN` and `GITHUB_TOKEN` in precedence order.

Kit reads user and assistant text from
`~/.copilot/session-state/<id>/events.jsonl`. Set `COPILOT_HOME` to relocate
Copilot's session directory. Peers can identify native CLI and npm launcher
processes for inventory.

Copilot reads repository hooks from `.github/hooks/*.json`. Kit's hook installer
writes the user-level `~/.copilot/hooks/hev.json` (under `COPILOT_HOME` when
set). Copilot's normal trust settings govern repository hooks. Command hooks
can deny a tool before execution and supply context at session start or after
a successful tool call.

Dispatch places project skills in `.github/skills/`. Kit discovers personal
skills in `~/.copilot/skills/` (or under `COPILOT_HOME`) and installs bundled
skills in the personal Copilot directory when using its default location.
