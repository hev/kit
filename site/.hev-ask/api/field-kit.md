---
id: "api/field-kit"
title: "Field Kit API"
heading: null
group: "API"
order: 49
url: "/docs/api/field-kit"
anchor: null
terms: ["hosted","field","early","access","deployed","subject","change","lets","signed","machines","send","search","traces","team","board","usage","calls","bearer","obtained","signing","never","created","hand","errors","return","short","code","expired","keys","authorization","error","unauthenticated","login","hevkit","machine","dashboard","link","routes","open","page"]
hash: "6533d298de07757278d8709785e086eed85973897bfb5c185efcbca3389ae042"
mode: "source-primary"
facts: [{"kind":"code","literal":"Authorization: Bearer hk_...","chunkId":"api/field-kit"},{"kind":"code","literal":"{ \"error\": \"unauthenticated\" }","chunkId":"api/field-kit"},{"kind":"code","literal":"hk_","chunkId":"api/field-kit"},{"kind":"code","literal":"hev login","chunkId":"api/field-kit"},{"kind":"code","literal":"401 {\"error\":\"unauthenticated\"}","chunkId":"api/field-kit"},{"kind":"value","literal":"hevkit.com","chunkId":"api/field-kit"}]
sources: [{"chunkId":"api/field-kit","url":"/docs/api/field-kit","anchor":null}]
---

The hosted Field Kit API (early access, not yet deployed and subject to change) lets signed-in machines send and search traces, use the team board and see usage. Calls use a bearer kit key obtained by signing in, never created by hand; errors return a short code, and bad or expired keys get 401.
