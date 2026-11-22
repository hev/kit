# hev foh: the native front-of-house harness

`hev foh` is an optional agent harness built into `hev` for front-of-house
work: coordinating dispatches, peers, the board and traces. It runs a lean Go
agent loop in a terminal UI, through [OpenRouter](https://openrouter.ai), your
OpenAI/Codex ChatGPT subscription, or both.
Back-of-house coding still runs on `claude`, `codex` and `agy` through
`hev dispatch`. The design is in [RFC 0029](rfcs/0029-native-foh-harness.md).

```sh
export OPENROUTER_API_KEY=sk-or-...   # and/or: codex login (ChatGPT plan)
hev foh                          # interactive TUI
hev foh --model openai/gpt-5     # start on a specific model
hev foh --model codex/gpt-5.4    # start on your Codex subscription
hev foh --once "what is running on oven?"   # one turn, then exit
hev foh --show-mascot            # print the mascot and exit
```

## YOLO by default

Tools run without asking first. Put isolation in the host, workspace,
[admission](dispatch.md#repository-admission) and gates, not in approval
prompts. Every tool call is still bounded:

- `bash` times out after 2 minutes by default. The model can ask for up to
  10 minutes.
- `hev_*` calls time out after 60 seconds.
- A tool result sent back to the model is capped at 32 KiB.
- `glob` and `grep` stop at fixed limits on results and files walked.

Tool subprocesses don't receive `OPENROUTER_API_KEY` or `HEV_FOH_API_KEY`.

## Providers

| Provider | Badge | Model IDs | Sign-in |
|----------|-------|-----------|---------|
| `openrouter` | `[OR] $3.00/$15.00` | vendor slugs, such as `google/gemini-4-neon` | `OPENROUTER_API_KEY`, `HEV_FOH_API_KEY` or `[foh].api_key_ref` |
| `codex-sub` | `[codex-sub] sub` | `codex/NAME`, such as `codex/gpt-6-astra` | your Codex ChatGPT login (`codex login`) |

You need at least one. With both, every model from both shows in the same
picker.

### Codex subscription (`codex-sub`)

`hev foh` reuses the ChatGPT login the Codex CLI saved. It reads `auth.json`
from the first of:

1. `[foh.codex].home`;
2. `CODEX_HOME`;
3. the active workspace's codex profile (`HEV_WORKSPACE`, see
   [workspaces](workspaces.md));
4. `~/.codex`.

The `codex/...` models show up only when that file has a usable ChatGPT
login. An `auth.json` that holds only an API key doesn't count. An expired
access token is refreshed once with the stored refresh token, and the new
tokens are written back to `auth.json` so the Codex CLI keeps working. If the
login can't be used, the turn ends with an error that tells you to run
`codex login`, and `hev foh` keeps running.

The default models are `gpt-6-astra`, `gpt-5.4` and `gpt-5.3-codex`.
`[foh.codex].models` replaces the list.

### Claude subscriptions are never used

Subscription providers are added only where the provider's terms allow it.
Today the only one is OpenAI/Codex ChatGPT. `hev foh` never reads or uses
Anthropic/Claude OAuth subscription credentials, and it refuses:

- a Codex home inside `.claude` or `CLAUDE_CONFIG_DIR`;
- an auth file that holds a Claude login;
- `codex/claude-*` model names.

Claude models in `hev foh` go through OpenRouter (or another
OpenAI-compatible gateway) with an API key. Use your Claude subscription in
Claude Code.

## Flags

| Flag | Meaning |
|------|---------|
| `--model ID` | Start on this model: an OpenRouter slug or `codex/NAME`. |
| `--prompt PATH` | Append this file to the system prompt. |
| `--once MESSAGE` | Run one turn without the TUI. Text goes to stdout and tool activity to stderr. |
| `--show-mascot` | Print the mascot and exit 0. |
| `--no-color` | Use the ASCII rendering and plain styles. |

When `--model` isn't given, `hev foh` uses `HEV_FOH_MODEL`, then
`[foh].model`, then your first favorite whose provider is set up, then
`anthropic/claude-sonnet-5.5`. With no OpenRouter key, it starts on the first
`codex/` model instead.

## Configuration

All settings are optional. They go in `~/.hev/config.toml`, or in the file
named by `HEV_CONFIG`:

```toml
[foh]
model = "anthropic/claude-sonnet-5.5"
base_url = "https://openrouter.ai/api/v1"          # or HEV_FOH_BASE_URL
api_key_ref = "op://Private/OpenRouter/credential"  # or secret://openrouter
system_prompt = "~/.config/hev/foh-team.md"         # inline text or a path

[foh.codex]
models = ["gpt-6-astra", "gpt-5.4", "gpt-5.3-codex"]  # shown as codex/NAME
# enabled = true    # list codex/ models even without a login; false hides them
# home = "~/.codex" # Codex home holding auth.json (default: CODEX_HOME, workspace profile, ~/.codex)
# base_url = "https://chatgpt.com/backend-api/codex"  # or HEV_FOH_CODEX_BASE_URL
```

The key is read from `OPENROUTER_API_KEY` first, then `HEV_FOH_API_KEY`, then
`api_key_ref`. `api_key_ref` is resolved through
[hev secrets](secrets.md). The key never appears in error messages or session
files, and neither do Codex access, refresh or id tokens or the
`ChatGPT-Account-Id`.

## Model picker

To open the picker, press `Ctrl+M` or type `/model`. Most terminals send the
same byte for `Ctrl+M` and Enter, so Enter on an empty prompt also opens it.

- Type to fuzzy-search IDs, names and providers across every provider.
- Use the arrow keys or `Ctrl+N`/`Ctrl+P` to move.
- Press Enter to switch.
- Press `Ctrl+F` to star or unstar a model. You can also press Tab to focus
  the list and then `f`.
- Esc closes the picker.

Each row shows the ID, the context window (such as `200k`) and a
provider/cost badge: `[codex-sub] sub` for a subscription model, or
`[OR] $3.00/$15.00` for the OpenRouter prompt/completion price per million
tokens. Subscription models list first. Starred models from either provider
sort above the rest and are saved to `~/.hev/foh-favorites.json`.

A switch takes effect on the next request and keeps the conversation, even
across providers. For example, turn 1 can run on `codex/gpt-6-astra` and
turn 2 on `google/gemini-4-neon`, with the same history and tool results. The
transcript shows a badge such as `⇄ model a → b`, and it adds
`(codex-sub → openrouter)` when the provider changes. `/model ID` switches
without opening the picker.

The catalog is cached at `~/.hev/cache/openrouter-models.json` for 24 hours.
When OpenRouter can't be reached, the picker uses the stale cache, or a small
built-in list if there is no cache.

## System prompt

The prompt is assembled from these parts, in order:

1. kit's built-in FOH instructions;
2. `AGENTS.md` and `MISSION.md` in the current directory;
3. `~/.config/hev/foh-prompt.md`;
4. `[foh].system_prompt`;
5. the `--prompt` file;
6. a live header: host, cwd, workspace, model and kit version.

`/prompt` shows which parts were used and the full text. `/reload` rebuilds
the prompt after you edit any of those files, without leaving the thread.

## Tools

| Tool | Actions |
|------|---------|
| `hev_dispatch` | `ls`, `show NAME`, `hosts`, `send` (a full order, or `name`, `repo` and `body` plus optional `host`, `base`, `harness`, `model`) |
| `hev_peers` | `ls`, `read TARGET`, `send TARGET MESSAGE` |
| `hev_board` | `search QUERY`, `read ID`, `post` (`board`, `title`, `body`) |
| `hev_traces` | `search QUERY`, `stats` |
| `hev_status` | workspace and cluster overview (`hev --json`) |
| `read_file`, `glob`, `grep`, `bash`, `edit_file` | local work in the current directory |

The `hev_*` tools appear in the transcript as bordered cards with the tool,
action, a one-line summary and up to six lines of the result. `Ctrl+O`
expands every card. Generic tools appear as one dim line with a short preview
of the output.

## Steering

While a turn is running, anything you type and send is a steer. It arrives
after the current tool call finishes. Tool calls the model had queued behind
it are skipped, and the model sees your message before it continues.
`Ctrl+C` cancels the turn, and `Ctrl+C` again when idle quits.

## Slash commands

`/model [SLUG]`, `/prompt`, `/reload`, `/clear` (start a new thread), `/help`,
`/quit`.

## Session traces

Each session writes redacted JSONL to `~/.hev/foh/sessions/<ulid>.jsonl`.
`HEV_FOH_SESSIONS_DIR` changes the directory. A record holds the turn number,
its type (`user`, `assistant`, `tool`, `steer`, `model_switch`, `error`), the
`provider` and exact `model` that served it, any tool calls and the token
usage. A `model_switch` also records `from` and `from_provider`. API keys,
OAuth tokens, bearer headers and the ChatGPT account ID are removed before a
record is written. kit keeps the newest 500 sessions and
caps each file at 64 MiB. Phase 1 doesn't add these sessions to the
[trace archive](trace-archive.md).

## Mascot

The splash header shows the mascot next to the version, working directory,
workspace, active model and shortcuts. The color version uses half-blocks
with 24-bit color, and transparent pixels never paint a background. Terminals
narrower than 90 columns get a 20×10 version. With `NO_COLOR`, `TERM=dumb` or
`--no-color`, an ASCII version with no escape sequences is shown instead.
