---
id: "api#versioning"
title: "API overview"
heading: "Versioning"
group: "API"
order: 12
url: "/docs/api#versioning"
anchor: "versioning"
terms: ["versioning","local","apis","version","prefix","change","between","releases","match","server","ignore","unknown","fields","club","carries","path","authenticated","calls","while","sign","routes","along","login","command","ship","changelog","notes","changes","script","against","prefer","ignoring","know","failing","puts","made","under"]
hash: "6988e24a07e5012cb181e9720133437d037d78ffcb28eaad75e1b731e86deca9"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev","chunkId":"api#versioning"},{"kind":"code","literal":"/v1/…","chunkId":"api#versioning"},{"kind":"code","literal":"/cli/","chunkId":"api#versioning"},{"kind":"code","literal":"hev login","chunkId":"api#versioning"}]
sources: [{"chunkId":"api#versioning","url":"/docs/api#versioning","anchor":"versioning"}]
---

The local APIs have no version prefix and can change between releases, so match the server's version and ignore unknown fields. The Club Kit API carries its version in the path for key-authenticated calls, while its sign-in routes change along with the login command.
