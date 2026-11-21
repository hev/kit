# Workspaces

Use a workspace to associate harness subscriptions and authentication, a
home directory, and a private network with a cluster.

```toml
active_workspace = "hev"

[workspaces.hev]
dir = "~/workspace"
hosts = ["tower", "oven", "mini"]

[workspaces.bcc]
dir = "~/workspace/clients/bristlecone"
hosts = ["win-oven"]
login = "own"

[workspaces.bcc.subscriptions.env_refs]
GEMINI_API_KEY = "secret://bcc-gemini"

[workspaces.bcc.cluster]
network = "bcc"
```

Put the configuration in `~/.hev/config.toml` or the file named by
`HEV_CONFIG`. Without configuration, `hev` uses `~/workspace`. The global
`-w` flag takes precedence over `HEV_WORKSPACE`, the longest canonical home
directory matching the current directory, the saved `active_workspace`, and
the default `hev`. Invalid explicit selections fail. Archived workspaces are
excluded from selection.

`hev workspace ls`, `current`, and `show NAME` support JSON output.
`use NAME` saves the fallback selection. Use `add NAME --dir PATH` to create
a workspace, `archive NAME` to retire it, and `import --themes FILE --lists FILE`
to import shell themes and client lists.

Provision the same workspace name on the dispatch target. With `login = "own"`,
harness profiles live under `~/.hev/workspaces/NAME/harnesses/`. Use
`claude_config_dir`, `codex_home`, or `agy_data_dir` under `subscriptions`
to override a profile. The target resolves subscription `env_refs` at launch.

Set `layer.endpoint`, `traces_namespace`, `board_namespace`, `board_url`, and
`api_key_op_ref` under the workspace to select hosted targets. Client
workspaces without a Layer reference use `~/.hev/workspaces/NAME/archive`
and `~/.hev/workspaces/NAME/board`. To migrate from `hev bucket`, copy the
client archive endpoint, namespace and key reference into the named workspace
and configure its board separately. Existing bucket workflows remain available
in the default workspace.

An empty host list preserves the configured host inventory. A populated list
limits dispatch routing and cluster, orchestrator and peer inventory. Configure
the workspace's private network and SWIM daemon on its member hosts.
