# Resource CLI (removed)

The resource backend bridge is gone. `hev get`, `apply`, `describe`, `logs`,
`wait`, `api-resources`, `controller`, `orders` and `runs` no longer delegate
to an external executable. They are hidden stubs that exit 1 with
`hev VERB has been removed; use hev dispatch or hev orchestrate`. The
`HEV_RESOURCE_BACKEND` and `HEV_COORDINATION_BACKEND` variables are no longer read.

`hev get`, `hev apply` and `hev describe` are the natural verbs for a resource surface, and the old backend that served them is gone. A future resource surface should reuse those verbs over kit's native state (dispatch and the orchestrator), not a separate coordination service: for example `hev get runs`, `hev describe run NAME` and `hev apply -f order.md`. Nothing here implements that surface. Until it exists, the verbs fail with a message that points to `hev dispatch` and `hev orchestrate`, and no executable is looked up on PATH. See [resource verbs](design/resource-verbs.md).

## Operational orientation

Bare `hev` shows a read-only overview of `hev peers`; `hev --json` emits
versioned section data. It makes one five-second read with no retries. Missing or
failed observations are unknown, and exit 1 signals a failed read. The former
nodes and orders sections are not available; the text view says so and the JSON
omits them. Help stays offline. Use `hev browse` for the interactive trace browser.
