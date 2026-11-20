# Cursor CLI

Set `harness: cursor` in a dispatch order, swarm, or orchestrator spec.
Kit launches `cursor-agent` in print mode with structured output and
`--force`. Set `model` to select a Cursor model. Cursor uses its own default
effort. Authenticate with Cursor login or set `CURSOR_API_KEY`, optionally
through `[harnesses.cursor.env_refs]` in dispatch.toml.

Kit indexes Cursor JSONL agent transcripts under
`~/.cursor/projects/*/agent-transcripts/`. Cursor's separate CLI chat database
is outside this integration.

Install user hooks with `hev hooks install --harness cursor`. Cursor also
reads project hooks from `.cursor/hooks.json` using the same version 1 format.

Skill discovery reads `~/.cursor/skills/` and `.cursor/skills/`. Peers
identifies Cursor processes for observation.
