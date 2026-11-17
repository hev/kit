---
id: "updating#package-managers"
title: "Updating and channels"
heading: "Package managers"
group: "Operate"
order: 400
url: "/docs/updating#package-managers"
anchor: "package-managers"
terms: ["package","managers","update","command","never","replaces","file","owned","manager","homebrew","install","stops","tells","upgrade","through","carries","releases","only","follow","edge","such","machine","uninstall","packaged","copy","release","archive","directory","path","switch","channel","there","scoop","system","directories","treated","same","brew","local","owns"]
hash: "dda88502a6d7fd5ecba479d8b2717318c112cc610f10e95218e6d58cb95e82b2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev update","chunkId":"updating#package-managers"},{"kind":"code","literal":"brew upgrade hev/tap/hev","chunkId":"updating#package-managers"},{"kind":"code","literal":"brew uninstall hev","chunkId":"updating#package-managers"},{"kind":"code","literal":"~/.local/bin","chunkId":"updating#package-managers"},{"kind":"code","literal":"hev up","chunkId":"updating#package-managers"},{"kind":"code","literal":"hev update --channel edge","chunkId":"updating#package-managers"},{"kind":"code","literal":"/usr/bin","chunkId":"updating#package-managers"}]
sources: [{"chunkId":"updating#package-managers","url":"/docs/updating#package-managers","anchor":"package-managers"}]
---

The update command never replaces a file owned by a package manager; on a Homebrew install it stops and tells you to upgrade through Homebrew, which carries releases only. To follow edge on such a machine, uninstall the packaged copy, install from a release archive into a directory you own on your path, and switch channel from there. Nix, Scoop and system package directories are treated the same way.
