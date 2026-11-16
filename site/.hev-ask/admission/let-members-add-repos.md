---
id: "admission#let-members-add-repos"
title: "Admission"
heading: "Let members add repos"
group: "Operate"
order: 1
url: "/docs/admission#let-members-add-repos"
anchor: "let-members-add-repos"
terms: ["members","repos","shell","host","owner","lists","organization","prefixes","come","grants","right","individual","entries","must","least","level","below","prefix","match","transport","without","setting","nobody","page","includes","prompt","configuring","admission","https","github","acme","azure","nodes","accept","request","allow","alex","deny","node","xxxx"]
hash: "659c276ef29e83d71d7a0ea7978970dd0a6c7c640005169c2da6c27bce03b793"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[admission]\nrepos = [\"https://github.com/acme/api.git\"]\nadd_from = [\n  \"https://github.com/acme/\",\n  \"https://dev.azure.com/acme/\",\n]","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev nodes accept REQUEST --allow repos\nhev nodes allow alex repos\nhev nodes deny alex repos","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev admission ls --host hev-node-XXXX\nhev admission add https://github.com/acme/web.git --host hev-node-XXXX","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"Error: your access does not include adding repos\nError: https://github.com/other/web.git is not in an allowed organization","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev admission remove https://github.com/acme/web.git --host hev-node-XXXX","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"add_from","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"[admission]","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"repos","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"https://","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"hev nodes ls","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"admission-clone.log","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"was not added from an allowed organization; ask the owner","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"add","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"remove","chunkId":"admission#let-members-add-repos"},{"kind":"code","literal":"repos = []","chunkId":"admission#let-members-add-repos"}]
sources: [{"chunkId":"admission#let-members-add-repos","url":"/docs/admission#let-members-add-repos","anchor":"let-members-add-repos"}]
---

Members have no shell on the host, so the owner lists the organization prefixes repos may come from and then grants the right to individual members. Entries must go at least one level below the prefix and match transport. Without the setting nobody can add repos, and the page includes a prompt for configuring it.
