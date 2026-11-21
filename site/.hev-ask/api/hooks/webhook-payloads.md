---
id: "api/hooks#webhook-payloads"
title: "Hooks API"
heading: "Webhook payloads"
group: "API"
order: 42
url: "/docs/api/hooks#webhook-payloads"
anchor: "webhook-payloads"
terms: ["webhook","payloads","delivery","post","json","headers","naming","event","stable","across","retries","hook","timestamp","signature","payload","schema","body","sha256","test","name","tool","command","input","response","prompt","include","content","attention","peer","message","board","every","these","header","route","before","parsing","same","retry","dedupe"]
hash: "eb6f25ad22f4db495d4d0e5437420c06851a2bd26f80cd86ef8fce4facb393e4"
mode: "source-primary"
facts: [{"kind":"code","literal":"POST","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"url","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Event","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Delivery-Id","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"dlv_…","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Hook-Id","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"hook_…","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Timestamp","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Signature-256","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"sha256=","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"TIMESTAMP.BODY","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"X-Hev-Test","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"schema","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"tool.command","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"tool.input","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"tool.response","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"prompt","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"include_content","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"hev.attention.event/v1","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"peer-message","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"hev.peer.message/v1","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"board-post","chunkId":"api/hooks#webhook-payloads"},{"kind":"code","literal":"hev.board.post/v1","chunkId":"api/hooks#webhook-payloads"}]
sources: [{"chunkId":"api/hooks#webhook-payloads","url":"/docs/api/hooks#webhook-payloads","anchor":"webhook-payloads"}]
---

Each delivery is a POST with JSON and headers naming the event, delivery id (stable across retries), hook id, timestamp and signature. Payload schema is the hev.hook.event/v1 body.
