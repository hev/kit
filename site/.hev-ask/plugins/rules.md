---
id: "plugins#rules"
title: "Plugins"
heading: "Rules"
group: "Operate"
order: 230
url: "/docs/plugins#rules"
anchor: "rules"
terms: ["rules","built","commands","aliases","always","plugins","only","first","argument","matched","starting","dash","never","plugin","directory","path","wins","duplicate","help","lists","discovered","section","browse","completion","hello","flag","their","file","named","neither","runs","starts","directories","same","command","list","find","name","without","duplicates"]
hash: "5edaabcd5bd775f1e184d748fa481b6b19b829996e97837ff53c8d8e2d78e610"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev-up","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-browse","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-help","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-completion","chunkId":"plugins#rules"},{"kind":"code","literal":"hev hello --flag","chunkId":"plugins#rules"},{"kind":"code","literal":"hev-hello","chunkId":"plugins#rules"},{"kind":"code","literal":"--flag","chunkId":"plugins#rules"},{"kind":"code","literal":"hev help","chunkId":"plugins#rules"},{"kind":"code","literal":"hev --help","chunkId":"plugins#rules"},{"kind":"code","literal":"Plugins","chunkId":"plugins#rules"}]
sources: [{"chunkId":"plugins#rules","url":"/docs/plugins#rules","anchor":"rules"}]
---

Built-in commands and aliases always win over plugins. Only the first argument is matched, and one starting with a dash is never a plugin. The first directory on PATH wins for duplicate plugins. Help lists discovered plugins in a Plugins section.
