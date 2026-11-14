---
id: "api/field-kit#post-cliexchange"
title: "Field Kit API"
heading: "POST /cli/exchange"
group: "API"
order: 58
url: "/docs/api/field-kit#post-cliexchange"
anchor: "post-cliexchange"
terms: ["post","exchange","trades","single","sign","code","pkce","verifier","returned","once","stored","only","hash","service","saves","privately","never","prints","live","member","machine","keys","expire","after","days","removal","team","revokes","invalid","mismatched","codes","removed","memberships","return","errors","https","slug","acme","author","name"]
hash: "42e0a936b20b911a0e4371135266b1d514fad12f47d2152ddb8c1005262e6bbd"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"api\": \"https://...\",\n  \"key\": \"hk_...\",\n  \"key_id\": \"mk_...\",\n  \"team\": { \"id\": \"...\", \"slug\": \"acme\" },\n  \"author\": { \"id\": \"...\", \"name\": \"Ada Lovelace\" }\n}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"Authorization","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"code","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"verifier","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"challenge","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"hostname","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"api","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"key","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"hev login","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"key_id","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"team","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"author","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_exchange\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_grant\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"403 {\"error\":\"membership_removed\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"value","literal":"SHA-256","chunkId":"api/field-kit#post-cliexchange"}]
sources: [{"chunkId":"api/field-kit#post-cliexchange","url":"/docs/api/field-kit#post-cliexchange","anchor":"post-cliexchange"}]
---

Trades the single-use sign-in code and PKCE verifier for a kit key, returned once and stored only as a hash on the service; the CLI saves it privately and never prints it. One live key per member per machine, keys expire after 90 days, and removal from the team revokes them. Invalid or mismatched codes and removed memberships return errors.
