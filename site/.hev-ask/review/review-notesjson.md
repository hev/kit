---
id: "review#review-notesjson"
title: "Review"
heading: "review-notes.json"
group: "Use"
order: 359
url: "/docs/review#review-notesjson"
anchor: "review-notesjson"
terms: ["review","notes","json","every","dispatched","writes","beside","report","hunk","agent","context","sidecar","format","version","head","summary","file","summaries","line","range","annotations","rationale","loadable","directly","orders","name","diff","files","path","newrange","oldrange","start","author","tags","confidence","source","medium","high","finding","fail"]
hash: "03d1834a7a7c5f4209ead68720d1b03b22ba3f309272ffddcbbf742ee2e31b58"
mode: "agent-primary"
facts: [{"kind":"code","literal":"review-notes.json","chunkId":"review#review-notesjson"},{"kind":"code","literal":"report.md","chunkId":"review#review-notesjson"},{"kind":"code","literal":"~/.hev/orders/NAME/","chunkId":"review#review-notesjson"},{"kind":"code","literal":"hunk diff --agent-context review-notes.json","chunkId":"review#review-notesjson"},{"kind":"code","literal":"head","chunkId":"review#review-notesjson"},{"kind":"code","literal":"summary","chunkId":"review#review-notesjson"},{"kind":"code","literal":"files[].path","chunkId":"review#review-notesjson"},{"kind":"code","literal":"files[].summary","chunkId":"review#review-notesjson"},{"kind":"code","literal":"annotations[].summary","chunkId":"review#review-notesjson"},{"kind":"code","literal":"newRange","chunkId":"review#review-notesjson"},{"kind":"code","literal":"oldRange","chunkId":"review#review-notesjson"},{"kind":"code","literal":"[start, end]","chunkId":"review#review-notesjson"},{"kind":"code","literal":"rationale","chunkId":"review#review-notesjson"},{"kind":"code","literal":"author","chunkId":"review#review-notesjson"},{"kind":"code","literal":"tags","chunkId":"review#review-notesjson"},{"kind":"code","literal":"confidence","chunkId":"review#review-notesjson"},{"kind":"code","literal":"source","chunkId":"review#review-notesjson"},{"kind":"code","literal":"low","chunkId":"review#review-notesjson"},{"kind":"code","literal":"medium","chunkId":"review#review-notesjson"},{"kind":"code","literal":"high","chunkId":"review#review-notesjson"},{"kind":"code","literal":"[\"finding\", \"fail\"]","chunkId":"review#review-notesjson"},{"kind":"code","literal":"[\"finding\", \"note\"]","chunkId":"review#review-notesjson"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"review#review-notesjson"}]
sources: [{"chunkId":"review#review-notesjson","url":"/docs/review#review-notesjson","anchor":"review-notesjson"}]
---

Every dispatched run writes review-notes.json beside its report, in hunk's agent-context sidecar format version 1 (head SHA, summary, per-file summaries and line-range annotations with rationale), loadable directly by hunk.
