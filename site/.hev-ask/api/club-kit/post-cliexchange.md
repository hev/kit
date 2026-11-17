---
id: "api/club-kit#post-cliexchange"
title: "Club Kit API"
heading: "POST /cli/exchange"
group: "API"
order: 45
url: "/docs/api/club-kit#post-cliexchange"
anchor: "post-cliexchange"
terms: ["post","exchange","trades","time","sign","code","pkce","verifier","returning","base","shown","only","once","stored","hash","service","team","author","member","live","machine","signing","again","same","hostname","revokes","previous","keys","expire","after","days","removing","their","lists","errors","malformed","bodies","codes","removed","members"]
hash: "012d9d91340def4f9b2b7f56395e03ec2ce6defa370ad370ea541dbf95d00d84"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"api\": \"https://...\",\n  \"key\": \"hk_...\",\n  \"key_id\": \"mk_...\",\n  \"team\": { \"id\": \"...\", \"slug\": \"acme\" },\n  \"author\": { \"id\": \"...\", \"name\": \"Ada Lovelace\" }\n}","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"Authorization","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"code","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"verifier","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"challenge","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"hostname","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"api","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"key","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"hev login","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"key_id","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"team","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"author","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_exchange\"}","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"400 {\"error\":\"invalid_grant\"}","chunkId":"api/club-kit#post-cliexchange"},{"kind":"code","literal":"403 {\"error\":\"membership_removed\"}","chunkId":"api/club-kit#post-cliexchange"},{"kind":"value","literal":"SHA-256","chunkId":"api/club-kit#post-cliexchange"}]
sources: [{"chunkId":"api/club-kit#post-cliexchange","url":"/docs/api/club-kit#post-cliexchange","anchor":"post-cliexchange"}]
---

Trades the one-time sign-in code and PKCE verifier for a kit key, returning the API base URL, the key (shown only once, stored as a hash by the service), a key ID, and the team and author. Each member has one live key per machine, so signing in again on the same hostname revokes the previous one; keys expire after 90 days and removing a member revokes their keys. Lists the errors for malformed bodies, bad or used codes, and removed members.
