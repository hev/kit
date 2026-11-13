---
id: "updating#package-managers"
title: "Updating and channels"
heading: "Package managers"
group: "Operate"
order: 338
url: "/docs/updating#package-managers"
anchor: "package-managers"
terms: ["package","managers","update","never","replaces","file","owned","manager","homebrew","stops","tells","brew","upgrade","installs","releases","only","follow","edge","there","uninstall","install","release","archive","user","path","directory","channel","scoop","system","directories","treated","same","local","owns","says","instead","such","machine","already","puts"]
hash: "dda88502a6d7fd5ecba479d8b2717318c112cc610f10e95218e6d58cb95e82b2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev update","chunkId":"updating#package-managers"},{"kind":"code","literal":"brew upgrade hev/tap/hev","chunkId":"updating#package-managers"},{"kind":"code","literal":"brew uninstall hev","chunkId":"updating#package-managers"},{"kind":"code","literal":"~/.local/bin","chunkId":"updating#package-managers"},{"kind":"code","literal":"hev up","chunkId":"updating#package-managers"},{"kind":"code","literal":"hev update --channel edge","chunkId":"updating#package-managers"},{"kind":"code","literal":"/usr/bin","chunkId":"updating#package-managers"}]
sources: [{"chunkId":"updating#package-managers","url":"/docs/updating#package-managers","anchor":"package-managers"}]
---

hev update never replaces a file owned by a package manager. On Homebrew it stops and tells you to use brew upgrade, and Homebrew installs releases only. To follow edge there, uninstall via brew, install from a release archive into a user-owned PATH directory and run update with the edge channel. Nix, Scoop and system directories are treated the same.
