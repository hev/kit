---
id: "api/field-kit"
title: "Field Kit API"
heading: null
group: "API"
order: 49
url: "/docs/api/field-kit"
anchor: null
terms: ["hosted","field","early","access","deployed","signed","machines","obtained","login","bearer","token","every","call","errors","status","plus","short","code","covering","unauthenticated","keys","writing","another","author","session","billing","read","only","state","writes","refused","reads","continue","unsent","data","should","kept","resent","later","authorization"]
hash: "e9ae9c580f583f012436a6377ad956042708bf3a992405bd2faf347db123af8c"
mode: "source-primary"
facts: [{"kind":"code","literal":"Authorization: Bearer hk_...","chunkId":"api/field-kit"},{"kind":"code","literal":"{ \"error\": \"unauthenticated\" }","chunkId":"api/field-kit"},{"kind":"code","literal":"hk_","chunkId":"api/field-kit"},{"kind":"code","literal":"hev login","chunkId":"api/field-kit"},{"kind":"code","literal":"401 {\"error\":\"unauthenticated\"}","chunkId":"api/field-kit"},{"kind":"code","literal":"403 {\"error\":\"not_your_session\"}","chunkId":"api/field-kit"},{"kind":"code","literal":"402 {\"error\":\"billing_read_only\"}","chunkId":"api/field-kit"},{"kind":"code","literal":"hevd","chunkId":"api/field-kit"},{"kind":"value","literal":"SignupOpen.astro","chunkId":"api/field-kit"},{"kind":"value","literal":"SignupClosed.astro","chunkId":"api/field-kit"},{"kind":"value","literal":"hevkit.com","chunkId":"api/field-kit"}]
sources: [{"chunkId":"api/field-kit","url":"/docs/api/field-kit","anchor":null}]
---

The hosted Field Kit API, in early access and not yet deployed: signed-in machines use a kit key obtained at login as a bearer token on every call. Errors are a status plus a short code, covering unauthenticated keys, writing to another author's session, and a billing read-only state in which writes are refused but reads continue and unsent data should be kept and resent later.
