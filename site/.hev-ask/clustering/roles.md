---
id: "clustering#roles"
title: "Clustering"
heading: "Roles"
group: "Operate"
order: 124
url: "/docs/clustering#roles"
anchor: "roles"
terms: ["roles","owner","shell","host","sees","every","member","restricted","dispatch","sends","runs","manages","only","their","hears","finish","nodes","join","notices","pull","repos","config","toml","requested","access","usually","root","admin","locked","gets","running","machine","accepted","send","list","show","logs","stop","hear","finishes"]
hash: "fcc5f4299cfc6c4530adf217af7f780f97e74c266676a9dcd47dc94d3d3c6551"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev dispatch rpc","chunkId":"clustering#roles"},{"kind":"code","literal":"hev nodes join","chunkId":"clustering#roles"},{"kind":"code","literal":"hev dispatch notices pull","chunkId":"clustering#roles"},{"kind":"code","literal":"add_from","chunkId":"clustering#roles"},{"kind":"code","literal":"repos","chunkId":"clustering#roles"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"clustering#roles"},{"kind":"code","literal":"requested_by","chunkId":"clustering#roles"}]
sources: [{"chunkId":"clustering#roles","url":"/docs/clustering#roles","anchor":"roles"}]
---

Owner has shell on the host and sees every run; member has an SSH key restricted to dispatch rpc with no shell, sends runs and manages only their own, and hears when runs finish.
