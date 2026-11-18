---
id: "config-reference"
title: "Config reference"
heading: null
group: "Reference"
order: 118
url: "/docs/config-reference"
anchor: null
terms: ["dispatch","config","toml","file","found","through","explicit","flag","environment","variable","default","path","unknown","field","error","same","serves","senders","find","reach","hosts","targets","admission","notifications","launch","settings","reference","marks","role","reads","capture","search","live","separate","codetabs","astro","agentprompt","every","type","meaning"]
hash: "a081e68e06c3e26ed8bb704dc18d3314fefab31a9b32dc633099f6b14831fa9d"
mode: "source-primary"
facts: [{"kind":"code","literal":"--config PATH","chunkId":"config-reference"},{"kind":"code","literal":"HEV_DISPATCH_CONFIG","chunkId":"config-reference"},{"kind":"code","literal":"~/.config/hev/dispatch.toml","chunkId":"config-reference"},{"kind":"code","literal":"unknown config key: KEY","chunkId":"config-reference"},{"kind":"code","literal":"hev dispatch","chunkId":"config-reference"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"config-reference"},{"kind":"value","literal":"CodeTabs.astro","chunkId":"config-reference"},{"kind":"value","literal":"AgentPrompt.astro","chunkId":"config-reference"}]
sources: [{"chunkId":"config-reference","url":"/docs/config-reference","anchor":null}]
---

The dispatch config is a TOML file found through an explicit flag, then an environment variable, then a default path, and any unknown field is an error. The same file serves senders (to find and reach hosts) and targets (for admission, notifications and launch settings), and the reference marks which role reads each field. Capture and search settings live in a separate kit config file.
