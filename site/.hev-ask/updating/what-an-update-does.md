---
id: "updating#what-an-update-does"
title: "Updating and channels"
heading: "What an update does"
group: "Operate"
order: 400
url: "/docs/updating#what-an-update-does"
anchor: "what-an-update-does"
terms: ["update","does","reads","build","manifest","downloads","archive","host","architecture","stops","nothing","replaced","checksum","match","unpacks","versioned","directory","test","runs","keeps","switches","atomically","restarts","capture","daemon","reverting","stay","refreshes","bundled","skills","post","commands","progress","never","signalled","keep","binary","started","current","previous"]
hash: "fb78c60cf9c9c84d9ad6e33a64fed50a31968ecb338ee716fab02bf9a5a76b4e"
mode: "agent-primary"
facts: [{"kind":"code","literal":"manifest.json","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"checksums.txt","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/VERSION/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev version","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.hev/versions/","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"~/.local/bin/hev","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev up","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"hev skills sync","chunkId":"updating#what-an-update-does"},{"kind":"code","literal":"post_update","chunkId":"updating#what-an-update-does"}]
sources: [{"chunkId":"updating#what-an-update-does","url":"/docs/updating#what-an-update-does","anchor":"what-an-update-does"}]
---

An update reads the build's manifest, downloads the archive for the host's OS and architecture, and stops with nothing replaced if the checksum does not match. It unpacks the new build into a versioned directory, test-runs it, keeps the old build, and switches atomically; it then restarts the capture daemon, reverting if the daemon does not stay up, refreshes bundled skills and runs the host's post-update commands. Runs in progress are never signalled and keep the binary they started with, and the current, previous and three most recent other builds are retained.
