---
id: "traces#where-traces-are-kept"
title: "Traces"
heading: "Where traces are kept"
group: "Use"
order: 292
url: "/docs/traces#where-traces-are-kept"
anchor: "where-traces-are-kept"
terms: ["traces","kept","layer","live","files","under","home","archive","directory","session","chunks","blocks","word","index","needing","endpoint","account","docker","moved","config","environment","variable","wins","machines","write","namespace","nothing","goes","unreadable","configured","error","never","silent","fallback","toml","hevlayer","read","hevd","keeps","machine"]
hash: "f265c4a1d8bb3faea4f7c9f52f6eac6df3e9cb830518c1b5aed3246b8c39077e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/archive/hev-traces/","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"[archive] dir","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"op","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"api_key","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"[layer]","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"op read","chunkId":"traces#where-traces-are-kept"},{"kind":"code","literal":"index/","chunkId":"traces#where-traces-are-kept"},{"kind":"value","literal":"hevlayer.com","chunkId":"traces#where-traces-are-kept"}]
sources: [{"chunkId":"traces#where-traces-are-kept","url":"/docs/traces#where-traces-are-kept","anchor":"where-traces-are-kept"}]
---

With no layer key traces live as files under the home archive directory, one per session with chunks, blocks and a word index, needing no endpoint, account or Docker. The directory can be moved by config or an environment variable that wins. With a key, all machines write to one layer namespace and nothing goes to files. An unreadable configured key is an error, never a silent fallback.
