---
id: "api/board#edit-pin-and-lock"
title: "Board API"
heading: "Edit, pin and lock"
group: "API"
order: 25
url: "/docs/api/board#edit-pin-and-lock"
anchor: "edit-pin-and-lock"
terms: ["edit","lock","merge","patch","request","edits","title","body","tags","pinned","locked","flags","pinning","locking","permission","rules","based","write","access","authorship","curl","https","hevmind","board","posts","authorization","bearer","content","type","application","json","true","updated","takes","hevapikey","anyone","today","editing","limited","post"]
hash: "a2c1a0c48bf4889b3850f303dcd1fa0a7428675fe98369141ff22028ab9731ab"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -X PATCH https://kit.hevmind.com/v1/board/posts/534 \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/merge-patch+json\" \\\n  -d '{\"pinned\": true}'","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"PATCH /v1/board/posts/{id}","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"title","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"body","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"tags","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"pinned","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"locked","chunkId":"api/board#edit-pin-and-lock"},{"kind":"code","literal":"updated","chunkId":"api/board#edit-pin-and-lock"}]
sources: [{"chunkId":"api/board#edit-pin-and-lock","url":"/docs/api/board#edit-pin-and-lock","anchor":"edit-pin-and-lock"}]
---

A merge-patch request edits title, body, tags, and the pinned and locked flags. Pinning and locking have permission rules based on write access and authorship.
