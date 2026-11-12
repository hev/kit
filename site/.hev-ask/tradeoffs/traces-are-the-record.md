---
id: "tradeoffs#traces-are-the-record"
title: "Tradeoffs"
heading: "Traces are the record"
group: "Reference"
order: 298
url: "/docs/tradeoffs#traces-are-the-record"
anchor: "traces-are-the-record"
terms: ["traces","record","directory","holds","supervisor","knows","while","step","activity","lives","session","transcript","capture","daemon","indexes","tags","order","name","cost","depends","running","host","still","finishes","searchable","until","indexed","search","start","exit","code","branch","head","commit","agent","archive","tagged","read","dispatch","does"]
hash: "8b3d575276ed6e760b7ae1b8436c311b6e0044d969b7857e1e7f304994be1785"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev traces search --order NAME","chunkId":"tradeoffs#traces-are-the-record"}]
sources: [{"chunkId":"tradeoffs#traces-are-the-record","url":"/docs/tradeoffs#traces-are-the-record","anchor":"traces-are-the-record"}]
---

The run directory holds what the supervisor knows, while step-by-step activity lives in the session transcript that the capture daemon indexes and tags with the order name. The cost is that this depends on capture running on the host: if it is off, the run still finishes but its transcript is not searchable until indexed.
