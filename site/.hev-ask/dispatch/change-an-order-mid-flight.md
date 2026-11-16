---
id: "dispatch#change-an-order-mid-flight"
title: "Dispatch"
heading: "Change an order mid-flight"
group: "Use"
order: 147
url: "/docs/dispatch#change-an-order-mid-flight"
anchor: "change-an-order-mid-flight"
terms: ["change","order","flight","amending","replaces","full","while","keeping","name","branch","saving","diff","history","live","body","edits","sent","worker","inbox","timeout","reply","validation","update","place","changing","host","harness","model","base","restarts","attempt","ended","starts","another","move","checks","capacity","admission","destination","carries"]
hash: "0acee20a19325a331bf0b494750692fa5549d41144b817e60dbdb5b88b74fcfc"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch amend api-rate-limit-headers revised.md\nhev dispatch amend api-rate-limit-headers - \u003c revised.md","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"amend","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"history.json","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"show","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"order.md","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"inbox.md","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"timeout","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"reply","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"validation","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"host","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"harness","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"model","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"base","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"say","chunkId":"dispatch#change-an-order-mid-flight"},{"kind":"code","literal":"rework","chunkId":"dispatch#change-an-order-mid-flight"}]
sources: [{"chunkId":"dispatch#change-an-order-mid-flight","url":"/docs/dispatch#change-an-order-mid-flight","anchor":"change-an-order-mid-flight"}]
---

Amending replaces the full order while keeping its name, branch and PR, saving the old order and a diff in history. On a live run, body edits are sent to the worker's inbox and timeout, reply and validation update in place, while changing host, harness, model or base restarts the attempt; an ended run starts another attempt. A host move checks capacity and admission at the destination, carries committed and uncommitted work with inbox and history, and leaves the original owner forwarding commands.
