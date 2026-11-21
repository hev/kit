---
id: "api/hooks#gating-from-a-host"
title: "Hooks API"
heading: "Gating from a host"
group: "API"
order: 36
url: "/docs/api/hooks#gating-from-a-host"
anchor: "gating-from-a-host"
terms: ["gating","host","harness","event","sent","hosted","events","route","after","normalization","server","records","redacted","queues","observe","only","hooks","calls","blocking","parallel","under","their","timeouts","returns","merged","decision","01jd2x8c7k4m5n6p7q8r9s0t1v","block","reason","force","pushes","agents","failed","open","elapsed","hook","post","timeout","context","name"]
hash: "6448e9883e1936935dc0e1abcc701ddcd1252d0fb3437f04c3718020356df1f2"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"event_id\": \"01jd2x8c7k4m5n6p7q8r9s0t1v\",\n  \"decision\": \"block\",\n  \"reason\": \"No force pushes from agents.\",\n  \"blocking_hooks\": 2,\n  \"failed_open\": 0,\n  \"elapsed_ms\": 212\n}","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"hev hook","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"POST /v1/hooks/events","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"timeout_ms","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"add_context","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"hev-hook-NAME","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"steer","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"peers-inbox","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"claude","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"codex","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"id","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"include_content","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"allow","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"failed_open","chunkId":"api/hooks#gating-from-a-host"},{"kind":"code","literal":"[hooks] timeout","chunkId":"api/hooks#gating-from-a-host"},{"kind":"value","literal":"non-2xx","chunkId":"api/hooks#gating-from-a-host"}]
sources: [{"chunkId":"api/hooks#gating-from-a-host","url":"/docs/api/hooks#gating-from-a-host","anchor":"gating-from-a-host"}]
---

On a Pro host, each harness event is sent to the hosted events route after normalization; the server records a redacted event, queues observe-only hooks, calls blocking hooks in parallel under their timeouts and returns the merged decision.
