# RFC 0020 · Harness registry

Status: Implemented (Phase 0)

## Summary and motivation

A harness descriptor in `internal/harness/registry.go` gives kit one place to
declare a harness's identity and supported capabilities. The registry imports
only the standard library. The subsystems own their native adapters and depend
on the registry, so the registry never imports them back.

## Decision

`Lookup` resolves canonical names and aliases. `TraceName` is the established
identity in trace records. Dispatch and hooks retain their canonical-only CLI
vocabulary. A nil capability means the subsystem has no integration.

| Subsystem | Interface |
|-----------|-----------|
| Dispatch | `DispatchFor` gates orders, intake, validation and orchestrators. `Dispatch.Argv` builds the headless invocation from model, effort and prompt, including the outside-repository flag where supported. `CallerIdentity` reads the descriptor's session environment. Optional harness environment entries layer over the supervisor’s shared requester and trace context. |
| Hooks | `HooksFor` gives the event names, settings path and environment override, invocation settings format, response shape and trust requirement. The hooks package renders payloads and installs entries. |
| Peers | `Peers` describes process matching, headless detection and whether screen-based delivery or inbox-only delivery is available. The engine and compatibility detector retain their existing matching rules and precedence. |
| Traces | `Trace.Root` resolves the standard root and environment overrides. `internal/trace/registry.go` binds descriptors to `Source` factories. The daemon and CLI share discovery through `Sources`. `SourceInfo` exposes the native root and persisted name for indexing. |
| Skills | `Skills` lists user and project skill directories, instruction files, and install targets and markers. `SkillTargets` exposes the existing metadata targets. Dispatch uses the harness's designated project write directory. |

## Goals and non-goals

Add future capabilities through descriptors and adapters. Preserve flags,
settings and archive formats, source discovery, and harness availability.
Registration alone does not enable dispatch, install hooks, or grant peer
delivery. Phase 0 adds no harnesses.

## How to add a harness

- Add the canonical name, executable, aliases, and only the supported
  capabilities in `internal/harness/registry.go`. Extend `TestLookup` and
  `TestCapabilities` in `internal/harness/registry_test.go`.
- For dispatch, set the headless arguments, model flag, effort builder, prompt prefix, project skill path,
  and session environment. If the harness needs its own launch behavior, add an
  adapter used by `internal/dispatch/run.go` and `internal/dispatch/orchestrator.go`.
  Exercise admission in `internal/dispatch/classes_test.go` and launch with an
  isolated fake executable. Update the order, orchestrator and swarm schemas
  when their accepted values expand.
- For hooks, supply verified event, path, trust and response settings. Extend
  `internal/hooks/install.go` and `internal/hooks/response.go` for new wire shapes.
  Add native payload and response fixtures in `internal/hooks/hooks_test.go`
  and installation cases in `internal/hooks/install_test.go`.
- For peers, add the process fingerprints, matching precedence and delivery
  capability. Update both `PeerDetectionOrder` and `CompatDetectionOrder`
  in `internal/harness/registry.go`, documenting any intentional omission
  from either detector. Test real process command lines and pane states in
  `internal/peers/compat` and `internal/peers`. Verify delivery in an isolated
  tmux session before enabling it.
- For traces, add the parser in `internal/trace/<name>.go` and bind its factory
  and discovery identity in `internal/trace/registry.go`. Add sanitized
  fixtures in `internal/trace/testdata/` and parser tests next to the adapter.
  Exercise daemon and CLI discovery if adding an explicit configured root.
- For skills, set the user and project paths and install markers. Cover
  instruction discovery in `internal/index/startcontext_test.go` and exercise
  installation using `scripts/test-skill-sources.py` with a temporary home.
- Run `go build ./...`, `go vet ./...` and `go test ./...`.

## Open questions

A new hook protocol, launch mode, or peer screen may need a new adapter. Use
fixtures to make that decision when the first harness with that behavior lands.
