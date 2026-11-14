---
id: "windows#take-dispatched-work"
title: "Windows"
heading: "Take dispatched work"
group: "Reference"
order: 368
url: "/docs/windows#take-dispatched-work"
anchor: "take-dispatched-work"
terms: ["take","dispatched","work","windows","host","taking","needs","openssh","server","auth","harness","session","path","prefix","doesn","apply","existing","clones","origin","remotes","sender","declare","destination","marker","capacity","workspace","optional","credential","wrapper","hosts","winbox","pwsh","noprofile","file","tools","creds","codex","claude","authentication","sees"]
hash: "72c49cd0ef20f5afef947bbee84b095dfa627f4d6069126596a9f48fe267129f"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.winbox]\nssh = \"dev@winbox\"\nos = \"windows\"\ncap = 3\nworkspace = \"C:/src\"\nharness_wrapper = [\"pwsh.exe\", \"-NoProfile\", \"-File\", \"C:/tools/with-creds.ps1\"]","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"hev.exe","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"codex","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"claude","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"path_prefix","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"origin","chunkId":"windows#take-dispatched-work"}]
sources: [{"chunkId":"windows#take-dispatched-work","url":"/docs/windows#take-dispatched-work","anchor":"take-dispatched-work"}]
---

A Windows host taking dispatched work needs OpenSSH Server with key auth, hev, Git and the harness on the SSH session's PATH (path prefix doesn't apply), and existing clones with origin remotes. On the sender, declare the host with its SSH destination, Windows OS marker, capacity, workspace and optional credential wrapper.
