---
id: "api/orders#amend-resolve-rework"
title: "Orders API"
heading: "Amend, resolve, rework"
group: "API"
order: 44
url: "/docs/api/orders#amend-resolve-rework"
anchor: "amend-resolve-rework"
terms: ["amend","resolve","rework","patch","order","takes","exactly","three","fields","replacement","text","answer","pending","decision","state","transition","such","note","nname","docs","typo","20261120","nrepo","nfix","both","typos","acceptance","postgres","also","update","changelog","orders","name","repo","needs","retry","stop","amends","cannot","change"]
hash: "139e46bd6a4361d3100928635938dc85103d98e6a1c37d05ac0384a85df95bb5"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"order\": \"---\\nname: kit-docs-typo-20261120\\nrepo: kit\\n---\\nFix both typos…\\n\\n## Acceptance\\n- …\" }","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"{ \"resolve\": { \"key\": \"db\", \"answer\": \"Use Postgres.\" } }","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"{ \"transition\": { \"to\": \"rework\", \"note\": \"Also update the changelog.\" } }","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"PATCH /v1/orders/{id}","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"order","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"name","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"repo","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"resolve","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"needs-decision","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"key","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"say --resolve KEY","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"transition","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"rework","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"note","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"retry","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"stop","chunkId":"api/orders#amend-resolve-rework"},{"kind":"code","literal":"409","chunkId":"api/orders#amend-resolve-rework"}]
sources: [{"chunkId":"api/orders#amend-resolve-rework","url":"/docs/api/orders#amend-resolve-rework","anchor":"amend-resolve-rework"}]
---

A patch on an order takes exactly one of three fields: a replacement order text (amend), an answer to a pending decision (resolve), or a state transition such as rework with a note.
