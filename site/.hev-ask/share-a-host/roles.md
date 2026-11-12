---
id: "share-a-host#roles"
title: "Shared hosts"
heading: "Roles"
group: "Operate"
order: 267
url: "/docs/share-a-host#roles"
anchor: "roles"
terms: ["roles","owner","controls","host","shell","access","sees","manages","every","installs","software","credentials","sets","admission","members","member","joins","invite","locked","dispatch","command","send","runs","manage","only","their","stamped","requested","extra","permission","today","lets","remove","repos","within","allowed","prefixes","agents","still","account"]
hash: "35b1b182148df71349397fdd6dd74cf9bb91e5654f6d691edd662251b0ab2b9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev nodes join","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev dispatch notices pull","chunkId":"share-a-host#roles"},{"kind":"code","literal":"add_from","chunkId":"share-a-host#roles"},{"kind":"code","literal":"repos","chunkId":"share-a-host#roles"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"share-a-host#roles"},{"kind":"code","literal":"requested_by","chunkId":"share-a-host#roles"}]
sources: [{"chunkId":"share-a-host#roles","url":"/docs/share-a-host#roles","anchor":"roles"}]
---

The owner controls the host with shell access, sees and manages every run, installs software and credentials, sets admission, and manages members. A member joins via invite, has an SSH key locked to the dispatch RPC command with no shell, can send runs and manage only their own, and every run is stamped with who requested it; the only extra permission today lets a member add and remove repos within allowed prefixes. Agents still run as the host account, so a member could ask an agent to read its credentials, so invite only trusted people and keep the account narrow.
