---
id: "api/peers#register-and-heartbeat"
title: "Peers API"
heading: "Register and heartbeat"
group: "API"
order: 59
url: "/docs/api/peers#register-and-heartbeat"
anchor: "register-and-heartbeat"
terms: ["register","heartbeat","host","registers","session","starts","least","every","seconds","reporting","agent","state","role","similar","presence","fields","curl","https","hevmind","peers","mini","authorization","bearer","content","type","application","json","claude","working","recipient","incarnation","process","48213","1763575622","users","workspace","branch","docs","overhaul","20261119"]
hash: "e7c988dcd369ed990d43bfa1dff10aa73ed6fb53646eade4d631ca312c7602b3"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -X PUT https://kit.hevmind.com/v1/peers/mini:foh-kit \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\n    \"agent\": \"claude\",\n    \"state\": \"working\",\n    \"role\": \"foh\",\n    \"recipient_incarnation\": \"process:mini:48213:1763575622\",\n    \"cwd\": \"/Users/me/workspace/kit\",\n    \"branch\": \"d/kit-docs-overhaul-20261119\",\n    \"inbox\": true\n  }'","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"PUT /v1/peers/{id}","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"agent","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"state","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"recipient_incarnation","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"201","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"200","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"Peer","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"last_seen","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"screen","chunkId":"api/peers#register-and-heartbeat"},{"kind":"code","literal":"process:HOST:PID:BIRTH","chunkId":"api/peers#register-and-heartbeat"}]
sources: [{"chunkId":"api/peers#register-and-heartbeat","url":"/docs/api/peers#register-and-heartbeat","anchor":"register-and-heartbeat"}]
---

A host registers each session with a PUT when it starts and at least every 60 seconds, reporting agent, state, role and similar presence fields.
