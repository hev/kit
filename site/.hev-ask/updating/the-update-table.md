---
id: "updating#the-update-table"
title: "Updating and channels"
heading: "The [update] table"
group: "Operate"
order: 446
url: "/docs/updating#the-update-table"
anchor: "the-update-table"
terms: ["update","table","config","channel","release","edge","auto","check","interval","post","options","host","default","false","daemon","schedule","often","checks","least","after","successful","rollback","systemctl","user","restart","wrapper","service","toml","hevconfig","postupdate","updates","file","names","true","another","asked","commands","only","gave","runs"]
hash: "8937ffaec72c24dac2a855791708c150d1f96c5eb9c52f890bb7c08ef3f7f70c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[update]\nchannel = \"edge\"        # release (default) or edge\nauto = false            # let the daemon update this host on a schedule\ninterval = \"1h\"         # how often the daemon checks; at least 1m\npost_update = [         # run after each successful update or rollback\n  \"systemctl --user restart my-wrapper.service\",\n]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"[update]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"updating#the-update-table"},{"kind":"code","literal":"$HEV_CONFIG","chunkId":"updating#the-update-table"},{"kind":"code","literal":"post_update","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_FROM","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_TO","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_CHANNEL","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev update --channel","chunkId":"updating#the-update-table"},{"kind":"code","literal":"channel","chunkId":"updating#the-update-table"}]
sources: [{"chunkId":"updating#the-update-table","url":"/docs/updating#the-update-table","anchor":"the-update-table"}]
---

The [update] config table: channel (release or edge), auto, check interval (min 1m) and post-update options, in the host's own config.
