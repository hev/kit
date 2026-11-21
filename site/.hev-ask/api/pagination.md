---
id: "api#pagination"
title: "API overview"
heading: "Pagination"
group: "API"
order: 16
url: "/docs/api#pagination"
anchor: "pagination"
terms: ["pagination","list","routes","opaque","cursor","limit","default","returning","data","plus","next","null","last","page","valid","only","filters","produced","eyjvijo1mh0","take","return","shape","nextcursor","pass","back"]
hash: "79f978a84dc9e2f706f78782acbd276c352e376da736697ea453c4013bb45803"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"data\": [ ... ],\n  \"next_cursor\": \"eyJvIjo1MH0\"\n}","chunkId":"api#pagination"},{"kind":"code","literal":"cursor","chunkId":"api#pagination"},{"kind":"code","literal":"limit","chunkId":"api#pagination"},{"kind":"code","literal":"next_cursor","chunkId":"api#pagination"},{"kind":"code","literal":"null","chunkId":"api#pagination"}]
sources: [{"chunkId":"api#pagination","url":"/docs/api#pagination","anchor":"pagination"}]
---

List routes use an opaque cursor and a limit of 1 to 200 (default 50), returning data plus a next cursor that is null on the last page. A cursor is valid only with the filters that produced it.
