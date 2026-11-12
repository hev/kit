---
id: "secrets#2-restart-the-cache"
title: "Secrets"
heading: "2. Restart the cache"
group: "Operate"
order: 243
url: "/docs/secrets#2-restart-the-cache"
anchor: "2-restart-the-cache"
terms: ["restart","cache","token","default","file","hevd","picks","1password","account","check","ping","reports","unavailable","exit","make","sure","calling","shell","same","either","environment","systemctl","user","linux","macos","windows","install","secrets","serves","another","drop","switch","first","service","page","describes","should","print","endpoint","prints"]
hash: "1d6934bd6eb9cc0354678b9c9206b5604060c8c4df8fe5a2d469cb17ce863caa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"systemctl --user restart hevd   # Linux; on macOS and Windows see Install\nhev secrets ping","chunkId":"secrets#2-restart-the-cache"}]
sources: [{"chunkId":"secrets#2-restart-the-cache","url":"/docs/secrets#2-restart-the-cache","anchor":"2-restart-the-cache"}]
---

With the token in the default file, restart hevd so its cache picks up the 1Password account and check that ping reports up. If it reports unavailable (exit 69), make sure hevd and the calling shell see the same token, either the same environment token or the same token file.
