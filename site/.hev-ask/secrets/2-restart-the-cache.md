---
id: "secrets#2-restart-the-cache"
title: "Secrets"
heading: "2. Restart the cache"
group: "Operate"
order: 295
url: "/docs/secrets#2-restart-the-cache"
anchor: "2-restart-the-cache"
terms: ["restart","cache","daemon","picks","1password","token","ping","unavailable","result","exit","means","shell","same","systemctl","user","hevd","linux","macos","windows","install","secrets","default","file","check","serves","account","another","drop","switch","first","service","page","describes","should","print","endpoint","prints","opserviceaccounttoken","hevoptokenfile","config"]
hash: "1d6934bd6eb9cc0354678b9c9206b5604060c8c4df8fe5a2d469cb17ce863caa"
mode: "agent-primary"
facts: [{"kind":"code","literal":"systemctl --user restart hevd   # Linux; on macOS and Windows see Install\nhev secrets ping","chunkId":"secrets#2-restart-the-cache"}]
sources: [{"chunkId":"secrets#2-restart-the-cache","url":"/docs/secrets#2-restart-the-cache","anchor":"2-restart-the-cache"}]
---

Restart the daemon so its cache picks up the 1Password token, then ping it; an unavailable result (exit 69) means the daemon and your shell do not see the same token.
