---
id: "config-reference#where-data-is-kept"
title: "Config reference"
heading: "Where data is kept"
group: "Reference"
order: 68
url: "/docs/config-reference#where-data-is-kept"
anchor: "where-data-is-kept"
terms: ["data","kept","alone","keeps","everything","files","layer","config","decides","whether","traces","board","move","lists","settings","file","dispatch","toml","path","hevlayer","init","join","namespace","archive","factory","hosts","name","working","team","moves","only","thing","these","prints","setting","default","meaning","apikey","none","hevlayerapikey"]
hash: "f504739d02d9c1290197d047201f48c18fc9294b8d21186c6a27226affc32335"
mode: "source-primary"
facts: [{"kind":"code","literal":"~/.hev/config.toml","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev config path","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] api_key","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev up","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev init","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"hev join","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[layer] namespace","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"\"hev-traces\"","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[archive] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/archive","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_ARCHIVE_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[board] dir","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"~/.hev/board","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEV_BOARD_DIR","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"op","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"FACTORY_BOARD_LAYER_API_KEY","chunkId":"config-reference#where-data-is-kept"},{"kind":"code","literal":"[hosts.NAME].namespace","chunkId":"config-reference#where-data-is-kept"}]
sources: [{"chunkId":"config-reference#where-data-is-kept","url":"/docs/config-reference#where-data-is-kept","anchor":"where-data-is-kept"}]
---

Kit alone keeps everything in files; a hev layer API key in kit's own config decides whether traces and the board move to hev layer. Lists the settings in that file, not the dispatch config.
