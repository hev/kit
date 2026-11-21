---
id: "updating#automatic-updates"
title: "Updating and channels"
heading: "Automatic updates"
group: "Operate"
order: 441
url: "/docs/updating#automatic-updates"
anchor: "automatic-updates"
terms: ["automatic","updates","need","both","auto","enabled","channel","capture","daemon","checks","interval","between","scan","cycles","same","path","update","default","true","state","json","host","needed","only","never","middle","after","switching","executes","itself","build","confirms","clean","cycle","fails","start","three","times","next","switches"]
hash: "d7d443c8215d05080c1f999c806c89e870f5bc25ee3e34a67c66688b39ca05f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"auto = true","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"channel","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"interval","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"hev update","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/update/state.json","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/hev.log","chunkId":"updating#automatic-updates"}]
sources: [{"chunkId":"updating#automatic-updates","url":"/docs/updating#automatic-updates","anchor":"automatic-updates"}]
---

Automatic updates need both auto enabled and a channel set; the capture daemon checks on its interval between scan cycles using the same path as hev update. Off by default.
