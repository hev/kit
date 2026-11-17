# Steering

`hev steer` changes what a busy agent is doing, with a receipt that says
whether it saw the message ([RFC 0013](rfcs/0013-hooks-and-steering.md)). It is
one front door for dispatch runs, orchestrators (`o-NAME`) and local peers
sessions.

```sh
hev steer fix-ci 'Also cover the empty case'                      # note (next turn)
hev steer fix-ci 'Scope changed: candidate B, not A' --level urgent
hev steer fix-ci 'CI was cancelled upstream' --level halt
hev steer fix-ci 'Switch to candidate C' --level redirect
hev steer ls [TARGET]                                              # receipts on this host
hev steer show ID
hev steer ack ID...                                                # run by the agent
```

## Levels

| Level | What happens | Use it when |
| --- | --- | --- |
| `note` | Waits for the next turn: injected at the next prompt. Dispatch runs and orchestrators also get it in `inbox.md`, as `say` does. `dispatch say` to a running run, `peers send` to a dispatch run and `orchestrate say` are notes with receipts: they print or return the `st-...` ID and count in `ls`/`show`. `peers send` to an interactive session is that session's note path and keeps peers' own inbox receipts. `dispatch say` to an ended run reworks it, and the message is the rework's instruction. | It can wait. |
| `urgent` | Injected at the agent's next tool call, once. Until the agent runs `hev steer ack ID`, file writes and edits, `git commit/push/...`, `hev dispatch` (other than reads) and `gh pr` writes are blocked with a reason that names the ack. Reads keep working. | Facts changed and the agent must not build on the old ones (#292). |
| `halt` | Every tool call is blocked with "end your turn now", at the next tool call. The agent ends its turn; a dispatch run shows `halting`, then `halted`. | Work must stop now. |
| `redirect` | A halt, then the run's worktree is committed as a checkpoint and the same run is reworked with the message appended. Branch and PR are kept. Dispatch runs only. | Stop and continue differently, without losing work. |

A halt or redirect sent while a run or round is still starting is delivered
at its first prompt: the harness refuses the prompt, so the agent does no
work, and the run ends `halted` (a redirect then checkpoints and continues).

Nothing is killed to deliver. A halt that has not ended its turn after
`--halt-timeout` (default `[hooks] halt_timeout`, `10m`) becomes a hard stop of
the dispatch run or orchestrator round; the receipt records `escalated` and
`ls` shows `halted`. A peers session has no supervisor and is never
hard-stopped.

The urgent hold classifies shell commands by splitting them into words the way
the shell does, so `git -C "a dir" commit` and `git --git-dir="a dir/.git" push`
are held, as are commands inside `$(...)`, backquotes and `sh -c '...'`. Inline
interpreter code (`python3 -c`, `node -e`) is held when it visibly writes a file
(`open(..., 'w')`, `write_text`, `writeFileSync`, `os.remove` and the like). A
script file an interpreter runs is not inspected: the hold is a steering aid,
not a sandbox.

## Delivery

`hev steer` prints the delivery a target gets:

- **tool-boundary**: kit's [hooks](hooks.md) run in the target. Every dispatch
  run and orchestrator round launched by this kit has them (Claude Code and
  Codex). A peers session has them once `hev hooks install` ran and a hook has
  fired since the session started.
- **turn-boundary** (fallback): a run launched by an older kit, or a session
  without hooks. Urgent goes to
  `inbox.md` (dispatch) or `peers send` (peers session), with nothing blocked.
  Halt sends the harness's own interrupt: SIGINT to a headless dispatch harness
  or orchestrator round (the receipt is marked injected by `interrupt`, so the
  run still ends `halted` and a redirect still checkpoints and continues),
  Escape to a peers pane.

An urgent is injected as context after the call that was running, or, if the
next call is a mutation, as the reason that call is blocked. Measured on Claude
Code 2.1 and Codex 0.162, delivery took 1 to 10 seconds with a 20-second tool
call running: never more than one tool call.

## Receipts

Each message gets an ID (`st-...`) and a receipt:

```
queued -> injected (time, event, turn, tool-call index) -> acked (time, by)
```

- Only the hook injects, and only `hev steer ack` (or a turn ending, for halt
  and redirect) acknowledges. Reading `inbox.md` by hand is not an
  acknowledgement, and acking a message that was never injected is refused.
- `Blocked` counts the tool calls a message held.
- Receipts live with the target: `~/dispatch/NAME/steer/` (run),
  `~/dispatch/orch-NAME/steer/` (orchestrator), `~/.hev/steer/peers/SESSION/`
  (peers session). `state.json` beside them counts turns and tool calls.

Where to see them:

- `hev dispatch ls`: a `STEER` column, `q1 i0 a2` (queued, injected, acked),
  plus `halting` or `halted`. Halting and halted runs are listed by default.
- `hev dispatch show NAME`: `steer: 1 queued, 0 injected, 2 acked` and a receipt
  table with injection turn, tool call and latency.
- `hev orchestrate show NAME`: the same for an orchestrator.
- `hev peers`: `steer: ...` in a session's flags while anything is unacknowledged.

## Targets

- A dispatch run on this host is steered in place; on another host, through the
  dispatch RPC (`--host` when a name is ambiguous). Teammates may steer their
  own running runs with note, urgent and halt; redirect reworks, which only the
  owner can do.
- An orchestrator, `o-NAME`: note is `orchestrate say`; urgent reaches a running
  round at its next tool call (or the next round); halt ends the running round.
  Redirect is refused. Orchestrator authority maps steer to `say` (note,
  urgent), `stop` (halt) and `rework` (redirect).
- A local peers session (a tmux session on this host): note is `peers send`;
  urgent and halt use the session's hooks. A remote `HOST/SESSION` takes notes
  only.

## For agents

When kit injects a steering message, read it, take it into account, and run the
ack command it names (`hev steer ack ID`, with this host's hev path). Do not ack
a message you have not acted on; the ack is what lets mutating calls through.
On a halt, end your turn with one line saying where you stopped.
