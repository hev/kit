---
id: "hosts#per-host-settings"
title: "Hosts"
heading: "Per-host settings"
group: "Operate"
order: 119
url: "/docs/hosts#per-host-settings"
anchor: "per-host-settings"
terms: ["host","settings","mark","windows","hosts","workspace","relative","repo","paths","pass","trace","namespace","workers","wrapper","front","harness","entry","wins","forward","extra","environment","variable","names","point","supervisor","alternate","config","follow","lowercase","pattern","should","match","across","configs","hevlayer","shift","exec","name","path","marks"]
hash: "3af59a7e9ba6fab3e667eb8e585a9e729ca1a8e292ca2d0d5617bd0162bbb866"
mode: "agent-primary"
facts: [{"kind":"code","literal":"os = \"windows\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"repo","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"namespace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"harness_wrapper","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"--","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"shift; exec \"$@\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"[hosts.NAME.env]","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"config_path","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"hosts#per-host-settings"}]
sources: [{"chunkId":"hosts#per-host-settings","url":"/docs/hosts#per-host-settings","anchor":"per-host-settings"}]
---

Per-host settings mark Windows hosts, set the workspace for relative repo paths, pass a trace namespace to workers, run a wrapper in front of the harness (host's own entry wins), forward extra environment variable names, and point the supervisor at an alternate config. Host names follow a lowercase pattern and should match across configs.
