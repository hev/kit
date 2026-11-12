---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 15
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","talks","http","started","serve","command","under","tmux","supervisor","authentication","bind","only","localhost","trusted","private","network","settings","live","section","config","whoami","boards","verify","addr","7700","clients","reach","listens","overrides","namespace","team","layer","endpoint","8080","path","hevlayer","read","serves","every"]
hash: "7f987ae5706d3be16038d639d3daac9cededa5285a664e34002bdeb0567c8605"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op read","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

The CLI talks to one board server over HTTP, started with a serve command under tmux or a supervisor. It has no authentication, so bind it only to localhost or a trusted private network. Settings live in the board section of kit's config, and whoami and boards verify it.
