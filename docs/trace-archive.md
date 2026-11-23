# Capture to multiple trace archives

By default, `hevd` captures to the primary trace archive. The existing
`[layer]` settings and Layer environment variables select that archive.
When no key or Layer configuration is present, the primary uses the local
file archive. See the [trace archive guide](https://hevkit.com/docs/trace-archive)
for primary archive configuration and search.

Add a `[[capture.targets]]` table for each additional destination in
`~/.hev/config.toml` (or the file selected by `HEV_CONFIG`).

```toml
[[capture.targets]]
name = "club"
endpoint = "https://your-layer-gateway.example"
namespace = "club-traces"
api_key_ref = "secret://club-capture-key"

[[capture.targets]]
name = "cloud-test"
endpoint = "https://your-layer-gateway.example"
namespace = "cloud-test-traces"
api_key_ref = "env://CLOUD_TEST_CAPTURE_KEY"
workspaces = ["cloud-test"]

[[capture.targets]]
name = "local-project"
archive_dir = "~/private-traces"
namespace = "project-traces"
dirs = ["~/workspace/project"]
```

- Give each target a unique name and its own destination. The name `primary`
  is reserved. Use letters, digits, underscores, and hyphens, starting with a
  letter, digit, or underscore.
- For Layer, set `namespace` and a target-specific `api_key_ref`. The
  `secret://`, `op://`, and `azkv://` forms use the daemon's secret resolver.
  `env://VARIABLE` reads a variable from the daemon's environment. Omitting
  `endpoint` uses the default Layer gateway. Set `archive_dir` instead to
  write locally under that directory's namespace subdirectory.
- An empty filter selects mirror mode. Set `workspaces` or `dirs` for
  workspace mode. These filters are alternatives. A workdir qualifies if
  its most specific workspace is named or if it is inside an explicit
  directory. A session needs at least one qualifying workdir and every
  known workdir must qualify. Workspace mode captures transcript data.
- For targets configured directly on a workspace, see
  [Workspace capture](workspaces.md).

Restart `hevd` after editing target definitions. Each target scans on the
configured `[capture].scan_interval` and keeps its own cursor. The primary
continues using `~/.hev/index-state.json`. Additional targets use
`~/.hev/targets/<lowercase-name>/state.json`. Changing a target's filter
rescans its sources. Existing archive rows are retained.

Use `hev status`, `hev s`, or `hev daemon status` to check each target's
namespace, key source, last scan, indexed units, and error. A failed Layer key
read is reported for that target and retried on its next scan. Other targets
continue capturing. Unqualified trace commands keep using the primary
archive. Use the applicable workspace selection or credentials and namespace
to read an additional archive.
