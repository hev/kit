---
id: "plugins#write-one"
title: "Plugins"
heading: "Write one"
group: "Operate"
order: 233
url: "/docs/plugins#write-one"
anchor: "write-one"
terms: ["write","plugin","executable","named","plus","command","name","such","small","shell","script","user","local","make","ensure","directory","path","verify","runs","appears","help","reports","unknown","check","exact","file","hello","printf","chmod","world","confirm","prints","says","exactly","docs","https","hevkit","plugins","pages","llms"]
hash: "6c3f4139ded63599909170094dd7c28e5df9e6fa4e5ef77e6e2c0b41cc53ba33"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# ~/.local/bin/hev-hello\nprintf 'Hello %s\\n' \"$*\"","chunkId":"plugins#write-one"},{"kind":"code","literal":"chmod +x ~/.local/bin/hev-hello\nhev hello world","chunkId":"plugins#write-one"},{"kind":"code","literal":"Make my plugin ~/.local/bin/hev-hello executable with `chmod +x ~/.local/bin/hev-hello`, then run `hev hello world` and confirm it prints \"Hello world\". If hev says the command is unknown, check that ~/.local/bin is on PATH and that the file is named exactly hev-hello.\nDocs: https://hevkit.com/docs/plugins#write-one (all pages: https://hevkit.com/llms.txt)","chunkId":"plugins#write-one"},{"kind":"code","literal":"Hello world","chunkId":"plugins#write-one"},{"kind":"code","literal":"hev-","chunkId":"plugins#write-one"}]
sources: [{"chunkId":"plugins#write-one","url":"/docs/plugins#write-one","anchor":"write-one"}]
---

A plugin is an executable named hev- plus the command name, such as a small shell script in the user's local bin. Make it executable, ensure the directory is on PATH, and verify it runs and appears in help. If hev reports the command as unknown, check PATH and the exact file name.
