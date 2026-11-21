---
id: "review#send-notes-back"
title: "Review"
heading: "Send notes back"
group: "Use"
order: 360
url: "/docs/review#send-notes-back"
anchor: "send-notes-back"
terms: ["send","notes","back","hunk","adds","note","replies","review","option","turns","kept","items","path","line","ranges","delivers","message","urgent","steer","live","else","reworks","ended","same","branch","deliveries","recorded","later","sends","include","only","edited","8c1d2e4f0a1b","internal","followthrough","stop","keep","order","behind","flag"]
hash: "92463ebd957c8329663c53637556dd55e07b8d20da1c5c5bf4a737a110194a64"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev review send RUN --dry-run\nhev review send RUN","chunkId":"review#send-notes-back"},{"kind":"code","literal":"Review notes for fix-ci at 8c1d2e4f0a1b from hunk (2):\n\n1. internal/followthrough/stop.go:44: Keep the old order behind a flag for one release.\n2. internal/followthrough/stop.go:52 (reply to fix-ci-review claude-sonnet-5-5 note \"Race on schedule removal\"): Agree, fix this before merge.","chunkId":"review#send-notes-back"},{"kind":"code","literal":"send","chunkId":"review#send-notes-back"},{"kind":"code","literal":"path:line","chunkId":"review#send-notes-back"},{"kind":"code","literal":"path:start-end","chunkId":"review#send-notes-back"},{"kind":"code","literal":"(old side)","chunkId":"review#send-notes-back"},{"kind":"code","literal":"hev steer RUN --level urgent","chunkId":"review#send-notes-back"},{"kind":"code","literal":"hev dispatch say RUN","chunkId":"review#send-notes-back"},{"kind":"code","literal":"--dry-run","chunkId":"review#send-notes-back"},{"kind":"code","literal":"--resend","chunkId":"review#send-notes-back"},{"kind":"code","literal":"--include-live","chunkId":"review#send-notes-back"},{"kind":"code","literal":"hunk session comment add","chunkId":"review#send-notes-back"}]
sources: [{"chunkId":"review#send-notes-back","url":"/docs/review#send-notes-back","anchor":"send-notes-back"}]
---

Send notes back: in hunk, c adds a note and R replies; review send (with a dry-run option) turns kept notes into items with path and line ranges and delivers one message, as an urgent steer if the run is live, else via say, which reworks an ended run on the same branch and PR. Deliveries are recorded so later sends include only new or edited notes.
