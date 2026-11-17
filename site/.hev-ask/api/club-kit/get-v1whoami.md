---
id: "api/club-kit#get-v1whoami"
title: "Club Kit API"
heading: "GET /v1/whoami"
group: "API"
order: 44
url: "/docs/api/club-kit#get-v1whoami"
anchor: "get-v1whoami"
terms: ["whoami","says","belongs","team","author","machine","hostname","login","status","command","uses","simplest","check","still","works","slug","acme","name","lovelace","build","calls","also","keyid"]
hash: "c41b9fda64145e6116b96ab8139b188b83d0ee15f8bc960180f67a1201e59cdc"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"key_id\": \"mk_...\",\n  \"team\": { \"id\": \"...\", \"slug\": \"acme\" },\n  \"author\": { \"id\": \"...\", \"name\": \"Ada Lovelace\" },\n  \"hostname\": \"build-01\"\n}","chunkId":"api/club-kit#get-v1whoami"},{"kind":"code","literal":"hev login status","chunkId":"api/club-kit#get-v1whoami"}]
sources: [{"chunkId":"api/club-kit#get-v1whoami","url":"/docs/api/club-kit#get-v1whoami","anchor":"get-v1whoami"}]
---

Says who a key belongs to: its key ID, team, author and machine hostname. The login status command uses it, and it is the simplest check that a key still works.
