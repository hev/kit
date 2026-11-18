---
id: "updating#windows"
title: "Updating and channels"
heading: "Windows"
group: "Operate"
order: 402
url: "/docs/updating#windows"
anchor: "windows"
terms: ["windows","running","executable","cannot","overwritten","updater","renames","aside","moves","build","place","deletes","file","later","update","once","nothing","runs","scheduled","task","ended","again","daemon","updates","itself","exits","restarted","within","minute","name","removes","restarts"]
hash: "61864e24d2d2165d56bcfbb9b79dfeec8770f24fbf7b9b2844babc8d8c563bdf"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev.exe","chunkId":"updating#windows"},{"kind":"code","literal":"hev update","chunkId":"updating#windows"},{"kind":"code","literal":"hev.exe.old-…","chunkId":"updating#windows"}]
sources: [{"chunkId":"updating#windows","url":"/docs/updating#windows","anchor":"windows"}]
---

On Windows a running executable cannot be overwritten, so the updater renames it aside, moves the new build into its place, and deletes the old file on a later update once nothing runs it. The scheduled task is ended and run again, and a daemon that updates itself exits and is restarted by the task within a minute.
