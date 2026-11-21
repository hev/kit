---
id: "updating#what-an-update-does"
title: "Updating and channels"
heading: "What an update does"
group: "Operate"
order: 449
url: "/docs/updating#what-an-update-does"
anchor: "what-an-update-does"
terms: ["update","does","reads","build","manifest","checks","names","requested","version","downloads","archive","architecture","verifies","sha256","installs","swaps","binary","json","checksums","versions","local","skills","sync","post","release","published","before","manifests","asked","against","mismatch","stops","nothing","been","replaced","unpacks","runs","once","check","starts"]
hash: "fb78c60cf9c9c84d9ad6e33a64fed50a31968ecb338ee716fab02bf9a5a76b4e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"manifest.json","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"checksums.txt","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/VERSION/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev version","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.local/bin/hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev up","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev skills sync","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"post_update","chunkId":"updating#what-an-update-does"}]
sources: [{"chunkId":"updating#what-an-update-does","url":"/docs/updating#what-an-update-does","anchor":"what-an-update-does"}]
---

An update reads the build manifest, checks it names the requested version, downloads the archive for this OS/architecture, verifies its sha256, then installs and swaps the binary.
