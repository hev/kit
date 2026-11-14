---
id: "upgrade#pro-kit"
title: "Upgrade"
heading: "Pro Kit"
group: "Start"
order: 358
url: "/docs/upgrade#pro-kit"
anchor: "pro-kit"
terms: ["points","layer","cloud","against","store","endpoint","nothing","passing","through","hosted","service","export","before","setup","configure","interactively","daemon","reads","config","while","board","server","only","environment","unreadable","error","rather","file","fallback","front","turbopuffer","dockerized","gateway","hevlayer","init","toml","hevd","read","pointed","yourself"]
hash: "22d395c80c8063e37c3b53f2f3f363ee861f642f401825fbc951e316e600705c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev up","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev init","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hevd","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"op read","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev up --store turbopuffer","chunkId":"upgrade#pro-kit"}]
sources: [{"chunkId":"upgrade#pro-kit","url":"/docs/upgrade#pro-kit","anchor":"pro-kit"}]
---

Pro Kit points kit at a hev layer you run in your own cloud against your own store, with your own endpoint and key and nothing passing through the hosted service. Export the key before setup or configure interactively; the daemon reads the key from config while the board server reads it only from the environment, an unreadable key is an error rather than a file fallback, and you can front your own turbopuffer with a Dockerized gateway.
