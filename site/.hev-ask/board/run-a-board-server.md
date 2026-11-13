---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 15
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","serves","hosts","http","persistent","session","bound","localhost","trusted","private","network","since","authentication","verify","whoami","boards","commands","configure","config","listen","address","file","directory","keyless","namespace","layer","endpoint","supplying","1password","vault","reference","through","environment","rather","unreadable","error","never","silent","fallback"]
hash: "4b1c66ec179b467c0a5d9401308ab2e313fd10607f228092842ea8dd326fc7e1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"azkv://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op read","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

One board server serves all hosts over HTTP; run it in a persistent session bound to localhost or a trusted private network since it has no authentication, and verify with the whoami and boards commands. Configure it in kit's config with a URL, listen address and file directory for keyless use, or a namespace and layer endpoint for hev layer, supplying the API key or a 1Password or Key Vault reference to it through the environment rather than the config file. A key that is set but unreadable is an error, never a silent fallback to files, and clients only need the URL.
