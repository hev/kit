---
id: "admission#let-members-add-repos"
title: "Admission"
heading: "Let members add repos"
group: "Operate"
order: 1
url: "/docs/admission#let-members-add-repos"
anchor: "let-members-add-repos"
terms: ["members","repos","owners","shell","host","remove","themselves","owner","lists","allowed","organization","prefixes","config","grants","repo","right","individual","list","their","laptop","checks","against","organizations","clones","credentials","records","made","change","refusals","come","member","lacks","grant","outside","must","least","level","deeper","entry","match"]
hash: "659c276ef29e83d71d7a0ea7978970dd0a6c7c640005169c2da6c27bce03b793"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://github.com/acme/api.git\"]\nadd_from = [\n  \"https://github.com/acme/\",\n  \"https://dev.azure.com/acme/\",\n]","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev nodes accept REQUEST --allow repos\nhev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev admission ls --host hev-node-XXXX\nhev admission add https://github.com/acme/web.git --host hev-node-XXXX","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"Error: your access does not include adding repos\nError: https://github.com/other/web.git is not in an allowed organization","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev admission remove https://github.com/acme/web.git --host hev-node-XXXX","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"add_from","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"[admission]","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"repos","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"https://","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev nodes ls","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"admission-clone.log","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"was not added from an allowed organization; ask the owner","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"add","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"remove","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"repos = []","chunkId":"admission#let-members-add-repos"}]
sources: [{"chunkId":"admission#let-members-add-repos","url":"/docs/admission#let-members-add-repos","anchor":"let-members-add-repos"}]
---

Owners can let members (who have no shell on the host) add and remove repos themselves: the owner lists allowed organization URL prefixes in the host config, then grants the repo right to individual members. Members then list, add and remove repos from their laptop; the host checks the URL against the allowed organizations, clones it with its own git credentials, and records who made the change. Refusals come when the member lacks the grant or the repo is outside an allowed organization; prefixes must go at least one level deeper than the entry and match the transport (HTTPS vs SSH).
