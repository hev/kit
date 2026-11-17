---
id: "windows#paths"
title: "Windows"
heading: "Paths"
group: "Reference"
order: 418
url: "/docs/windows#paths"
anchor: "paths"
terms: ["paths","table","things","live","under","user","profile","windows","host","dispatch","config","admission","default","workspace","relative","repository","values","record","directory","worktree","lock","setting","accepts","drive","absolute","order","either","userprofile","toml","repo","name","lockdir","path","including","status","report","result","branch","like"]
hash: "50d533210626256219d6bdc1ad0639292ffaa568700ee3f56d930cc84a511444"
mode: "source-primary"
facts: [{"kind":"code","literal":"%USERPROFILE%","chunkId":"windows#paths"},{"kind":"code","literal":"%USERPROFILE%\\.config\\hev\\dispatch.toml","chunkId":"windows#paths"},{"kind":"code","literal":"%USERPROFILE%\\workspace","chunkId":"windows#paths"},{"kind":"code","literal":"repo","chunkId":"windows#paths"},{"kind":"code","literal":"%USERPROFILE%\\dispatch\\NAME","chunkId":"windows#paths"},{"kind":"code","literal":"%USERPROFILE%\\dispatch\\NAME\\wt","chunkId":"windows#paths"},{"kind":"code","literal":"d/NAME","chunkId":"windows#paths"},{"kind":"code","literal":"%USERPROFILE%\\dispatch\\.lockdir","chunkId":"windows#paths"},{"kind":"code","literal":"workspace","chunkId":"windows#paths"},{"kind":"code","literal":"C:/src","chunkId":"windows#paths"}]
sources: [{"chunkId":"windows#paths","url":"/docs/windows#paths","anchor":"paths"}]
---

A table of where things live under the user profile on a Windows host: the dispatch config with admission, the default workspace for relative repository values, each run's record directory and its Git worktree, and the admission lock. The workspace setting accepts drive paths and UNC paths, and absolute repository values in an order can use either.
