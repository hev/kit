---
id: "hosts#per-host-settings"
title: "Hosts"
heading: "Per-host settings"
group: "Operate"
order: 174
url: "/docs/hosts#per-host-settings"
anchor: "per-host-settings"
terms: ["host","settings","cover","marking","windows","hosts","workspace","relative","repos","archive","namespace","passed","workers","harness","wrapper","must","wait","return","exit","code","wins","forwarded","environment","variable","names","unix","alternate","dispatch","config","supervisor","lowercase","consistent","across","configs","repo","hevlayer","shift","exec","name","path"]
hash: "3af59a7e9ba6fab3e667eb8e585a9e729ca1a8e292ca2d0d5617bd0162bbb866"
mode: "agent-primary"
facts: [{"kind":"code","literal":"os = \"windows\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"repo","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"namespace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"harness_wrapper","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"--","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"shift; exec \"$@\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"[hosts.NAME.env]","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"config_path","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"hosts#per-host-settings"}]
sources: [{"chunkId":"hosts#per-host-settings","url":"/docs/hosts#per-host-settings","anchor":"per-host-settings"}]
---

Per-host settings cover marking Windows hosts, the workspace for relative repos, the archive namespace passed to workers, a harness wrapper that must wait for and return the harness's exit code (host's own wins), forwarded environment variable names on Unix, and an alternate dispatch config for the supervisor. Host names must be lowercase and consistent across configs.
