---
id: "order-reference#frontmatter-syntax"
title: "Order reference"
heading: "Frontmatter syntax"
group: "Reference"
order: 210
url: "/docs/order-reference#frontmatter-syntax"
anchor: "frontmatter-syntax"
terms: ["frontmatter","syntax","order","strict","flat","subset","yaml","starts","ends","line","three","dashes","case","sensitive","known","keys","nesting","lists","repeated","unquoted","values","comment","character","cannot","contain","brackets","braces","tabs","quoted","escapes","windows","endings","accepted","value","unknown","duplicate","item","expected","port","preview"]
hash: "81fa4043cd9a9fca42d9580642de7bd059b9bc161b65ae4b38c1acd953e9bf45"
mode: "source-primary"
facts: [{"kind":"code","literal":"---","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"unknown key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"duplicate key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"- item","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"expected flat key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"{port}","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"preview","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"empty field","chunkId":"order-reference#frontmatter-syntax"}]
sources: [{"chunkId":"order-reference#frontmatter-syntax","url":"/docs/order-reference#frontmatter-syntax","anchor":"frontmatter-syntax"}]
---

Order frontmatter is a strict flat subset, not YAML: starts and ends with a line of three dashes, one key per line, case-sensitive known keys, no nesting or lists, no repeated keys. Unquoted values end at a comment character and cannot contain brackets, braces or tabs; quoted values have no escapes. Windows line endings are accepted.
