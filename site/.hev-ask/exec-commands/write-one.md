---
id: "exec-commands#write-one"
title: "Exec commands"
heading: "Write one"
group: "Operate"
order: 216
url: "/docs/exec-commands#write-one"
anchor: "write-one"
terms: ["write","exec","command","making","executable","named","name","shell","script","local","directory","chmod","ensure","path","verify","runs","appears","help","hello","printf","world","make","confirm","prints","says","unknown","check","file","exactly","docs","https","hevkit","commands","pages","llms","plus","create","called","first","line"]
hash: "22b3ac29f0e94da46b807691b89c112640f807680f993168c4dd11da1794fabd"
mode: "agent-primary"
facts: [{"kind":"code","literal":"#!/bin/sh\n# ~/.local/bin/hev-hello\nprintf 'Hello %s\\n' \"$*\"","chunkId":"exec-commands#write-one"},{"kind":"code","literal":"chmod +x ~/.local/bin/hev-hello\nhev hello world","chunkId":"exec-commands#write-one"},{"kind":"code","literal":"Make my exec command ~/.local/bin/hev-hello executable with `chmod +x ~/.local/bin/hev-hello`, then run `hev hello world` and confirm it prints \"Hello world\". If hev says the command is unknown, check that ~/.local/bin is on PATH and that the file is named exactly hev-hello.\nDocs: https://hevkit.com/docs/exec-commands#write-one (all pages: https://hevkit.com/llms.txt)","chunkId":"exec-commands#write-one"},{"kind":"code","literal":"Hello world","chunkId":"exec-commands#write-one"},{"kind":"code","literal":"hev-","chunkId":"exec-commands#write-one"}]
sources: [{"chunkId":"exec-commands#write-one","url":"/docs/exec-commands#write-one","anchor":"write-one"}]
---

Write an exec command by making an executable named hev-NAME (e.g. a shell script in the local bin directory), chmod +x, ensure it's on PATH, then verify it runs and appears in help.
