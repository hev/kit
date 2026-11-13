---
id: "traces#images"
title: "Traces"
heading: "Images"
group: "Use"
order: 307
url: "/docs/traces#images"
anchor: "images"
terms: ["images","hevd","keeps","session","showed","agent","pasted","screenshots","browser","captures","reads","claude","code","codex","opencode","transcripts","transcript","text","holds","short","marker","image","list","save","including","json","searching","word","finds","sessions","pictures","390x700","sha256","3f2a9c1b04de","traces","show","3f2a","markers","inline","type"]
hash: "341bb6bf3cf1aa561052c44647b9f6fcff2fae6da002c88efb445e797b2390a5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de]","chunkId":"traces#images"},{"kind":"code","literal":"hev traces show 3f2a                      # markers inline in the transcript\nhev traces images 3f2a                    # each image: type, dimensions, size, hash\nhev traces images 3f2a --out ./seen       # save them in transcript order; prints each path\nhev traces images 3f2a --json             # the same, for scripts and agents","chunkId":"traces#images"},{"kind":"code","literal":"[capture]\nimages = false","chunkId":"traces#images"},{"kind":"code","literal":"Read","chunkId":"traces#images"},{"kind":"code","literal":"image","chunkId":"traces#images"},{"kind":"code","literal":"blobs/sha256/","chunkId":"traces#images"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#images"},{"kind":"code","literal":"HEVD_CAPTURE_IMAGES=0","chunkId":"traces#images"}]
sources: [{"chunkId":"traces#images","url":"/docs/traces#images","anchor":"images"}]
---

hevd keeps images a session showed its agent (pasted screenshots, browser captures, PNG reads) from Claude Code, Codex, pi and OpenCode transcripts. The transcript text holds a short marker where an image was, and you can list or save images from a session, including as JSON. Searching for the word image finds sessions with pictures.
