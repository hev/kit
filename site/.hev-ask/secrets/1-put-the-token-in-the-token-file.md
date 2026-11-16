---
id: "secrets#1-put-the-token-in-the-token-file"
title: "Secrets"
heading: "1. Put the token in the token file"
group: "Operate"
order: 300
url: "/docs/secrets#1-put-the-token-in-the-token-file"
anchor: "1-put-the-token-in-the-token-file"
terms: ["token","file","1password","setup","step","keep","service","account","dedicated","daemon","every","caller","share","without","appearing","unit","files","profiles","linux","macos","ignored","permissions","grant","anything","group","others","mkdir","config","chmod","touch","paste","editor","same","profile","ignores","mode","grants"]
hash: "8fe23cdcc426ea32e7858f6e5dba1b19f8a199311c9b3868f3a744719cd60430"
mode: "agent-primary"
facts: [{"kind":"code","literal":"mkdir -p ~/.config/op\nchmod 700 ~/.config/op\ntouch ~/.config/op/token\nchmod 600 ~/.config/op/token\n# paste the service-account token into ~/.config/op/token with an editor","chunkId":"secrets#1-put-the-token-in-the-token-file"},{"kind":"code","literal":"~/.config/op/token","chunkId":"secrets#1-put-the-token-in-the-token-file"}]
sources: [{"chunkId":"secrets#1-put-the-token-in-the-token-file","url":"/docs/secrets#1-put-the-token-in-the-token-file","anchor":"1-put-the-token-in-the-token-file"}]
---

1Password setup, step one: keep the service-account token in a dedicated token file so the daemon and every caller share it without it appearing in unit files or profiles. On Linux and macOS the file is ignored if its permissions grant anything to group or others.
