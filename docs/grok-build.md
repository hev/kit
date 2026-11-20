# Grok Build

Use `harness: grok` in a dispatch order, orchestrator or swarm. Kit runs the
`grok` executable in single-prompt mode with streaming JSON. An explicit
`model` or `effort` is passed to Grok. The child can inherit `XAI_API_KEY`
or receive it through `[harnesses.grok.env_refs]` in dispatch configuration.

Kit reads Grok's ACP session updates from `~/.grok/sessions` or
`$GROK_HOME/sessions`. Local and project skills are available through
`~/.grok/skills` and `.grok/skills`, alongside Grok's documented
compatibility directories.

`hev hooks install --harness grok` uses a dedicated `hooks/hev-kit.json`
under the Grok home. This loads alongside Grok's TOML hook tables and
keeps user configuration intact. Dispatch has no session-scoped Grok hook
injection, so kit uses its process and turn-boundary controls for those runs.

Peers recognizes Grok's npm package executable and its distinctive
single-prompt streaming command. An unqualified interactive process named
`grok` is ambiguous. Grok peers are observation-only.
