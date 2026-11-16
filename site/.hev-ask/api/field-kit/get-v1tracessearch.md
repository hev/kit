---
id: "api/field-kit#get-v1tracessearch"
title: "Field Kit API"
heading: "GET /v1/traces/search"
group: "API"
order: 52
url: "/docs/api/field-kit#get-v1tracessearch"
anchor: "get-v1tracessearch"
terms: ["traces","search","searches","team","phrase","mode","exact","words","order","hybrid","meaning","otherwise","matching","dashboard","endpoint","counts","toward","monthly","usage","upload","20race","authorization","bearer","query","same","parameters","shape"]
hash: "163e6b17915a1e175dcb7ba932e3808631c9d192c295636a31defb725d0339c9"
mode: "source-primary"
facts: [{"kind":"code","literal":"GET /v1/traces/search?q=upload%20race\u0026mode=hybrid\nAuthorization: Bearer hk_...","chunkId":"api/field-kit#get-v1tracessearch"},{"kind":"code","literal":"mode","chunkId":"api/field-kit#get-v1tracessearch"},{"kind":"code","literal":"phrase","chunkId":"api/field-kit#get-v1tracessearch"},{"kind":"code","literal":"hybrid","chunkId":"api/field-kit#get-v1tracessearch"},{"kind":"code","literal":"GET /api/search","chunkId":"api/field-kit#get-v1tracessearch"},{"kind":"code","literal":"GET /v1/usage","chunkId":"api/field-kit#get-v1tracessearch"}]
sources: [{"chunkId":"api/field-kit#get-v1tracessearch","url":"/docs/api/field-kit#get-v1tracessearch","anchor":"get-v1tracessearch"}]
---

Searches the team's traces in phrase mode (exact words in order) or hybrid mode (meaning and words), otherwise matching the dashboard's search endpoint. Each search counts toward the team's monthly search usage.
