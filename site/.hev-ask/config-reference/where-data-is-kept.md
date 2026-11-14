---
id: "config-reference#where-data-is-kept"
title: "Config reference"
heading: "Where data is kept"
group: "Reference"
order: 123
url: "/docs/config-reference#where-data-is-kept"
anchor: "where-data-is-kept"
terms: ["data","kept","starter","keeps","everything","files","moves","traces","board","layer","only","decides","settings","archive","namespace","directory","live","config","environment","overrides","server","reads","unreadable","error","rather","fallback","dispatch","records","queues","peers","admission","always","toml","path","hevlayer","init","join","factory","hosts","name"]
hash: "0591fbf9563a592c61cfa5c4d347c37850db7e362dc9cd0d3e627bcc1d2fecf6"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/.hev/config.toml","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev config path","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] api_key","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev up","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev init","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev join","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] namespace","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"\"hev-traces\"","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[archive] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/archive","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[board] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/board","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_BOARD_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"op","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"FACTORY_BOARD_LAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[hosts.NAME].namespace","chunkId":"config-reference#where-data-is-kept"}]
sources: [{"chunkId":"config-reference#where-data-is-kept","url":"/docs/config-reference#where-data-is-kept","anchor":"where-data-is-kept"}]
---

Starter Kit keeps everything in files; Pro Kit moves traces and the board to your own hev layer, and only a hev layer API key decides which. Settings for key, archive namespace, archive directory and board directory live in kit's config, with environment overrides; the board server reads its key only from the environment, and an unreadable key is an error rather than a fallback. Dispatch run records, queues, peers and admission are always files.
