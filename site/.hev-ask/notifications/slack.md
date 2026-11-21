---
id: "notifications#slack"
title: "Notifications"
heading: "Slack"
group: "Use"
order: 280
url: "/docs/notifications#slack"
anchor: "slack"
terms: ["slack","attention","notices","configured","notify","table","user","channel","token","secret","reference","kinds","default","needs","ready","done","optional","never","literal","config","toml","u0123456789","opens","read","through","secrets","widen","test","prefix","every","message","name","azkv","host","text","uncertain","dispatch","show","deliver","slackbottoken"]
hash: "45e73ece9de71bfaf6064ef9b056943a3c049366388290e396d8bba5f08e3ca7"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.hev/config.toml\n[notify.slack]\nchannel = \"U0123456789\"             # a user ID opens the bot's DM; or #channel / C...\ntoken = \"secret://SLACK_BOT_TOKEN\"  # a reference, read through hev secrets\nkinds = [\"needs-you\"]               # default; add \"ready\", \"done\" to widen\n# tag = \"[test]\"                    # optional prefix on every message","chunkId":"notifications#slack"},{"kind":"code","literal":"token","chunkId":"notifications#slack"},{"kind":"code","literal":"secret://NAME","chunkId":"notifications#slack"},{"kind":"code","literal":"op://…","chunkId":"notifications#slack"},{"kind":"code","literal":"azkv://…","chunkId":"notifications#slack"},{"kind":"code","literal":"channel","chunkId":"notifications#slack"},{"kind":"code","literal":"kinds","chunkId":"notifications#slack"},{"kind":"code","literal":"[\"needs-you\"]","chunkId":"notifications#slack"},{"kind":"code","literal":"done","chunkId":"notifications#slack"},{"kind":"code","literal":"[needs you] NAME on HOST: TEXT","chunkId":"notifications#slack"},{"kind":"code","literal":"uncertain","chunkId":"notifications#slack"},{"kind":"code","literal":"hev s","chunkId":"notifications#slack"},{"kind":"code","literal":"hev dispatch ls","chunkId":"notifications#slack"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"notifications#slack"},{"kind":"code","literal":"[notify.slack]","chunkId":"notifications#slack"},{"kind":"code","literal":"and","chunkId":"notifications#slack"},{"kind":"flag","literal":"--deliver","chunkId":"notifications#slack"},{"kind":"flag","literal":"--host","chunkId":"notifications#slack"}]
sources: [{"chunkId":"notifications#slack","url":"/docs/notifications#slack","anchor":"slack"}]
---

Slack attention notices are configured with a [notify.slack] table: a user ID (DM) or channel, a bot token secret reference, kinds (default needs-you; add ready, done) and an optional tag. Never put a literal token.
