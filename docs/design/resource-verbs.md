# Resource verbs

`hev get`, `hev apply` and `hev describe` are the natural verbs for a resource surface, and the old backend that served them is gone. A future resource surface should reuse those verbs over kit's native state (dispatch and the orchestrator), not a separate coordination service: for example `hev get runs`, `hev describe run NAME` and `hev apply -f order.md`. Nothing here implements that surface. Until it exists, the verbs fail with a message that points to `hev dispatch` and `hev orchestrate`, and no executable is looked up on PATH.
