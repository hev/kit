---
id: "updating#automatic-updates"
title: "Updating and channels"
heading: "Automatic updates"
group: "Operate"
order: 334
url: "/docs/updating#automatic-updates"
anchor: "automatic-updates"
terms: ["automatic","updates","auto","enabled","channel","capture","daemon","checks","interval","between","scan","cycles","same","path","update","executes","itself","build","fails","start","three","times","switches","back","failed","never","moves","host","backwards","activity","recorded","state","file","default","true","json","both","needed","only","middle"]
hash: "d7d443c8215d05080c1f999c806c89e870f5bc25ee3e34a67c66688b39ca05f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"auto = true","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"channel","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"interval","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"hev update","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/update/state.json","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/hev.log","chunkId":"updating#automatic-updates"}]
sources: [{"chunkId":"updating#automatic-updates","url":"/docs/updating#automatic-updates","anchor":"automatic-updates"}]
---

With auto enabled and a channel set, the capture daemon checks on its interval between scan cycles and updates via the same path as hev update, then re-executes itself. If the new build fails to start three times it switches back. Failed checks back off up to a day, it never moves a host backwards, and activity is recorded in a state file and the log. It is off by default.
