---
id: "api/field-kit#traces"
title: "Field Kit API"
heading: "Traces"
group: "API"
order: 63
url: "/docs/api/field-kit#traces"
anchor: "traces"
terms: ["traces","signed","machines","send","field","read","back","same","daemon","writes","trace","search","commands","local","dashboard","server","sets","authorship","host","team","ignores","those","fields","request","bodies","only","write","person","machine","issued","hevd","find","serve","author","name","session","does","writing","reading","call"]
hash: "bc2387f4200863da1703912a3a1d697c8eb0d52058b68f7645ef180adf237314"
mode: "source-primary"
facts: [{"kind":"code","literal":"hevd","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"hev traces search","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"hev find","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"hev serve","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"author_id","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"author_name","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"host","chunkId":"api/field-kit#traces"},{"kind":"code","literal":"session_id","chunkId":"api/field-kit#traces"}]
sources: [{"chunkId":"api/field-kit#traces","url":"/docs/api/field-kit#traces","anchor":"traces"}]
---

Signed-in machines send traces to the Field Kit API and read them back with the same key: the daemon writes, and the trace search commands and the local dashboard read. The server sets authorship, host and team from the key and ignores those fields in request bodies, so a key can only write as the person and machine it was issued to.
