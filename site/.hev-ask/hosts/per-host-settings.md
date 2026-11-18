---
id: "hosts#per-host-settings"
title: "Hosts"
heading: "Per-host settings"
group: "Operate"
order: 181
url: "/docs/hosts#per-host-settings"
anchor: "per-host-settings"
terms: ["host","settings","explains","marking","windows","relative","repo","paths","resolve","trace","namespace","passed","workers","harness","wrapper","must","wait","return","exit","code","extra","environment","variable","names","forwarded","unix","alternate","supervisor","config","cases","like","smoke","tests","follow","fixed","pattern","should","identical","every","because"]
hash: "3af59a7e9ba6fab3e667eb8e585a9e729ca1a8e292ca2d0d5617bd0162bbb866"
mode: "agent-primary"
facts: [{"kind":"code","literal":"os = \"windows\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"repo","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"~/workspace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"namespace","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"harness_wrapper","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"--","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"shift; exec \"$@\"","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"[hosts.NAME.env]","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"config_path","chunkId":"hosts#per-host-settings"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"hosts#per-host-settings"}]
sources: [{"chunkId":"hosts#per-host-settings","url":"/docs/hosts#per-host-settings","anchor":"per-host-settings"}]
---

Explains per-host settings: marking a Windows host, where relative repo paths resolve, the trace namespace passed to workers, a harness wrapper that must wait for the harness and return its exit code, extra environment variable names forwarded on Unix, and an alternate supervisor config for cases like smoke tests. Host names follow a fixed pattern and should be identical in every config because browser validation looks hosts up by name.
