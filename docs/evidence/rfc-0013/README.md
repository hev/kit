# RFC 0013 acceptance evidence

Captured on oven, 2026-11-17, in a temporary HOME (`/tmp/kit-e2e-steer`) with a
private tmux server, so the host's `~/.claude` and `~/.codex` hook config was
never touched. Workers were ordinary `hev dispatch` runs of a harmless order:
poll `./fake-ci` (`sleep 20; echo pending`) and rewrite `progress.md`, 30 times,
in one turn, with no commits or pushes.

- `claude-transcript-timeline.txt`: the Claude Code run `e2e-steer-2` (three
  attempts) from its session transcripts: every tool call and result, and what
  kit's hooks injected or denied.
- `receipts.txt`: `hev steer ls` for every run and the final `hev dispatch ls --all`.
- `failopen.txt`: a `claude -p` session with a hanging and a crashing
  `hev-hook-*` handler on PATH; every tool call ran and each event logged two
  warnings.
