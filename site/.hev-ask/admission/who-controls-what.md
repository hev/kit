---
id: "admission#who-controls-what"
title: "Admission"
heading: "Who controls what"
group: "Operate"
order: 7
url: "/docs/admission#who-controls-what"
anchor: "who-controls-what"
terms: ["controls","only","owner","sets","allowed","organizations","grants","revokes","members","right","repos","granted","remove","within","those","anything","actually","clone","depends","whoever","manages","host","credentials","nodes","accept","allow","deny","admission","control","come","addfrom","dispatch","config","grant","list","shell","change","their","without","asking"]
hash: "90ad1ccb2619fbd950089146beb2eef16d4c9b0a7ec2ce8cff6ecbabaabe9d1f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"add_from","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes accept --allow repos","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev nodes allow","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"deny","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"hev admission add","chunkId":"admission#who-controls-what"},{"kind":"code","literal":"remove","chunkId":"admission#who-controls-what"}]
sources: [{"chunkId":"admission#who-controls-what","url":"/docs/admission#who-controls-what","anchor":"who-controls-what"}]
---

Only the owner sets the allowed organizations and grants or revokes members' right to add repos; granted members can add and remove repos within those organizations, and the owner can add anything. Which repos the bot can actually clone depends on whoever manages the host's git credentials.
