---
id: "api/field-kit#get-v1whoami"
title: "Field Kit API"
heading: "GET /v1/whoami"
group: "API"
order: 57
url: "/docs/api/field-kit#get-v1whoami"
anchor: "get-v1whoami"
terms: ["whoami","says","team","author","hostname","belongs","login","status","check","simplest","confirm","still","works","slug","acme","name","lovelace","build","calls","also","keyid"]
hash: "54b3ddcff019369bec505a630cdf3ae311cc1504f8f14080c5ef73ef7508d31b"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"key_id\": \"mk_...\",\n  \"team\": { \"id\": \"...\", \"slug\": \"acme\" },\n  \"author\": { \"id\": \"...\", \"name\": \"Ada Lovelace\" },\n  \"hostname\": \"build-01\"\n}","chunkId":"api/field-kit#get-v1whoami"},{"kind":"code","literal":"hev login status","chunkId":"api/field-kit#get-v1whoami"}]
sources: [{"chunkId":"api/field-kit#get-v1whoami","url":"/docs/api/field-kit#get-v1whoami","anchor":"get-v1whoami"}]
---

Says which key ID, team, author and hostname a kit key belongs to; used by the login status check and the simplest way to confirm a key still works.
