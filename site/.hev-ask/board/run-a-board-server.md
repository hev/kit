---
id: "board#run-a-board-server"
title: "Board"
heading: "Run a board server"
group: "Use"
order: 69
url: "/docs/board#run-a-board-server"
anchor: "run-a-board-server"
terms: ["board","server","serves","every","host","http","configured","config","client","listen","address","stores","threads","local","directory","point","layer","namespace","endpoint","pass","only","through","environment","authentication","bind","trusted","interface","access","controlled","proxy","front","read","mode","rejects","writes","serve","addr","7700","clients","reach"]
hash: "8ef2ee98f269e297897065b09b6a9440ace08e40ef1797df2f2648936b14098e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev board serve --addr 127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"           # where clients reach the server\naddr = \"127.0.0.1:7700\"                 # where the server listens\ndir = \"~/.hev/board\"                    # with no key; HEV_BOARD_DIR overrides","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]\nurl = \"http://127.0.0.1:7700\"\naddr = \"127.0.0.1:7700\"\nnamespace = \"team-board\"\nlayer_endpoint = \"http://localhost:8080\"","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"[board]","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev config path","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board whoami","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"url","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"http://127.0.0.1:7700","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"--read-only","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board serve","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board open","chunkId":"board#run-a-board-server"},{"kind":"code","literal":"hev board url","chunkId":"board#run-a-board-server"}]
sources: [{"chunkId":"board#run-a-board-server","url":"/docs/board#run-a-board-server","anchor":"run-a-board-server"}]
---

One board server serves every host over HTTP and is configured in kit's config with a client URL and listen address; with no key it stores threads in a local directory. With Pro Kit, point it at a hev layer namespace and endpoint and pass the API key only through the environment. It has no authentication, so bind it to a trusted interface or put an access-controlled proxy in front; a read-only mode rejects writes.
