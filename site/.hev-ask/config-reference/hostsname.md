---
id: "config-reference#hostsname"
title: "Config reference"
heading: "[hosts.NAME]"
group: "Reference"
order: 64
url: "/docs/config-reference#hostsname"
anchor: "hostsname"
terms: ["hosts","name","host","table","matching","lowercase","pattern","fields","local","reach","optional","ceiling","workspace","path","prefixes","trace","namespace","harness","wrapper","alternate","supervisor","config","environment","variable","names","forward","invalid","entries","stop","sender","while","target","only","requires","entry","valid","context","variables","automatically","buildbox"]
hash: "f891d65fa7786c54dc8b603d8aadbdc6b3b1b5b4270cebb0f46a82c8a4fafd98"
mode: "source-primary"
facts: [{"kind":"code","literal":"[hosts.buildbox]\nssh = \"buildbox\"\ncap = 8\nnamespace = \"team-traces\"\npath_prefix = [\"~/.local/bin\"]\n\n[hosts.buildbox.env]\nNPM_TOKEN = \"\"","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"[hosts.NAME]","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"NAME","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"^[a-z0-9][a-z0-9-]*$","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"false","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"ssh","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"dev@buildbox","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"local = true","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"cap","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"os","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"\"windows\"","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"workspace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"repo","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"path_prefix","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"~/","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"namespace","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"HEVLAYER_NAMESPACE","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"harness_wrapper","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"--","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"config_path","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"env","chunkId":"config-reference#hostsname"},{"kind":"code","literal":"warning:","chunkId":"config-reference#hostsname"}]
sources: [{"chunkId":"config-reference#hostsname","url":"/docs/config-reference#hostsname","anchor":"hostsname"}]
---

Each host table has a name matching a lowercase pattern and fields for local or SSH reach, an optional cap ceiling, OS, workspace, PATH prefixes, trace namespace, a harness wrapper, an alternate supervisor config path, and environment variable names to forward. Invalid entries stop a sender, while a target only requires its own entry to be valid; trace context variables are set by the supervisor automatically.
