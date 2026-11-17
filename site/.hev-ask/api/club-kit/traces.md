---
id: "api/club-kit#traces"
title: "Club Kit API"
heading: "Traces"
group: "API"
order: 50
url: "/docs/api/club-kit#traces"
anchor: "traces"
terms: ["traces","signed","machines","send","club","read","back","same","daemon","writes","trace","search","commands","local","dashboard","server","sets","authorship","host","team","ignores","those","fields","request","bodies","only","write","person","machine","issued","hevd","find","serve","author","name","session","does","writing","reading","call"]
hash: "e66fcf77de29b791d5f233ee9da6f2ab69855d0315720a714c0f52081b35ff99"
mode: "source-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"hev traces search","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"hev find","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"hev serve","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"author_id","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"author_name","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"host","chunkId":"api/club-kit#traces"},{"kind":"code","literal":"session_id","chunkId":"api/club-kit#traces"}]
sources: [{"chunkId":"api/club-kit#traces","url":"/docs/api/club-kit#traces","anchor":"traces"}]
---

Signed-in machines send traces to the Club Kit API and read them back with the same key: the daemon writes, and the trace search commands and the local dashboard read. The server sets authorship, host and team from the key and ignores those fields in request bodies, so a key can only write as the person and machine it was issued to.
