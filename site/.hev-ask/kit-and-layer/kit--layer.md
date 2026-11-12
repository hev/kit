---
id: "kit-and-layer#kit--layer"
title: "kit + layer"
heading: "kit + layer"
group: "Start"
order: 152
url: "/docs/kit-and-layer#kit--layer"
anchor: "kit--layer"
terms: ["layer","switch","exporting","before","setup","running","interactive","init","redeeming","invite","saved","config","daemon","while","board","server","reads","only","environment","configured","unreadable","error","fallback","bringing","turbopuffer","account","runs","gateway","docker","export","hevlayer","join","code","toml","hevd","read","store","redeem","machine","traces"]
hash: "ff2fe075eba8631bb0fe1c4dd3a62e62681fc4cb6fbf4e9a7d2cbe429f576d2a"
mode: "agent-primary"
facts: [{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"hev up","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"hev init","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"hev join CODE","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"hevd","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"op read","chunkId":"kit-and-layer#kit--layer"},{"kind":"code","literal":"hev up --store turbopuffer","chunkId":"kit-and-layer#kit--layer"}]
sources: [{"chunkId":"kit-and-layer#kit--layer","url":"/docs/kit-and-layer#kit--layer","anchor":"kit--layer"}]
---

Switch to kit + layer by exporting a key before setup, running interactive init, or redeeming an invite; the key is saved in kit's config for the daemon, while the board server reads it only from the environment. A configured but unreadable key is an error with no fallback, and bringing your own turbopuffer account runs the gateway in Docker.
