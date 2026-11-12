---
id: "windows#take-dispatched-work"
title: "Windows"
heading: "Take dispatched work"
group: "Reference"
order: 306
url: "/docs/windows#take-dispatched-work"
anchor: "take-dispatched-work"
terms: ["take","dispatched","work","windows","host","needs","openssh","server","authentication","sender","harness","session","path","prefix","setting","does","apply","existing","clones","origin","remote","mark","dispatch","config","destination","capacity","workspace","optionally","wrapper","verify","reachable","free","slots","hosts","winbox","pwsh","noprofile","file","tools","creds"]
hash: "72c49cd0ef20f5afef947bbee84b095dfa627f4d6069126596a9f48fe267129f"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.winbox]\nssh = \"dev@winbox\"\nos = \"windows\"\ncap = 3\nworkspace = \"C:/src\"\nharness_wrapper = [\"pwsh.exe\", \"-NoProfile\", \"-File\", \"C:/tools/with-creds.ps1\"]","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"hev.exe","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"codex","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"claude","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"path_prefix","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"origin","chunkId":"windows#take-dispatched-work"}]
sources: [{"chunkId":"windows#take-dispatched-work","url":"/docs/windows#take-dispatched-work","anchor":"take-dispatched-work"}]
---

A Windows host needs OpenSSH Server with key authentication from each sender, hev.exe, Git and the harness on the SSH session's PATH (the path prefix setting does not apply), and existing clones with an origin remote. On the sender, mark the host as Windows in its dispatch config with SSH destination, capacity, workspace and optionally a harness wrapper, then verify it is reachable with free slots.
