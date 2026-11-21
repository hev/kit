---
id: "config-reference#hostsname"
title: "Config reference"
heading: "[hosts.NAME]"
group: "Reference"
order: 160
url: "/docs/config-reference#hostsname"
anchor: "hostsname"
terms: ["hosts","name","table","host","lowercase","pattern","fields","include","local","workspace","namespace","path","prefixes","side","reads","buildbox","team","traces","prefix","token","false","true","windows","repo","hevlayer","harness","wrapper","config","must","match","should","same","every","machine","field","type","default","read","meaning","bool"]
hash: "3808db137da601141dd73a11fc93311bfd0225298c50ae68c5772eb0568f31e1"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\nnamespace = \"team-traces\"\npath_prefix = [\"~/.local/bin\"]\nenv = [\"NPM_TOKEN\"]","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"[hosts.NAME]","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"NAME","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"false","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"ssh","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"dev@buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local = true","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"cap","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"os","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"\"windows\"","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"workspace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"repo","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"path_prefix","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"~/","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"namespace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"harness_wrapper","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"--","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"config_path","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"env","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"[hosts.NAME.env]","chunkId":"config-reference#hostsname"}]
sources: [{"chunkId":"config-reference#hostsname","url":"/docs/config-reference#hostsname","anchor":"hostsname"}]
---

One table per host with a lowercase-name pattern; fields include local, ssh, cap, workspace, namespace, os and path prefixes, with which side reads each.
