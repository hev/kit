---
id: "order-reference#frontmatter-syntax"
title: "Order reference"
heading: "Frontmatter syntax"
group: "Reference"
order: 327
url: "/docs/order-reference#frontmatter-syntax"
anchor: "frontmatter-syntax"
terms: ["frontmatter","syntax","strict","yaml","subset","first","line","exactly","ends","next","value","comments","blank","lines","skipped","case","sensitive","known","keys","only","repeats","nesting","lists","unquoted","values","comment","marker","unknown","duplicate","item","expected","flat","port","preview","empty","field","parser","accepts","small","file"]
hash: "81fa4043cd9a9fca42d9580642de7bd059b9bc161b65ae4b38c1acd953e9bf45"
mode: "source-primary"
facts: [{"kind":"code","literal":"---","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"unknown key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"duplicate key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"- item","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"expected flat key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"{port}","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"preview","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"empty field","chunkId":"order-reference#frontmatter-syntax"}]
sources: [{"chunkId":"order-reference#frontmatter-syntax","url":"/docs/order-reference#frontmatter-syntax","anchor":"frontmatter-syntax"}]
---

Frontmatter is a strict non-YAML subset: first line exactly ---, ends at next ---, one key: value per line, comments and blank lines skipped, case-sensitive known keys only, no repeats, no nesting or lists, unquoted values end at a comment marker.
