---
id: "install#verify"
title: "Install"
heading: "Verify"
group: "Start"
order: 255
url: "/docs/install#verify"
anchor: "verify"
terms: ["verify","daemon","last","units","indexed","error","trace","search","empty","results","machine","fine","errors","check","working","tell","many","docs","https","hevkit","install","pages","llms","traces","preflight","fail","confirm","works","whether","returned","sessions","result","show","reports","dispatch","needs","extra","config","file","sending"]
hash: "178ce143cc557b6fd29aff717fca54b4ce6d80cdd9c8c791d5219b945211ede3"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev s","chunkId":"install#verify"},{"kind":"code","literal":"Check that the hev daemon is working: run `hev s` and tell me its last run, how many units it indexed and the last error, if any.\nDocs: https://hevkit.com/docs/install#verify (all pages: https://hevkit.com/llms.txt)","chunkId":"install#verify"},{"kind":"code","literal":"hev traces search \"why did the preflight fail\"","chunkId":"install#verify"},{"kind":"code","literal":"Confirm trace search works: run `hev traces search \"why did the preflight fail\"` and tell me whether it returned sessions. An empty result on a new machine is fine; an error is not, so show me any error.\nDocs: https://hevkit.com/docs/install#verify (all pages: https://hevkit.com/llms.txt)","chunkId":"install#verify"}]
sources: [{"chunkId":"install#verify","url":"/docs/install#verify","anchor":"verify"}]
---

Verify the daemon with hev s (last run, units indexed, last error), then run a trace search; empty results on a new machine are fine but errors are not.
