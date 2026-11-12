---
id: "plugins#rules"
title: "Plugins"
heading: "Rules"
group: "Operate"
order: 224
url: "/docs/plugins#rules"
anchor: "rules"
terms: ["rules","built","commands","aliases","always","take","precedence","plugins","only","first","argument","matched","never","starting","dash","path","directories","contain","same","plugin","wins","help","output","lists","discovered","section","browse","completion","hello","flag","their","file","named","neither","runs","starts","command","list","find","name"]
hash: "5edaabcd5bd775f1e184d748fa481b6b19b829996e97837ff53c8d8e2d78e610"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-up","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-browse","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-help","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-completion","chunkId":"plugins#rules"},{"kind":"code","literal":"hev hello --flag","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-hello","chunkId":"plugins#rules"},{"kind":"code","literal":"--flag","chunkId":"plugins#rules"},{"kind":"code","literal":"hev help","chunkId":"plugins#rules"},{"kind":"code","literal":"hev --help","chunkId":"plugins#rules"},{"kind":"code","literal":"Plugins","chunkId":"plugins#rules"}]
sources: [{"chunkId":"plugins#rules","url":"/docs/plugins#rules","anchor":"rules"}]
---

Built-in commands and aliases always take precedence over plugins, and only the first argument is matched, never one starting with a dash. If two PATH directories contain the same plugin the first wins, and help output lists discovered plugins in a Plugins section.
