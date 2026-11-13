---
id: "order-reference#body"
title: "Order reference"
heading: "Body"
group: "Reference"
order: 207
url: "/docs/order-reference#body"
anchor: "body"
terms: ["body","free","markdown","requirement","line","reading","acceptance","heading","order","fails","parser","does","check","content","dispatch","generates","task","accept","bullets","rework","appends","section","copy","optionally","filled","failed","validation","items","missing","name","requires","thing","reads","surrounding","whitespace","ignored","without","contains","writes","text"]
hash: "832d143b3b959e56c2bcaf80eb9d0d248bd0c5f5f841c34dc8d6a8e3d4f6c5a9"
mode: "source-primary"
facts: [{"kind":"code","literal":"## Acceptance","chunkId":"order-reference#body"},{"kind":"code","literal":"missing ## Acceptance section","chunkId":"order-reference#body"},{"kind":"code","literal":"hev dispatch run","chunkId":"order-reference#body"},{"kind":"code","literal":"--accept","chunkId":"order-reference#body"},{"kind":"code","literal":"## Rework","chunkId":"order-reference#body"},{"kind":"code","literal":"~/dispatch/\u003cname\u003e/order.md","chunkId":"order-reference#body"},{"kind":"code","literal":"rework --from-validation","chunkId":"order-reference#body"}]
sources: [{"chunkId":"order-reference#body","url":"/docs/order-reference#body","anchor":"body"}]
---

The body is free Markdown with one requirement: a line reading the Acceptance heading, or the order fails. The parser does not check its content. dispatch run generates a body from the task and accept bullets, and rework appends a Rework section to the run's copy of the order, optionally filled from failed validation items.
