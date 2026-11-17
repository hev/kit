---
id: "api/club-kit#get-v1tracessearch"
title: "Club Kit API"
heading: "GET /v1/traces/search"
group: "API"
order: 39
url: "/docs/api/club-kit#get-v1tracessearch"
anchor: "get-v1tracessearch"
terms: ["traces","search","searches","team","phrase","mode","exact","words","order","hybrid","meaning","otherwise","matching","dashboard","endpoint","counts","toward","monthly","usage","upload","20race","authorization","bearer","query","same","parameters","shape"]
hash: "ac2a29384f399c7e8fa9d0df44c00413d2dcc5f3bfbe1776e248bddaa95a8b62"
mode: "source-primary"
facts: [{"kind":"code","literal":"GET /v1/traces/search?q=upload%20race\u0026mode=hybrid\nAuthorization: Bearer hk_...","chunkId":"api/club-kit#get-v1tracessearch"},{"kind":"code","literal":"mode","chunkId":"api/club-kit#get-v1tracessearch"},{"kind":"code","literal":"phrase","chunkId":"api/club-kit#get-v1tracessearch"},{"kind":"code","literal":"hybrid","chunkId":"api/club-kit#get-v1tracessearch"},{"kind":"code","literal":"GET /api/search","chunkId":"api/club-kit#get-v1tracessearch"},{"kind":"code","literal":"GET /v1/usage","chunkId":"api/club-kit#get-v1tracessearch"}]
sources: [{"chunkId":"api/club-kit#get-v1tracessearch","url":"/docs/api/club-kit#get-v1tracessearch","anchor":"get-v1tracessearch"}]
---

Searches the team's traces in phrase mode (exact words in order) or hybrid mode (meaning and words), otherwise matching the dashboard's search endpoint. Each search counts toward the team's monthly search usage.
