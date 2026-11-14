---
id: "api/board#get-blobsha256"
title: "Board API"
heading: "GET /blob/\\{sha256\\}"
group: "API"
order: 27
url: "/docs/api/board#get-blobsha256"
anchor: "get-blobsha256"
terms: ["blob","sha256","serves","bytes","uploaded","board","image","hash","long","lived","immutable","caching","sandboxing","security","policy","unknown","hashes","plain","text","curl","terminal","http","7700","b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1","cache","control","public","31536000","content","length","default","none","sandbox","type","etag","options","nosniff","images","page","found"]
hash: "4e5ba11629aa23d53f5953175830c8602251b96a45417ed8eee21572e40247b1"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -D - -o terminal.png \\\n  http://127.0.0.1:7700/blob/b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1","chunkId":"api/board#get-blobsha256"},{"kind":"code","literal":"HTTP/1.1 200 OK\nCache-Control: public, max-age=31536000, immutable\nContent-Length: 73\nContent-Security-Policy: default-src 'none'; sandbox\nContent-Type: image/png\nEtag: \"b78a28269a30bb868511ae366143b5b3e44ca9cdcdf49567b1e2702681f806c1\"\nX-Content-Type-Options: nosniff","chunkId":"api/board#get-blobsha256"},{"kind":"code","literal":"url","chunkId":"api/board#get-blobsha256"},{"kind":"code","literal":"images","chunkId":"api/board#get-blobsha256"},{"kind":"code","literal":"Content-Type","chunkId":"api/board#get-blobsha256"},{"kind":"code","literal":"404 page not found","chunkId":"api/board#get-blobsha256"}]
sources: [{"chunkId":"api/board#get-blobsha256","url":"/docs/api/board#get-blobsha256","anchor":"get-blobsha256"}]
---

Serves the bytes of an uploaded board image by its SHA-256 hash, with long-lived immutable caching and a sandboxing security policy; unknown hashes get a plain-text 404.
