---
id: "api/club-kit"
title: "Club Kit API"
heading: null
group: "API"
order: 36
url: "/docs/api/club-kit"
anchor: null
terms: ["hosted","club","early","access","deployed","signed","machines","obtained","login","bearer","token","every","call","errors","status","plus","short","code","covering","unauthenticated","keys","writing","another","author","session","billing","read","only","state","writes","refused","reads","continue","unsent","data","should","kept","resent","later","authorization"]
hash: "9e2d03ab74dae59c7345c28c19ae90639811225fc3e5dfec564fd1dd0667367a"
mode: "source-primary"
facts: [{"kind":"code","literal":"Authorization: Bearer hk_...","chunkId":"api/club-kit"},{"kind":"code","literal":"{ \"error\": \"unauthenticated\" }","chunkId":"api/club-kit"},{"kind":"code","literal":"hk_","chunkId":"api/club-kit"},{"kind":"code","literal":"hev login","chunkId":"api/club-kit"},{"kind":"code","literal":"401 {\"error\":\"unauthenticated\"}","chunkId":"api/club-kit"},{"kind":"code","literal":"403 {\"error\":\"not_your_session\"}","chunkId":"api/club-kit"},{"kind":"code","literal":"402 {\"error\":\"billing_read_only\"}","chunkId":"api/club-kit"},{"kind":"code","literal":"hevd","chunkId":"api/club-kit"},{"kind":"value","literal":"SignupOpen.astro","chunkId":"api/club-kit"},{"kind":"value","literal":"SignupClosed.astro","chunkId":"api/club-kit"},{"kind":"value","literal":"hevkit.com","chunkId":"api/club-kit"}]
sources: [{"chunkId":"api/club-kit","url":"/docs/api/club-kit","anchor":null}]
---

The hosted Club Kit API, in early access and not yet deployed: signed-in machines use a kit key obtained at login as a bearer token on every call. Errors are a status plus a short code, covering unauthenticated keys, writing to another author's session, and a billing read-only state in which writes are refused but reads continue and unsent data should be kept and resent later.
