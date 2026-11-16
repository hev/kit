---
id: "order-reference#frontmatter-syntax"
title: "Order reference"
heading: "Frontmatter syntax"
group: "Reference"
order: 264
url: "/docs/order-reference#frontmatter-syntax"
anchor: "frontmatter-syntax"
terms: ["frontmatter","syntax","order","yaml","small","strict","subset","flat","value","lines","between","delimiter","case","sensitive","known","keys","only","most","once","nesting","lists","unquoted","values","comment","marker","cannot","contain","brackets","braces","tabs","apart","preview","port","placeholder","quoted","allow","those","escapes","empty","errors"]
hash: "81fa4043cd9a9fca42d9580642de7bd059b9bc161b65ae4b38c1acd953e9bf45"
mode: "source-primary"
facts: [{"kind":"code","literal":"---","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"unknown key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"duplicate key","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"- item","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"expected flat key: value","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"{port}","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"preview","chunkId":"order-reference#frontmatter-syntax"},{"kind":"code","literal":"empty field","chunkId":"order-reference#frontmatter-syntax"}]
sources: [{"chunkId":"order-reference#frontmatter-syntax","url":"/docs/order-reference#frontmatter-syntax","anchor":"frontmatter-syntax"}]
---

Order frontmatter is not YAML but a small strict subset: flat key-value lines between two delimiter lines, case-sensitive known keys only, each at most once, no nesting or lists. Unquoted values end at a comment marker and cannot contain brackets, braces or tabs (apart from the preview port placeholder); quoted values allow those, have no escapes, and empty values are errors.
