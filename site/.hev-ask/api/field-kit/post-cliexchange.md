---
id: "api/field-kit#post-cliexchange"
title: "Field Kit API"
heading: "POST /cli/exchange"
group: "API"
order: 58
url: "/docs/api/field-kit#post-cliexchange"
anchor: "post-cliexchange"
terms: ["post","exchange","trades","time","sign","code","pkce","verifier","returning","base","shown","only","once","stored","hash","service","team","author","member","live","machine","signing","again","same","hostname","revokes","previous","keys","expire","after","days","removing","their","lists","errors","malformed","bodies","codes","removed","members"]
hash: "42e0a936b20b911a0e4371135266b1d514fad12f47d2152ddb8c1005262e6bbd"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"api\": \"https://...\",\n  \"key\": \"hk_...\",\n  \"key_id\": \"mk_...\",\n  \"team\": { \"id\": \"...\", \"slug\": \"acme\" },\n  \"author\": { \"id\": \"...\", \"name\": \"Ada Lovelace\" }\n}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"Authorization","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"code","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"verifier","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"challenge","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"hostname","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"api","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"key","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"hev login","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"key_id","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"team","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"author","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_exchange\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_grant\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"code","literal":"403 {\"error\":\"membership_removed\"}","chunkId":"api/field-kit#post-cliexchange"},{"kind":"value","literal":"SHA-256","chunkId":"api/field-kit#post-cliexchange"}]
sources: [{"chunkId":"api/field-kit#post-cliexchange","url":"/docs/api/field-kit#post-cliexchange","anchor":"post-cliexchange"}]
---

Trades the one-time sign-in code and PKCE verifier for a kit key, returning the API base URL, the key (shown only once, stored as a hash by the service), a key ID, and the team and author. Each member has one live key per machine, so signing in again on the same hostname revokes the previous one; keys expire after 90 days and removing a member revokes their keys. Lists the errors for malformed bodies, bad or used codes, and removed members.
