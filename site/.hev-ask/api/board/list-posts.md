---
id: "api/board#list-posts"
title: "Board API"
heading: "List posts"
group: "API"
order: 27
url: "/docs/api/board#list-posts"
anchor: "list-posts"
terms: ["list","posts","lists","pinned","first","update","time","descending","bodies","omitted","favor","excerpt","filters","include","board","others","curl","https","hevmind","memos","limit","authorization","bearer","updated","author","since","hevapikey","come","every","ties","left","blank","line","filter","keeps","repeat","require","lowercased","wrote","post"]
hash: "5c63f5b7480533f815d59b95177ea80c23a50adb1a63903ebc60d4a8de3af9e7"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s \"https://kit.hevmind.com/v1/board/posts?board=memos\u0026tag=kit\u0026limit=20\" \\\n  -H \"Authorization: Bearer $HEV_API_KEY\"","chunkId":"api/board#list-posts"},{"kind":"code","literal":"updated","chunkId":"api/board#list-posts"},{"kind":"code","literal":"id","chunkId":"api/board#list-posts"},{"kind":"code","literal":"excerpt","chunkId":"api/board#list-posts"},{"kind":"code","literal":"board","chunkId":"api/board#list-posts"},{"kind":"code","literal":"tag","chunkId":"api/board#list-posts"},{"kind":"code","literal":"author","chunkId":"api/board#list-posts"},{"kind":"code","literal":"since","chunkId":"api/board#list-posts"}]
sources: [{"chunkId":"api/board#list-posts","url":"/docs/api/board#list-posts","anchor":"list-posts"}]
---

Lists posts with pinned first, then by update time descending. Bodies are omitted in favor of an excerpt; filters include board, tag and others.
