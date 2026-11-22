# RFC 0029 — Optional native FOH harness (`hev foh`)

Status: Phase 1 implemented

## Summary

Add `hev foh`, an optional native front-of-house (FOH) harness built into the
`hev` binary: a lean Go agent loop, routed through OpenRouter, with a
specialized Bubble Tea TUI. The TUI has a model picker, a system prompt
assembled from several sources, and first-class cards for `hev` tools.
Back-of-house (BOH) work stays on the vendor harnesses (`claude`, `codex`,
`agy`) that kit already dispatches.

Board memo #570 proposed it. #534 framed kit as a meta-harness, and #562
defined the workspace primitive.

## Motivation

FOH sessions do coordination: they read peers, search the board and traces,
check status, and write orders. Vendor harnesses are built for BOH coding and
are a poor fit for this work:

- Every `hev` call goes through a generic shell tool. Its output is untyped text
  that the harness can't render as a dispatch, peer or board result.
- Changing models means leaving the session. A FOH thread can't move between
  a cheap model for triage and a strong model for a hard order.
- Steering a busy vendor harness needs PTY tricks (RFC 0013). A native loop
  can check for a steer between tool calls.
- Vendor approval prompts get in the way of an operator who has already put
  isolation where it belongs.

## Decision

### Two tiers

| Tier | Runs on | Why |
|------|---------|-----|
| BOH | `claude`, `codex`, `agy` through `hev dispatch` | Vendor harnesses are the best coding agents, and kit captures, hooks and steers them. |
| FOH | Vendor harness **or** `hev foh` (optional) | Coordination benefits from typed kit tools, model choice per thread and native steering. |

`hev foh` is opt-in. No existing flow depends on it, and BOH dispatch does
not change.

### YOLO by default

Tools run without approval prompts. Isolation belongs to the host (which
machine runs it), the workspace (RFC 0024: subscriptions, home and network),
admission (which repos a sender may touch), and gates (RFC 0019: fail-closed
checks set on the host). An in-loop "are you sure?" can't replace any of
those and wears the operator down. The loop is still bounded: every tool call
has a timeout and an output cap, and the provider key is kept out of tool
subprocess environments.

### OpenRouter first

The provider is an OpenAI-compatible streaming client pointed at
`https://openrouter.ai/api/v1`. `[foh].base_url` or `HEV_FOH_BASE_URL`
overrides the endpoint, so any compatible gateway works. One key reaches
every vendor's models, and this is what makes switching models mid-thread
cheap. The key comes from `OPENROUTER_API_KEY`, `HEV_FOH_API_KEY`, or
`[foh].api_key_ref`, which is resolved through kit's secrets layer (RFC 0009).
Error text and session traces never contain it.

The catalog from `/api/v1/models` is cached at
`~/.hev/cache/openrouter-models.json` for 24 hours and capped at 2,000 models.
When the network is down, the picker uses the stale cache, or a built-in
starter list if there is no cache.

### Model picker and hot swap

`Ctrl+M` or `/model` opens a fuzzy picker over the catalog. Each row shows the
slug, context window and prompt/completion price per million tokens. `Ctrl+F`
(or `f` with the list focused) stars a model. Stars are saved to
`~/.hev/foh-favorites.json`, and starred models sort first. Choosing a model
swaps the active model for the next request without touching the thread, and
the transcript shows a badge. Each session record is stamped with the exact
model that served it.

### Composable system prompt

The prompt is assembled from these parts, in order:

1. the built-in FOH meta-harness instructions;
2. `AGENTS.md` and `MISSION.md` from the working directory;
3. the user prompt, `~/.config/hev/foh-prompt.md`;
4. `[foh].system_prompt`, given as inline text or a path to a file;
5. the file passed with `--prompt`;
6. a live header: host, cwd, workspace, model, kit version and time.

`/prompt` shows the parts and the composed text. `/reload` composes the prompt
again, mid-thread.

### First-class `hev` tools

`hev_dispatch` (`ls`, `show`, `hosts`, `send`), `hev_peers` (`ls`, `read`,
`send`), `hev_board` (`search`, `read`, `post`), `hev_traces` (`search`,
`stats`) and `hev_status` call the `hev` binary itself with `--json` where the
subcommand has it. They return a typed card: tool, action, a one-line summary,
and a body that is bounded and collapsible. The TUI draws cards as styled
boxes, separate from generic `bash` output. `hev_dispatch send` takes either a
complete order or the fields name/repo/body (plus optional
host/base/harness/model). It validates the name and refuses multi-line
front-matter values, so a field can't inject keys.

The local tools are `read_file`, `glob`, `grep`, `bash` and `edit_file`.

### Steering and traces

A steer typed while a turn is running goes into a bounded queue, and the loop
checks that queue after every tool call. When a steer is waiting, the
remaining calls in that batch get a "skipped" result, which keeps the thread
valid. The steer then joins the thread as a user message and the loop
continues. A steer that arrives after the final answer starts another step.

Sessions are written as redacted JSONL to
`~/.hev/foh/sessions/<ulid>.jsonl`. Each record carries the turn, its type,
the model, tool calls and token usage. The newest 500 sessions are kept, and
each file is capped at 64 MiB. The harness registry (RFC 0020) lists `hev`
(aliases `hev_foh`, `foh`) with that trace root, which
`HEV_FOH_SESSIONS_DIR` overrides. It has no dispatch, hook, peer or skills
capability.

### Mascot

The splash header shows the deal-with-it pixel mascot, a 40×40 sprite in 16
colors, next to live kit metadata. Each text row holds two pixels using
half-blocks with 24-bit color. A transparent pixel never paints a background,
so the mascot blends into any terminal theme. Below 90 columns the TUI uses a
20×10 version. With `NO_COLOR`, `TERM=dumb` or `--no-color`, it prints an ASCII
version with no escape sequences. `hev foh --show-mascot` prints it and exits.

## Non-goals (Phase 1)

- Replacing vendor harnesses for BOH work.
- An approval or sandbox layer inside the loop.
- Ingesting FOH sessions into the trace archive. Phase 2 adds a trace source
  for the registered root.
- Providers other than OpenAI-compatible endpoints.
- Compacting context. A thread keeps at most 400 messages and drops whole
  exchanges from the front.

## Compatibility

New command, new config table (`[foh]`) and new files under `~/.hev`. Nothing
existing changes behavior. Builds and tests run on macOS, Linux and Windows.
On Windows, `bash` runs through `bash` when it is on `PATH`, and through
`cmd /C` otherwise.

## Phases

1. **This RFC.** Loop, OpenRouter client and catalog, tools, prompt
   composition, TUI with picker and cards, session JSONL, registry entry.
2. Archive ingestion of FOH sessions, plus usage and cost rollups in
   `hev traces stats`.
3. Workspace-scoped defaults (model, favorites, prompt) from RFC 0024
   workspaces, and gate-aware tool annotations from RFC 0019.
