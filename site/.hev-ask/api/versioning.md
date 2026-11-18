---
id: "api#versioning"
title: "API overview"
heading: "Versioning"
group: "API"
order: 12
url: "/docs/api#versioning"
anchor: "versioning"
terms: ["versioning","local","apis","version","prefix","change","between","releases","match","server","ignore","unknown","fields","field","carries","path","authenticated","calls","while","sign","routes","along","login","command","ship","changelog","notes","changes","script","against","prefer","ignoring","know","failing","puts","made","under"]
hash: "29e184cf1c998bbd41806d78a38535d99e891b7da333817471867387a588c6cd"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"api#versioning"},{"kind":"code","literal":"/v1/…","chunkId":"api#versioning"},{"kind":"code","literal":"/cli/","chunkId":"api#versioning"},{"kind":"code","literal":"hev login","chunkId":"api#versioning"}]
sources: [{"chunkId":"api#versioning","url":"/docs/api#versioning","anchor":"versioning"}]
---

The local APIs have no version prefix and can change between releases, so match the server's version and ignore unknown fields. The Field Kit API carries its version in the path for key-authenticated calls, while its sign-in routes change along with the login command.
