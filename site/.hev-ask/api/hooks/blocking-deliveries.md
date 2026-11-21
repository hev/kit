---
id: "api/hooks#blocking-deliveries"
title: "Hooks API"
heading: "Blocking deliveries"
group: "API"
order: 33
url: "/docs/api/hooks#blocking-deliveries"
anchor: "blocking-deliveries"
terms: ["blocking","deliveries","hook","receives","session","start","prompt","submit","tool","events","harness","waits","reply","response","decision","allow","block","empty","controls","action","reason","force","pushes","agents","true","post","halt","continue","context","system","message","timeout","hooks","answer","body","field","default","local","word","means"]
hash: "b3168031cc1d53ab24cdedc759ac83f8e1a9e701f43518e2fdef8e4d01746977"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"decision\": \"block\", \"reason\": \"No force pushes from agents.\" }","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"blocking: true","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"session-start","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"prompt-submit","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"pre-tool","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"post-tool","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"200","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"204","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"decision","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"allow","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"block","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"halt","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"continue","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"reason","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"add_context","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"context","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"system_message","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"timeout_ms","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"[hooks] timeout","chunkId":"api/hooks#blocking-deliveries"},{"kind":"code","literal":"hev hook","chunkId":"api/hooks#blocking-deliveries"},{"kind":"value","literal":"non-2xx","chunkId":"api/hooks#blocking-deliveries"}]
sources: [{"chunkId":"api/hooks#blocking-deliveries","url":"/docs/api/hooks#blocking-deliveries","anchor":"blocking-deliveries"}]
---

A blocking hook receives session-start, prompt-submit and tool events and the harness waits for its reply. A 200 hook response with a decision (allow, block, etc.) or an empty/204 reply to allow controls the action.
