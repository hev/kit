---
id: "api/club-kit#delete-v1tracessessionsid"
title: "Club Kit API"
heading: "DELETE /v1/traces/sessions/\\{id\\}"
group: "API"
order: 38
url: "/docs/api/club-kit#delete-v1tracessessionsid"
anchor: "delete-v1tracessessionsid"
terms: ["delete","traces","sessions","deletes","session","blocks","chunks","only","author","captured","teammate","forbidden","outside","team","found","deleted","8f0c2e1a","error","notyoursession","notfound"]
hash: "2175ce01a474bdc39fed4bdc702064fd82c8f950d55932dc1f88493e957f3cfd"
mode: "source-primary"
facts: [{"kind":"code","literal":"{ \"deleted\": \"8f0c2e1a-...\" }","chunkId":"api/club-kit#delete-v1tracessessionsid"},{"kind":"code","literal":"403 {\"error\":\"not_your_session\"}","chunkId":"api/club-kit#delete-v1tracessessionsid"},{"kind":"code","literal":"404 {\"error\":\"not_found\"}","chunkId":"api/club-kit#delete-v1tracessessionsid"}]
sources: [{"chunkId":"api/club-kit#delete-v1tracessessionsid","url":"/docs/api/club-kit#delete-v1tracessessionsid","anchor":"delete-v1tracessessionsid"}]
---

Deletes one session with its blocks and chunks. A key can delete only sessions its own author captured; a teammate's session is forbidden and a session outside the key's team is not found.
