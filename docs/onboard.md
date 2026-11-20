# First-run onboarding (`hev onboard`)

`hev onboard` sets kit up on a new machine from a terminal, with no hand-edited
TOML:

```sh
brew install hev/tap/hev
hev up                     # on a terminal, the first run onboards
```

The first `hev up` on a terminal runs onboarding when the machine is
unconfigured: no kit config (`~/.hev/config.toml`), no dispatch config
(`~/.config/hev/dispatch.toml`) and no onboarding record
(`~/.hev/onboard.toml`). Any one of them means someone set the machine up
already, and `hev up` just runs. Run `hev onboard` yourself at any time to go
through it again.

## Steps

1. **Harnesses.** kit lists the agent harnesses in its harness registry
   (Claude Code, Codex, Pi, OpenCode, agy) and marks those it finds on `PATH`
   or by their config directory (`~/.claude`, `~/.codex`, ...). Press Enter to
   keep the detected ones, or type names or numbers (`claude, codex`, `1 2`,
   `none`). An unknown name is refused and asked again.
2. **Workspace and data.** Confirm the workspace directory, where your
   repositories live (default `~/workspace`; `~/` paths stay relative to
   `HOME`). kit shows where each data surface will live under `~/.hev/`, and
   names any legacy directory it keeps. Answer `n` and nothing is written.
3. **Start kit.** Onboarding runs `hev up`: kit's config, the data directories,
   a dispatch config naming this machine as host `here` (when there is none),
   the bundled skills, hevd and the local dashboard (Traces, Stats, Asks; the
   board is `hev board`).
4. **Picker.** When the Picker is installed (a `hev pick` subcommand or a
   `hev-pick` plugin on `PATH`), onboarding opens it. Otherwise it prints next
   steps: `hev s`, the dashboard URL (`http://127.0.0.1:8099` by default) and
   `hev dispatch`. `--no-picker` skips the Picker (tests, headless machines).

Answers are saved in `~/.hev/onboard.toml` (mode 0600). A re-run offers them as
defaults and never rewrites an existing kit or dispatch config; only
`onboard.toml` itself is replaced.

## Never prompting

Onboarding needs a terminal on both stdin and stdout. Without one, `hev up`
never asks anything and `hev onboard` exits with an error instead of waiting.
`hev up --non-interactive` never asks even on a terminal, for scripts and CI.
A closed stdin takes every default.

## Data layout

kit keeps each surface in `~/.hev/<surface>/`: `traces`, `board`, `orders`,
`peers`, `hooks` and `evidence`. A machine set up before this layout keeps
`~/.hev/archive`, `~/dispatch` and `~/.local/state/peers-v2` where they are;
see [storage modes](design/storage-modes.md#data-layout).
