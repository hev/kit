---
id: "dispatch#rework"
title: "Dispatch"
heading: "Rework"
group: "Use"
order: 98
url: "/docs/dispatch#rework"
anchor: "rework"
terms: ["rework","sends","stopped","back","another","attempt","note","order","ends","section","still","running","before","stopping","dispatch","rate","limit","headers","test","asserts","wrong","header","name","stop","result","json","report","status","inbox","send","replace","place","text","confirm","show","holding","again","docs","https","hevkit"]
hash: "a0372627eec563717701de8431039db07e737e8885a1dc74ffe1ee892a51611e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rework api-rate-limit-headers 'The 429 test asserts on the wrong header name'","chunkId":"dispatch#rework"},{"kind":"code","literal":"stop","chunkId":"dispatch#rework"},{"kind":"code","literal":"## Rework","chunkId":"dispatch#rework"},{"kind":"code","literal":"order.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"result.json","chunkId":"dispatch#rework"},{"kind":"code","literal":"report.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"status.md","chunkId":"dispatch#rework"},{"kind":"code","literal":"log","chunkId":"dispatch#rework"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#rework"}]
sources: [{"chunkId":"dispatch#rework","url":"/docs/dispatch#rework","anchor":"rework"}]
---

Rework sends a stopped run back for another attempt with a note, and the order then ends with a rework section. If the run is still running, ask before stopping it.
