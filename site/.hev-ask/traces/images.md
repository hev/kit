---
id: "traces#images"
title: "Traces"
heading: "Images"
group: "Use"
order: 369
url: "/docs/traces#images"
anchor: "images"
terms: ["images","session","shows","agent","image","pasted","screenshot","browser","computer","capture","file","read","daemon","keeps","later","transcript","holds","short","marker","list","save","bytes","stay","capturing","machine","stored","once","unique","shared","archive","only","receives","type","size","dimensions","hash","another","listed","cannot","saved"]
hash: "341bb6bf3cf1aa561052c44647b9f6fcff2fae6da002c88efb445e797b2390a5"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de]","chunkId":"traces#images"},{"kind":"code","literal":"hev traces show 3f2a                      # markers inline in the transcript\nhev traces images 3f2a                    # each image: type, dimensions, size, hash\nhev traces images 3f2a --out ./seen       # save them in transcript order; prints each path\nhev traces images 3f2a --json             # the same, for scripts and agents","chunkId":"traces#images"},{"kind":"code","literal":"[capture]\nimages = false","chunkId":"traces#images"},{"kind":"code","literal":"Read","chunkId":"traces#images"},{"kind":"code","literal":"image","chunkId":"traces#images"},{"kind":"code","literal":"blobs/sha256/","chunkId":"traces#images"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"traces#images"},{"kind":"code","literal":"HEVD_CAPTURE_IMAGES=0","chunkId":"traces#images"}]
sources: [{"chunkId":"traces#images","url":"/docs/traces#images","anchor":"images"}]
---

When a session shows its agent an image (pasted screenshot, browser or computer-use capture, an image file it read), the daemon keeps it so a later agent can see what that session saw; the transcript holds a short marker and you can list or save a session's images. Image bytes stay on the capturing machine, stored once per unique image, and a shared archive only receives type, size, dimensions and hash, so images from another machine are listed but cannot be saved locally. PNG, JPEG, GIF and WebP up to 10 MB are kept, and image capture can be turned off, in which case markers are still written but no bytes are stored.
