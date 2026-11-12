---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 14
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","serves","hosts","http","configured","under","section","config","listen","address","storage","directory","without","namespace","layer","endpoint","supplied","only","through","environment","read","mode","rejects","writes","authentication","bind","localhost","trusted","network","commands","open","print","serve","addr","7700","clients","reach","listens","overrides"]
hash: "13a078aeb097114bbc8a4eefecabf2f384ec1265b980f9ba1a567343d450fe39"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"--read-only","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board serve","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board open","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board url","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

One board server serves all hosts over HTTP and is configured under a board section in kit's config: a listen address and storage directory without a key, or a namespace and layer endpoint with the API key supplied only through the environment. A read-only mode rejects writes, but the server has no authentication, so bind it to localhost or a trusted network. Commands open the web UI and print its address.
