---
id: "admission#what-a-refusal-looks-like"
title: "Admission"
heading: "What a refusal looks like"
group: "Operate"
order: 4
url: "/docs/admission#what-a-refusal-looks-like"
anchor: "what-a-refusal-looks-like"
terms: ["refusal","looks","like","refused","order","exits","code","error","saying","repo","admitted","host","also","records","failed","listed","inspected","find","origin","would","need","dispatch","acme","buildbox","show","rate","limit","headers","result","json","clean","name","command","message","uses","values","keeps","record","example","give"]
hash: "d606c216db2dc28419900c8605109ab04675338b01a7304fddefd7ef3c687722"
mode: "agent-primary"
facts: [{"kind":"code","literal":"$ hev dispatch order.md\nError: repo \"acme/api\" is not admitted on host \"buildbox\"","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"hev dispatch ls --all\nhev dispatch show api-rate-limit-headers","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"repo","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"host","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"ls --all","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"failed","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"ERROR","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"show","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"result.json","chunkId":"admission#what-a-refusal-looks-like"},{"kind":"code","literal":"hev dispatch clean NAME","chunkId":"admission#what-a-refusal-looks-like"}]
sources: [{"chunkId":"admission#what-a-refusal-looks-like","url":"/docs/admission#what-a-refusal-looks-like","anchor":"what-a-refusal-looks-like"}]
---

A refused order exits with code 1 and an error saying the repo is not admitted on that host. The host also records the refusal as a failed run, which can be listed and inspected to find the origin URL the host would need.
