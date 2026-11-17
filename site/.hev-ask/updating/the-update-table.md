---
id: "updating#the-update-table"
title: "Updating and channels"
heading: "The [update] table"
group: "Operate"
order: 401
url: "/docs/updating#the-update-table"
anchor: "the-update-table"
terms: ["update","table","host","config","section","sets","channel","whether","daemon","updates","automatically","often","checks","least","minute","optional","commands","after","successful","rollback","those","post","hooks","operator","services","wrap","through","shell","version","environment","failing","hook","reported","does","undo","only","ever","come","file","changing"]
hash: "8937ffaec72c24dac2a855791708c150d1f96c5eb9c52f890bb7c08ef3f7f70c"
mode: "agent-primary"
facts: [{"kind":"code","literal":"[update]\nchannel = \"edge\"        # release (default) or edge\nauto = false            # let the daemon update this host on a schedule\ninterval = \"1h\"         # how often the daemon checks; at least 1m\npost_update = [         # run after each successful update or rollback\n  \"systemctl --user restart my-wrapper.service\",\n]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"[update]","chunkId":"updating#the-update-table"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"updating#the-update-table"},{"kind":"code","literal":"$HEV_CONFIG","chunkId":"updating#the-update-table"},{"kind":"code","literal":"post_update","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_FROM","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_TO","chunkId":"updating#the-update-table"},{"kind":"code","literal":"HEV_UPDATE_CHANNEL","chunkId":"updating#the-update-table"},{"kind":"code","literal":"hev update --channel","chunkId":"updating#the-update-table"},{"kind":"code","literal":"channel","chunkId":"updating#the-update-table"}]
sources: [{"chunkId":"updating#the-update-table","url":"/docs/updating#the-update-table","anchor":"the-update-table"}]
---

The host's own config has an update section that sets the channel, whether the daemon updates automatically, how often it checks (at least one minute), and optional commands to run after each successful update or rollback. Those post-update hooks are for an operator's own services that wrap hev; they run through the shell with the old version, new version and channel in the environment, a failing hook is reported but does not undo the update, and hooks only ever come from this file. Changing channel from the command line edits just that line and preserves comments and other settings.
