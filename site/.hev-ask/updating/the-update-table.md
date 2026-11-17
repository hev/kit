---
id: "updating#the-update-table"
title: "Updating and channels"
heading: "The [update] table"
group: "Operate"
order: 339
url: "/docs/updating#the-update-table"
anchor: "the-update-table"
terms: ["update","table","host","config","sets","channel","release","edge","auto","daemon","check","interval","least","minute","post","shell","commands","after","successful","rollback","only","asked","show","file","before","default","false","schedule","often","checks","systemctl","user","restart","wrapper","service","toml","hevconfig","postupdate","updates","names"]
hash: "8937ffaec72c24dac2a855791708c150d1f96c5eb9c52f890bb7c08ef3f7f70c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[update]\nchannel = \"edge\"        # release (default) or edge\nauto = false            # let the daemon update this host on a schedule\ninterval = \"1h\"         # how often the daemon checks; at least 1m\npost_update = [         # run after each successful update or rollback\n  \"systemctl --user restart my-wrapper.service\",\n]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"[update]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"updating#the-update-table"},{"kind":"code","literal":"$HEV_CONFIG","chunkId":"updating#the-update-table"},{"kind":"code","literal":"post_update","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_FROM","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_TO","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_CHANNEL","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev update --channel","chunkId":"updating#the-update-table"},{"kind":"code","literal":"channel","chunkId":"updating#the-update-table"}]
sources: [{"chunkId":"updating#the-update-table","url":"/docs/updating#the-update-table","anchor":"the-update-table"}]
---

The update table in the host's config sets channel (release or edge), auto, the daemon check interval (at least one minute) and post-update shell commands that run after each successful update or rollback. Only add post-update commands if asked, and show the file before and after, then run a check.
