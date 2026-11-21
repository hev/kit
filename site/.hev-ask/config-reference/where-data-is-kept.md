---
id: "config-reference#where-data-is-kept"
title: "Config reference"
heading: "Where data is kept"
group: "Reference"
order: 164
url: "/docs/config-reference#where-data-is-kept"
anchor: "where-data-is-kept"
terms: ["data","kept","starter","keeps","everything","files","moves","traces","board","layer","decided","solely","whether","config","toml","path","hevlayer","init","join","namespace","archive","factory","hosts","name","working","team","there","only","thing","decides","these","settings","prints","dispatch","setting","default","meaning","apikey","none","file"]
hash: "02bda626e8098d7f7c04d11d34e59ea10894684af9c39117806caa4e3b6df246"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/.hev/config.toml","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev config path","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] api_key","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev up","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev init","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev join","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] namespace","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"\"hev-traces\"","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[archive] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/traces","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/archive","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[board] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/board","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_BOARD_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"op","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"FACTORY_BOARD_LAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[hosts.NAME].namespace","chunkId":"config-reference#where-data-is-kept"},{"kind":"value","literal":"0.7","chunkId":"config-reference#where-data-is-kept"}]
sources: [{"chunkId":"config-reference#where-data-is-kept","url":"/docs/config-reference#where-data-is-kept","anchor":"where-data-is-kept"}]
---

Starter Kit keeps everything in files; Pro Kit moves traces and the board to a hev layer, decided solely by whether a hev layer API key is set in kit's config.
