---
id: "share-a-host#roles"
title: "Shared hosts"
heading: "Roles"
group: "Operate"
order: 337
url: "/docs/share-a-host#roles"
anchor: "roles"
terms: ["roles","compares","owner","member","shell","access","sees","every","installs","software","sets","credentials","admission","manages","members","locked","single","dispatch","command","send","runs","manage","only","their","stamped","name","boundary","limits","agent","does","host","account","invite","people","would","trust","keep","narrowly","scoped","nodes"]
hash: "35b1b182148df71349397fdd6dd74cf9bb91e5654f6d691edd662251b0ab2b9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev nodes join","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev dispatch notices pull","chunkId":"share-a-host#roles"},{"kind":"code","literal":"add_from","chunkId":"share-a-host#roles"},{"kind":"code","literal":"repos","chunkId":"share-a-host#roles"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"share-a-host#roles"},{"kind":"code","literal":"requested_by","chunkId":"share-a-host#roles"}]
sources: [{"chunkId":"share-a-host#roles","url":"/docs/share-a-host#roles","anchor":"roles"}]
---

Compares owner and member: the owner has shell access, sees every run, installs software, sets credentials and admission, and manages members; a member has an SSH key locked to a single dispatch command, can send runs and manage only their own, and each run is stamped with their name. The member boundary limits what they can ask hev to do, not what an agent does as the host account, so invite only people you would trust with that account and keep it narrowly scoped.
