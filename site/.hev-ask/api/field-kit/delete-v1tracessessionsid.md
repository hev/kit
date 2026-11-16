---
id: "api/field-kit#delete-v1tracessessionsid"
title: "Field Kit API"
heading: "DELETE /v1/traces/sessions/\\{id\\}"
group: "API"
order: 51
url: "/docs/api/field-kit#delete-v1tracessessionsid"
anchor: "delete-v1tracessessionsid"
terms: ["delete","traces","sessions","deletes","session","blocks","chunks","only","author","captured","teammate","forbidden","outside","team","found","deleted","8f0c2e1a","error","notyoursession","notfound"]
hash: "88b3268803d5c0800a1032b82ca56576e1e28cba804cc44fd916f63b92dfa5ed"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"deleted\": \"8f0c2e1a-...\" }","chunkId":"api/field-kit#delete-v1tracessessionsid"},{"kind":"code","literal":"403 {\"error\":\"not_your_session\"}","chunkId":"api/field-kit#delete-v1tracessessionsid"},{"kind":"code","literal":"404 {\"error\":\"not_found\"}","chunkId":"api/field-kit#delete-v1tracessessionsid"}]
sources: [{"chunkId":"api/field-kit#delete-v1tracessessionsid","url":"/docs/api/field-kit#delete-v1tracessessionsid","anchor":"delete-v1tracessessionsid"}]
---

Deletes one session with its blocks and chunks. A key can delete only sessions its own author captured; a teammate's session is forbidden and a session outside the key's team is not found.
