---
id: "order-reference#frontmatter-syntax"
title: "Order reference"
heading: "Frontmatter syntax"
group: "Reference"
order: 204
url: "/docs/order-reference#frontmatter-syntax"
anchor: "frontmatter-syntax"
terms: ["frontmatter","syntax","strict","yaml","subset","dashed","delimiter","lines","flat","value","line","skipped","blanks","comments","case","sensitive","known","keys","only","repeats","nesting","lists","unquoted","values","comment","cannot","contain","brackets","braces","tabs","except","port","placeholder","quotes","allow","those","characters","escapes","empty","fail"]
hash: "81fa4043cd9a9fca42d9580642de7bd059b9bc161b65ae4b38c1acd953e9bf45"
mode: "source-primary"
facts: [{"kind":"code","literal":"---","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"unknown key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"duplicate key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"- item","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"expected flat key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"{port}","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"preview","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"empty field","chunkId":"order-reference#frontmatter-syntax"}]
sources: [{"chunkId":"order-reference#frontmatter-syntax","url":"/docs/order-reference#frontmatter-syntax","anchor":"frontmatter-syntax"}]
---

Frontmatter is a strict non-YAML subset: dashed delimiter lines, one flat key and value per line, skipped blanks and comments, case-sensitive known keys only, no repeats, nesting or lists. Unquoted values end at a comment and cannot contain brackets, braces or tabs except the port placeholder, quotes allow those characters with no escapes, and empty values fail.
