---
id: "tradeoffs#traces-are-the-record"
title: "Tradeoffs"
heading: "Traces are the record"
group: "Reference"
order: 352
url: "/docs/tradeoffs#traces-are-the-record"
anchor: "traces-are-the-record"
terms: ["traces","record","directory","stores","supervisor","knows","start","exit","code","branch","head","session","while","step","detail","lives","transcript","indexed","capture","daemon","searchable","order","cost","dependence","running","host","dispatch","does","configure","search","name","holds","commit","agent","indexes","archive","tagged","read","depends","still"]
hash: "8b3d575276ed6e760b7ae1b8436c311b6e0044d969b7857e1e7f304994be1785"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev traces search --order NAME","chunkId":"tradeoffs#traces-are-the-record"}]
sources: [{"chunkId":"tradeoffs#traces-are-the-record","url":"/docs/tradeoffs#traces-are-the-record","anchor":"traces-are-the-record"}]
---

The run directory stores what the supervisor knows (start, end, exit code, branch, head, PR, session), while step-by-step detail lives in the transcript indexed by the capture daemon and searchable by order. The cost is dependence on capture running on the host; dispatch does not configure it.
