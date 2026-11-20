# Factory Droid

Use `harness: droid` in dispatch orders, orchestrators and swarms. Install
Factory's `droid` executable on the execution host. For headless authentication,
set `FACTORY_API_KEY` on that host or resolve it through
`[harnesses.droid.env_refs]` in dispatch configuration. Droid does not use kit's
`model_provider` setting.

Kit launches a one-shot `droid exec` with `--auto high` and `--output-format json`.
The JSON result is captured in the run log, and kit records the session ID.
Dispatch passes a verified context bundle through a system prompt file when
the order includes a context recipe. Set `model` to a
Factory model ID and use a reasoning effort supported by that model.
Droid's autonomy policy still applies. Headless exec also runs repository and
enabled project plugin hooks without the folder-trust prompt used by the TUI.

`hev hooks install --harness droid` installs the kit hooks in
`~/.factory/settings.json`. Droid loads an event from `~/.factory/hooks.json`
in preference to the same event in user settings. The TUI runs hooks after
folder trust. Both modes use SessionStart on session startup or resume,
UserPromptSubmit before a model prompt, PreToolUse and PostToolUse around tool
calls, Stop at normal turn end, and PreCompact when compacting. A one-shot
prompt rejected by SessionStart never reaches UserPromptSubmit
and a prompt rejected by UserPromptSubmit ends the one-shot run.
Kit uses turn-boundary steering because it does not install invocation-scoped
Droid hooks for managed runs.

Peers detects interactive `droid` and headless `droid exec` processes. Terminal
paste delivery is disabled. Register an inbox to queue messages for a Droid pane. Skills use `~/.factory/skills/` and project
`.factory/skills/`. Use `harnesses: [factory]` in source skill metadata for a
Factory-only install.

Trace discovery reads v2 JSONL transcripts in project directories under
`~/.factory/sessions/`. It indexes conversation text, thinking and tool
calls/results, skipping hook audit records and unrecognized blocks.
