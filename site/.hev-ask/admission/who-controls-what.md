---
id: "admission#who-controls-what"
title: "Admission"
heading: "Who controls what"
group: "Operate"
order: 7
url: "/docs/admission#who-controls-what"
anchor: "who-controls-what"
terms: ["controls","table","part","admission","owner","alone","sets","organizations","repos","come","holds","remove","grant","granted","members","manage","list","within","those","whoever","manages","credentials","decides","actually","cloned","repo","allowed","organization","still","fails","cannot","read","nodes","accept","allow","deny","control","addfrom","only","host"]
hash: "90ad1ccb2619fbd950089146beb2eef16d4c9b0a7ec2ce8cff6ecbabaabe9d1f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"add_from","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes accept --allow repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes allow","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"deny","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev admission add","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"remove","chunkId":"admission#who-controls-what"}]
sources: [{"chunkId":"admission#who-controls-what","url":"/docs/admission#who-controls-what","anchor":"who-controls-what"}]
---

A table of who controls each part of admission: the owner alone sets which organizations repos may come from and who holds the add/remove grant, granted members manage the list within those organizations, and whoever manages the bot's git credentials decides what can actually be cloned. A repo in an allowed organization still fails if the bot cannot read it.
