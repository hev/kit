---
id: "config-reference#hostsname"
title: "Config reference"
heading: "[hosts.NAME]"
group: "Reference"
order: 64
url: "/docs/config-reference#hostsname"
anchor: "hostsname"
terms: ["hosts","name","host","tables","define","rules","fields","local","others","default","values","whether","sender","target","reads","buildbox","namespace","team","traces","path","prefix","token","false","true","windows","workspace","repo","hevlayer","harness","wrapper","config","warning","table","must","match","should","same","every","machine","field"]
hash: "5275ec944626bdeaf159811458073fafa89d2398b87cf60e0b7d4ea391d99a03"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\nnamespace = \"team-traces\"\npath_prefix = [\"~/.local/bin\"]\n\n[hosts.buildbox.env]\nNPM_TOKEN = \"\"","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"[hosts.NAME]","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"NAME","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"false","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"ssh","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"dev@buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local = true","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"cap","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"os","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"\"windows\"","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"workspace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"repo","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"path_prefix","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"~/","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"namespace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"harness_wrapper","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"--","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"config_path","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"env","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"warning:","chunkId":"config-reference#hostsname"}]
sources: [{"chunkId":"config-reference#hostsname","url":"/docs/config-reference#hostsname","anchor":"hostsname"}]
---

Host tables define each host's name rules and fields: local, ssh, cap and others, with default values and whether the sender or target reads them.
