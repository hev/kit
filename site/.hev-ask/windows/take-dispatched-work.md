---
id: "windows#take-dispatched-work"
title: "Windows"
heading: "Take dispatched work"
group: "Reference"
order: 320
url: "/docs/windows#take-dispatched-work"
anchor: "take-dispatched-work"
terms: ["take","dispatched","work","windows","host","needs","openssh","server","auth","senders","harness","path","session","sees","prefix","does","apply","existing","clone","origin","repo","sender","mark","workspace","optionally","wrapper","credentials","hosts","winbox","pwsh","noprofile","file","tools","creds","codex","claude","authentication","pathprefix","remote","harnesswrapper"]
hash: "72c49cd0ef20f5afef947bbee84b095dfa627f4d6069126596a9f48fe267129f"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.winbox]\nssh = \"dev@winbox\"\nos = \"windows\"\ncap = 3\nworkspace = \"C:/src\"\nharness_wrapper = [\"pwsh.exe\", \"-NoProfile\", \"-File\", \"C:/tools/with-creds.ps1\"]","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"hev.exe","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"codex","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"claude","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"path_prefix","chunkId":"windows#take-dispatched-work"},{"kind":"code","literal":"origin","chunkId":"windows#take-dispatched-work"}]
sources: [{"chunkId":"windows#take-dispatched-work","url":"/docs/windows#take-dispatched-work","anchor":"take-dispatched-work"}]
---

A Windows host needs OpenSSH Server with key auth from senders, hev.exe, Git and the harness on the PATH an SSH session sees (path prefix does not apply), and an existing clone with an origin per repo. On the sender, mark the host with os windows, a cap, the workspace and optionally a harness wrapper for credentials.
