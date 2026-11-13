---
id: "secrets#1-put-the-token-in-the-token-file"
title: "Secrets"
heading: "1. Put the token in the token file"
group: "Operate"
order: 246
url: "/docs/secrets#1-put-the-token-in-the-token-file"
anchor: "1-put-the-token-in-the-token-file"
terms: ["token","file","first","step","1password","setup","store","service","account","default","daemon","callers","share","without","appearing","unit","profile","linux","macos","ignores","readable","group","others","directory","must","private","mkdir","config","chmod","touch","paste","editor","keep","every","caller","same","mode","grants","anything"]
hash: "8fe23cdcc426ea32e7858f6e5dba1b19f8a199311c9b3868f3a744719cd60430"
mode: "agent-primary"
facts: [{"kind":"code","literal":"mkdir -p ~/.config/op\nchmod 700 ~/.config/op\ntouch ~/.config/op/token\nchmod 600 ~/.config/op/token\n# paste the service-account token into ~/.config/op/token with an editor","chunkId":"secrets#1-put-the-token-in-the-token-file"},{"kind":"code","literal":"~/.config/op/token","chunkId":"secrets#1-put-the-token-in-the-token-file"}]
sources: [{"chunkId":"secrets#1-put-the-token-in-the-token-file","url":"/docs/secrets#1-put-the-token-in-the-token-file","anchor":"1-put-the-token-in-the-token-file"}]
---

First step of 1Password setup: store the service-account token in the default token file so the daemon and all callers share it without it appearing in a unit file or profile. On Linux and macOS kit ignores the file if it is readable by group or others, so the directory and file must be private.
