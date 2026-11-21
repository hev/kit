---
id: "quickstart#onboard-a-new-machine"
title: "Quickstart"
heading: "Onboard a new machine"
group: "Start"
order: 355
url: "/docs/quickstart#onboard-a-new-machine"
anchor: "onboard-a-new-machine"
terms: ["onboard","machine","onboarding","first","unconfigured","terminal","runs","also","runnable","anytime","asking","about","harnesses","detected","ones","keep","enter","workspace","data","location","default","third","choice","hand","edited","toml","check","capture","daemon","healthy","tell","last","scan","time","many","units","indexed","error","there","recent"]
hash: "faeba774a4ffa3ec0751bb36235781062354a98068782a8a71916f9d122db4b6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev s","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"Check that the hev capture daemon is healthy: run `hev s` and tell me the last scan time, how many units it indexed and the last error, if any. If there is no recent scan on Linux, check whether a systemd user manager is available and say so.\nDocs: https://hevkit.com/docs/quickstart#2-start-the-daemon (all pages: https://hevkit.com/llms.txt)","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev up","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/.hev/onboard.toml","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev onboard","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"PATH","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"claude, codex","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"1 2","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"none","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/workspace","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/.hev/","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"here","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev dispatch","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev up --non-interactive","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev onboard --no-picker","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"~/.hev","chunkId":"quickstart#onboard-a-new-machine"},{"kind":"code","literal":"hev d","chunkId":"quickstart#onboard-a-new-machine"}]
sources: [{"chunkId":"quickstart#onboard-a-new-machine","url":"/docs/quickstart#onboard-a-new-machine","anchor":"onboard-a-new-machine"}]
---

Onboarding a new machine: the first hev up on an unconfigured terminal runs hev onboard (also runnable anytime), asking about harnesses (detected ones, keep with Enter), the workspace and data location (default /workspace), and a third choice, with no hand-edited TOML.
