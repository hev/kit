---
id: "share-a-host#roles"
title: "Shared hosts"
heading: "Roles"
group: "Operate"
order: 279
url: "/docs/share-a-host#roles"
anchor: "roles"
terms: ["roles","owner","controls","host","shell","installs","software","credentials","sets","admission","adds","removes","repos","invites","grants","permissions","member","locked","dispatch","send","runs","message","only","their","finish","notices","cannot","attach","clean","amend","rework","validate","manage","orchestrators","members","permission","within","allowed","prefixes","nodes"]
hash: "35b1b182148df71349397fdd6dd74cf9bb91e5654f6d691edd662251b0ab2b9e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev nodes join","chunkId":"share-a-host#roles"},{"kind":"code","literal":"hev dispatch notices pull","chunkId":"share-a-host#roles"},{"kind":"code","literal":"add_from","chunkId":"share-a-host#roles"},{"kind":"code","literal":"repos","chunkId":"share-a-host#roles"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"share-a-host#roles"},{"kind":"code","literal":"requested_by","chunkId":"share-a-host#roles"}]
sources: [{"chunkId":"share-a-host#roles","url":"/docs/share-a-host#roles","anchor":"roles"}]
---

Two roles. The owner controls the host with a shell, installs software and credentials, sets admission, adds and removes repos, and invites, removes and grants permissions. A member has an SSH key locked to dispatch rpc with no shell; they send runs, see and message only their own runs, and get finish notices, but cannot attach, clean, amend, rework, validate, or manage orchestrators. Members can add repos only with the repos permission and within the allowed prefixes.
