---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 70
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","talks","http","serves","every","host","needs","only","listen","address","keeps","threads","local","directory","given","layer","namespace","endpoint","supplied","environment","never","config","file","reports","mode","started","falls","back","files","configured","cannot","read","authentication","must","bound","localhost","trusted","network","clients"]
hash: "d6877b3c2ef99995b89e4bdbea21610763e3d5bde7f1a2a390a1c932b4123993"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"azkv://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op read","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

The CLI talks to one board server over HTTP, and one server serves every host. With no key it needs only a listen address and keeps threads in a local directory; with Pro Kit it is given a hev layer namespace and endpoint, with the key supplied in its environment and never in the config file. The server reports which mode it started in, never falls back to files when a configured key cannot be read, and has no authentication so it must be bound to localhost or a trusted network; clients only need the server's URL.
