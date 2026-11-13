---
id: "updating#what-an-update-does"
title: "Updating and channels"
heading: "What an update does"
group: "Operate"
order: 342
url: "/docs/updating#what-an-update-does"
anchor: "what-an-update-does"
terms: ["update","does","steps","read","manifest","confirm","version","download","archive","architecture","verify","sha256","mismatch","stops","nothing","replaced","unpack","versioned","directory","once","keep","build","switch","binary","atomically","symlink","swap","unix","restart","capture","daemon","through","service","manager","json","checksums","versions","local","skills","sync"]
hash: "fb78c60cf9c9c84d9ad6e33a64fed50a31968ecb338ee716fab02bf9a5a76b4e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"manifest.json","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"checksums.txt","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/VERSION/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev version","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.local/bin/hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev up","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev skills sync","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"post_update","chunkId":"updating#what-an-update-does"}]
sources: [{"chunkId":"updating#what-an-update-does","url":"/docs/updating#what-an-update-does","anchor":"what-an-update-does"}]
---

Steps: read the manifest and confirm the version, download the archive for this OS and architecture, verify sha256 (a mismatch stops with nothing replaced), unpack into a versioned directory and run it once, keep the replaced build, switch the binary atomically (a symlink swap on Unix), and restart the capture daemon through its service manager.
