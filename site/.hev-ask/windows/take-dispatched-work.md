---
id: "windows#take-dispatched-work"
title: "Windows"
heading: "Take dispatched work"
group: "Reference"
order: 415
url: "/docs/windows#take-dispatched-work"
anchor: "take-dispatched-work"
terms: ["take","dispatched","work","windows","host","accept","needs","openssh","server","authentication","sender","executable","harness","path","session","sees","prefix","setting","does","apply","existing","clone","repository","origin","remote","sending","machine","entry","dispatch","config","marked","given","destination","capacity","workspace","optionally","credentials","wrapper","hosts","winbox"]
hash: "72c49cd0ef20f5afef947bbee84b095dfa627f4d6069126596a9f48fe267129f"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.winbox]\nssh = \"dev@winbox\"\nos = \"windows\"\ncap = 3\nworkspace = \"C:/src\"\nharness_wrapper = [\"pwsh.exe\", \"-NoProfile\", \"-File\", \"C:/tools/with-creds.ps1\"]","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"hev.exe","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"codex","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"claude","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"path_prefix","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"origin","chunkId":"windows#take-dispatched-work"}]
sources: [{"chunkId":"windows#take-dispatched-work","url":"/docs/windows#take-dispatched-work","anchor":"take-dispatched-work"}]
---

For a Windows host to accept dispatched work it needs OpenSSH Server with key authentication from each sender, the hev executable, Git and the harness on the path an SSH session sees (the path-prefix setting does not apply on Windows), and an existing clone of each repository with an origin remote. On the sending machine, the host's entry in the dispatch config is marked as Windows and given its SSH destination, capacity, workspace and, optionally, a credentials wrapper.
