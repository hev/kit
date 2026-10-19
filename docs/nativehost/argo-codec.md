# Reference-only Argo task codec (disabled)

This is source plumbing, not a runnable host bridge. No endpoint, command-line
flag, preparation, continuation, Workflow client, DB claim or start is added.
The existing `native-receiver/v1` and `native-receiver-rpc/v1` protocols are unchanged.

Root `pkg/nativehost` provides `EncodeArgoTaskEnvelope` and
`DecodeArgoTaskEnvelope` for a separate `native-argo-task-rpc/v1` routing envelope:

```json
{"version":"native-argo-task-rpc/v1","profile_id":"local-profile","config_digest":"sha256:...","task_reference":{}}
```

The task object is bounded to 16384 bytes; the envelope uses the existing 64KiB
strict frame decoder. Root validates JSON framing, duplicates/aliases, routing
fields and limits, **not** Factory task schema or authority. It never imports
Factory. The optional adapter's `DecodeArgoTaskReference(io.Reader)` delegates
schema validation to `github.com/hev/factory/pkg/argohost.ParseTaskReference`
at exact Factory `2375bc517d82301464384f3d0a8cef616474f5ad`.

The future fixed `--argo-task` handler remains owned by Factory integration.
Before any effect it must independently bind the actual pinned local profile and
configuration, resolve/check canonical source and every parent artifact, observe
the exact Workflow UID/node/step/attempt, and retain immutable attempt identity.
A Running node or valid task reference is never launch permission. Preparation
and same-owner continuation are still missing capabilities. The existing DB
claim, final native gate, postcommit readiness and original completion/ACK remain
required. A lost response requires inspecting the same original; it never
permits blind task replay, PID adoption or a second native original.

The optional module explicitly pins the new Factory parser source, retaining
its previous SDK b4 pin. This candidate does not update deployed binaries or
transfer previous native acceptance to the new pair.
