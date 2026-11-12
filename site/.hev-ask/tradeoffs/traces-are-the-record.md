---
id: "tradeoffs#traces-are-the-record"
title: "Tradeoffs"
heading: "Traces are the record"
group: "Reference"
order: 296
url: "/docs/tradeoffs#traces-are-the-record"
anchor: "traces-are-the-record"
terms: ["traces","record","directory","holds","only","supervisor","knows","timing","exit","code","branch","commit","session","while","step","transcript","indexed","capture","daemon","searchable","order","dispatch","does","start","until","search","name","head","agent","lives","indexes","archive","tagged","read","cost","depends","running","host","configure","still"]
hash: "8b3d575276ed6e760b7ae1b8436c311b6e0044d969b7857e1e7f304994be1785"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev traces search --order NAME","chunkId":"tradeoffs#traces-are-the-record"}]
sources: [{"chunkId":"tradeoffs#traces-are-the-record","url":"/docs/tradeoffs#traces-are-the-record","anchor":"traces-are-the-record"}]
---

The run directory holds only what the supervisor knows (timing, exit code, branch, commit, PR and session ID), while the step-by-step record is the session transcript indexed by the capture daemon and searchable by order. Dispatch does not start capture, so if the daemon is off the transcript is not searchable until indexed.
