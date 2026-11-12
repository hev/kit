---
id: "secrets#1-put-the-token-in-the-token-file"
title: "Secrets"
heading: "1. Put the token in the token file"
group: "Operate"
order: 240
url: "/docs/secrets#1-put-the-token-in-the-token-file"
anchor: "1-put-the-token-in-the-token-file"
terms: ["token","file","keep","1password","service","account","under","user","config","directory","mode","daemon","callers","share","without","appearing","unit","files","profiles","ignores","grants","access","group","others","must","path","mkdir","chmod","touch","paste","editor","every","caller","same","profile","anything","prepare","host","secrets","create"]
hash: "e1631e79942fa2162fcd542fbc1b75768be2bb6510a41e1ab0389ccc6a3a3d68"
mode: "agent-primary"
facts: [{"kind":"code","literal":"mkdir -p ~/.config/op\nchmod 700 ~/.config/op\ntouch ~/.config/op/token\nchmod 600 ~/.config/op/token\n# paste the service-account token into ~/.config/op/token with an editor","chunkId":"secrets#1-put-the-token-in-the-token-file"},{"kind":"code","literal":"~/.config/op/token","chunkId":"secrets#1-put-the-token-in-the-token-file"}]
sources: [{"chunkId":"secrets#1-put-the-token-in-the-token-file","url":"/docs/secrets#1-put-the-token-in-the-token-file","anchor":"1-put-the-token-in-the-token-file"}]
---

Keep the 1Password service-account token in a token file under the user's op config directory, with the directory mode 700 and file mode 600, so the daemon and all callers share it without it appearing in unit files or profiles. kit ignores the file if it grants any access to group or others, and the op CLI must be on PATH.
