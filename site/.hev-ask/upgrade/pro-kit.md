---
id: "upgrade#pro-kit"
title: "Upgrade"
heading: "Pro Kit"
group: "Start"
order: 409
url: "/docs/upgrade#pro-kit"
anchor: "pro-kit"
terms: ["pointed","layer","deployment","yourself","cloud","endpoint","store","yours","nothing","passes","through","hosted","service","supply","before","setup","interactive","enter","namespace","saved","config","daemon","whereas","board","server","reads","only","environment","configured","cannot","read","error","never","silent","fallback","files","there","option","gateway","docker"]
hash: "22d395c80c8063e37c3b53f2f3f363ee861f642f401825fbc951e316e600705c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"export HEVLAYER_API_KEY=...\nhev up","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev up","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev init","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hevd","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"HEVLAYER_API_KEY","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"HEVLAYER_API_KEY_OP_REF","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"op read","chunkId":"upgrade#pro-kit"},{"kind":"code","literal":"hev up --store turbopuffer","chunkId":"upgrade#pro-kit"}]
sources: [{"chunkId":"upgrade#pro-kit","url":"/docs/upgrade#pro-kit","anchor":"pro-kit"}]
---

Pro Kit is kit pointed at a hev layer deployment you run yourself in your own cloud: the endpoint, store and API key are yours and nothing passes through the hosted service. You supply the key before setup, or use the interactive setup to enter endpoint, key and namespace; the key is saved in the config for the daemon, whereas the board server reads its key only from the environment. A configured key that cannot be read is an error, never a silent fallback to files, and there is an option to run the hev layer gateway in Docker on a node in front of your own turbopuffer account.
