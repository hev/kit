---
id: "api/peers#receipts"
title: "Peers API"
heading: "Receipts"
group: "API"
order: 58
url: "/docs/api/peers#receipts"
anchor: "receipts"
terms: ["receipts","message","moves","queued","consumed","shown","agent","acknowledged","acted","dropped","recipient","posts","sender","follows","transport","state","timestamps","curl","https","hevmind","peers","mini","authorization","bearer","content","type","application","json","kind","rebase","warning","incarnation","process","48213","1763575622","recorded","rejected","error","yaml","ends"]
hash: "9fca4e0da75a33705be01df2c66e1a9f08b2fa39a4add2ef48d7bbaa000dfd99"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/peers/mini:foh-kit/receipts \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"kind\": \"acknowledged\", \"ids\": [\"rebase-warning-1\"], \"recipient_incarnation\": \"process:mini:48213:1763575622\"}'","chunkId":"api/peers#receipts"},{"kind":"code","literal":"{ \"kind\": \"acknowledged\", \"recorded\": [\"rebase-warning-1\"], \"rejected\": [] }","chunkId":"api/peers#receipts"},{"kind":"code","literal":"queued","chunkId":"api/peers#receipts"},{"kind":"code","literal":"consumed","chunkId":"api/peers#receipts"},{"kind":"code","literal":"acknowledged","chunkId":"api/peers#receipts"},{"kind":"code","literal":"dropped","chunkId":"api/peers#receipts"},{"kind":"code","literal":"transport_state","chunkId":"api/peers#receipts"},{"kind":"code","literal":"consumed_at","chunkId":"api/peers#receipts"},{"kind":"code","literal":"acknowledged_at","chunkId":"api/peers#receipts"},{"kind":"code","literal":"kind","chunkId":"api/peers#receipts"},{"kind":"code","literal":"ids","chunkId":"api/peers#receipts"},{"kind":"code","literal":"rejected","chunkId":"api/peers#receipts"},{"kind":"code","literal":"error","chunkId":"api/peers#receipts"},{"kind":"value","literal":"kit-v1.yaml","chunkId":"api/peers#receipts"}]
sources: [{"chunkId":"api/peers#receipts","url":"/docs/api/peers#receipts","anchor":"receipts"}]
---

A message moves queued, consumed (shown to the agent), acknowledged (acted on) or dropped. The recipient posts receipts and the sender follows transport state and timestamps.
