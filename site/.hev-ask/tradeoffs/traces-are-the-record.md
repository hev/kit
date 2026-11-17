---
id: "tradeoffs#traces-are-the-record"
title: "Tradeoffs"
heading: "Traces are the record"
group: "Reference"
order: 391
url: "/docs/tradeoffs#traces-are-the-record"
anchor: "traces-are-the-record"
terms: ["traces","record","directory","records","only","supervisor","knows","start","exit","code","branch","commit","session","while","agent","step","actions","live","transcript","indexed","archive","tagged","order","name","cost","dependency","capture","running","host","dispatch","does","configure","daemon","still","completes","searchable","until","searching","needs","version"]
hash: "8b3d575276ed6e760b7ae1b8436c311b6e0044d969b7857e1e7f304994be1785"
mode: "source-primary"
facts: [{"kind":"code","literal":"hev traces search --order NAME","chunkId":"tradeoffs#traces-are-the-record"}]
sources: [{"chunkId":"tradeoffs#traces-are-the-record","url":"/docs/tradeoffs#traces-are-the-record","anchor":"traces-are-the-record"}]
---

The run directory records only what the supervisor knows (start, end, exit code, branch, commit, PR and session ID), while the agent's step-by-step actions live in its session transcript, indexed into the archive and tagged with the order name. The cost is a dependency on capture running on the host: dispatch does not start or configure it, so with the daemon off the run still completes but its transcript is not searchable until indexed, and searching by order needs a version that adds the order tag.
