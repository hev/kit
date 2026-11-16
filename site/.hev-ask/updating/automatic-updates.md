---
id: "updating#automatic-updates"
title: "Updating and channels"
heading: "Automatic updates"
group: "Operate"
order: 392
url: "/docs/updating#automatic-updates"
anchor: "automatic-updates"
terms: ["automatic","updates","default","need","both","auto","setting","channel","capture","daemon","checks","interval","between","scan","cycles","same","path","manual","update","after","switching","executes","itself","confirms","clean","cycle","reverts","build","fails","start","three","times","failed","back","never","moves","host","backwards","everything","recorded"]
hash: "d7d443c8215d05080c1f999c806c89e870f5bc25ee3e34a67c66688b39ca05f9"
mode: "agent-primary"
facts: [{"kind":"code","literal":"auto = true","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"channel","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"interval","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"hev update","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/update/state.json","chunkId":"updating#automatic-updates"},{"kind":"code","literal":"~/.hev/hev.log","chunkId":"updating#automatic-updates"}]
sources: [{"chunkId":"updating#automatic-updates","url":"/docs/updating#automatic-updates","anchor":"automatic-updates"}]
---

Automatic updates are off by default and need both the auto setting and a channel; the capture daemon then checks on its interval, between scan cycles, and updates by the same path as a manual update. After switching it re-executes itself, confirms the update after a clean cycle, and reverts if the new build fails to start three times. Failed checks back off up to a day, it never moves a host backwards, and everything is recorded in an update state file and the log.
