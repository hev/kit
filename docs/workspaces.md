# Workspace capture

See the [workspace guide](https://hevkit.com/docs/workspaces) for workspace
setup, host restrictions, and command selection. To capture a workspace's
transcripts into a separate Layer archive, put its trace settings in
`~/.hev/config.toml`.

```toml
[workspaces.bcc]
dir = "~/workspace/clients/bcc"
endpoint = "https://your-layer-gateway.example"
traces_namespace = "bcc-traces"
api_key_ref = "op://your-vault/your-item/credential"
mode = "workspace"
```

For a local workspace archive, set `archive_dir` in place of the Layer
endpoint and key reference. A workspace capture target with neither a Layer
endpoint nor a key reference defaults to the workspace's own local archive
directory if no `archive_dir` is set. The default namespace is
`<workspace>-traces`.

Existing nested `[workspaces.<name>.layer]` endpoint, namespace and
`api_key_op_ref` settings are also supported. Flat fields take precedence
over their nested equivalents. `api_key_op_ref` is an alias for
`api_key_ref`. Archived workspaces and workspaces restricted to other hosts
are skipped.

Workspace targets default to `mode = "workspace"`, which selects sessions
by their workdirs, using the most specific configured workspace. Set
`mode = "mirror"` to opt in to capturing sessions across all workspaces
into that archive. Check the destination's access policy before enabling a
mirror.

See [Capture to multiple trace archives](trace-archive.md) for named targets,
key reference formats, independent state, and status.
