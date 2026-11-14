---
id: "api#versioning"
title: "API overview"
heading: "Versioning"
group: "API"
order: 12
url: "/docs/api#versioning"
anchor: "versioning"
terms: ["versioning","local","apis","version","prefix","change","between","releases","match","server","ignore","unknown","fields","field","versions","authenticated","routes","path","while","sign","along","login","command","ship","changelog","notes","changes","script","against","prefer","ignoring","know","failing","puts","calls","made","under"]
hash: "29e184cf1c998bbd41806d78a38535d99e891b7da333817471867387a588c6cd"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"api#versioning"},{"kind":"code","literal":"/v1/…","chunkId":"api#versioning"},{"kind":"code","literal":"/cli/","chunkId":"api#versioning"},{"kind":"code","literal":"hev login","chunkId":"api#versioning"}]
sources: [{"chunkId":"api#versioning","url":"/docs/api#versioning","anchor":"versioning"}]
---

Local APIs have no version prefix and can change between kit releases, so match the server's hev version and ignore unknown fields. The Field Kit API versions its key-authenticated routes in the path, while the sign-in routes change along with the login command.
