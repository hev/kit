---
id: "updating#windows"
title: "Updating and channels"
heading: "Windows"
group: "Operate"
order: 313
url: "/docs/updating#windows"
anchor: "windows"
terms: ["windows","running","cannot","overwritten","update","renames","aside","moves","build","place","removes","file","later","scheduled","task","ended","rerun","self","updating","daemon","exits","restarts","within","minute","name","once","nothing","runs","again","updates","itself"]
hash: "61864e24d2d2165d56bcfbb9b79dfeec8770f24fbf7b9b2844babc8d8c563bdf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev.exe","chunkId":"updating#windows"},{"kind":"code","literal":"hev update","chunkId":"updating#windows"},{"kind":"code","literal":"hev.exe.old-…","chunkId":"updating#windows"}]
sources: [{"chunkId":"updating#windows","url":"/docs/updating#windows","anchor":"windows"}]
---

A running hev.exe cannot be overwritten, so update renames it aside, moves the new build into place and removes the old file on a later update. The scheduled task is ended and rerun, and a self-updating daemon exits and the task restarts it within a minute.
