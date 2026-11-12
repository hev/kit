---
id: "config-reference#where-data-is-kept"
title: "Config reference"
heading: "Where data is kept"
group: "Reference"
order: 68
url: "/docs/config-reference#where-data-is-kept"
anchor: "where-data-is-kept"
terms: ["data","kept","whether","traces","board","files","layer","decided","only","configured","config","environment","server","reads","unreadable","error","fallback","dispatch","records","queues","peers","admission","always","toml","path","hevlayer","init","join","namespace","archive","factory","hosts","name","keeps","everything","working","team","moves","thing","decides"]
hash: "f504739d02d9c1290197d047201f48c18fc9294b8d21186c6a27226affc32335"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/.hev/config.toml","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev config path","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] api_key","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev up","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev init","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev join","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] namespace","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"\"hev-traces\"","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[archive] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/archive","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[board] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/board","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_BOARD_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"op","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"FACTORY_BOARD_LAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[hosts.NAME].namespace","chunkId":"config-reference#where-data-is-kept"}]
sources: [{"chunkId":"config-reference#where-data-is-kept","url":"/docs/config-reference#where-data-is-kept","anchor":"where-data-is-kept"}]
---

Whether traces and the board use files or hev layer is decided only by whether a hev layer API key is configured in kit's config or environment. The board server reads its key only from the environment, an unreadable key is an error with no fallback to files, and dispatch run records, queues, peers and admission are always files.
