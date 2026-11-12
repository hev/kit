---
id: "admission#who-controls-what"
title: "Admission"
heading: "Who controls what"
group: "Operate"
order: 7
url: "/docs/admission#who-controls-what"
anchor: "who-controls-what"
terms: ["controls","only","owner","sets","organizations","repos","come","holds","grant","granted","members","remove","within","those","credentials","decide","actually","cloned","cannot","change","their","organization","list","nodes","accept","allow","deny","admission","control","addfrom","host","dispatch","config","anything","clone","whoever","manages","shell","without","asking"]
hash: "90ad1ccb2619fbd950089146beb2eef16d4c9b0a7ec2ce8cff6ecbabaabe9d1f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"add_from","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes accept --allow repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes allow","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"deny","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev admission add","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"remove","chunkId":"admission#who-controls-what"}]
sources: [{"chunkId":"admission#who-controls-what","url":"/docs/admission#who-controls-what","anchor":"who-controls-what"}]
---

Only the owner sets which organizations repos may come from and who holds the repos grant; granted members add and remove repos within those organizations, and the bot's git credentials decide what can actually be cloned. Members cannot change their own grant or the organization list.
