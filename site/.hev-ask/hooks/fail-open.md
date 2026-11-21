---
id: "hooks#fail-open"
title: "Hooks"
heading: "Fail open"
group: "Use"
order: 235
url: "/docs/hooks#fail-open"
anchor: "fail-open"
terms: ["fail","open","hooks","handlers","under","time","budget","default","failures","dropped","warning","process","group","killed","hook","always","exits","disabled","config","also","sets","timeouts","including","halt","timeout","enabled","false","true","handler","event","long","halted","turn","warnings","toml","broken","must","never","stop","agent"]
hash: "18f0875a77bf761927cbfdd11a1c668f31ab0d8df16a7c608d3c992be2c92a3b"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[hooks]\nenabled = false        # default true\ntimeout = \"2s\"         # handler budget per event\nhalt_timeout = \"10m\"   # how long a halted run has to end its turn","chunkId":"hooks#fail-open"},{"kind":"code","literal":"[hooks] timeout","chunkId":"hooks#fail-open"},{"kind":"code","literal":"2s","chunkId":"hooks#fail-open"},{"kind":"code","literal":"~/.hev/hooks/warnings.log","chunkId":"hooks#fail-open"},{"kind":"code","literal":"hev hook","chunkId":"hooks#fail-open"},{"kind":"code","literal":"HEV_HOOKS=off","chunkId":"hooks#fail-open"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"hooks#fail-open"}]
sources: [{"chunkId":"hooks#fail-open","url":"/docs/hooks#fail-open","anchor":"fail-open"}]
---

Hooks fail open: handlers run under a time budget (default 2s); failures are dropped with a warning log and the process group killed, and hev hook always exits 0. Hooks can be disabled by env var or config, which also sets timeouts including halt timeout.
