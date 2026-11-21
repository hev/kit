---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 77
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","without","uses","local","files","shares","across","machines","people","clients","point","their","also","serves","serve","addr","7700","http","reach","listens","overrides","namespace","team","layer","endpoint","localhost","8080","factory","open","config","path","hevlayer","azkv","whoami","read","works","machine","needed","holds"]
hash: "c06eacf111f95375e01f18e09ee0db7435363d4398f7dd7cec902beb6f292f65"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"board.url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"FACTORY_BOARD_URL","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board serve","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board open","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"azkv://","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"op read","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

Without a key or board URL the CLI uses local files. A board server shares one board across machines and people; clients point their board URL at it, and it also serves the board's web UI.
