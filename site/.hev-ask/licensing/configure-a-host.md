---
id: "licensing#configure-a-host"
title: "Licensing"
heading: "Configure a host"
group: "Operate"
order: 268
url: "/docs/licensing#configure-a-host"
anchor: "configure-a-host"
terms: ["configure","host","license","config","secret","reference","azkv","literal","never","write","print","check","state","expiry","trial","status","toml","public","protect","file","keyref","install","table","give","itself","keep","rest","json","tell","licensed","grace","floor","whether","docs","https","hevkit","licensing","pages","llms","reads"]
hash: "66f6f4e71ab97483a34defb3dfc34203bb322880bc18f6aea3ece363b86087d6"
mode: "agent-primary"
facts: [{"kind":"code","literal":"# ~/.hev/config.toml\n[license]\nkey_ref = \"secret://kit-license\"    # or op://... or azkv://...\n# key = \"v4.public....\"             # a literal key; protect the file if you use it","chunkId":"licensing#configure-a-host"},{"kind":"code","literal":"HEV_LICENSE","chunkId":"licensing#configure-a-host"},{"kind":"code","literal":"HEV_LICENSE_REF","chunkId":"licensing#configure-a-host"},{"kind":"code","literal":"[license] key","chunkId":"licensing#configure-a-host"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"licensing#configure-a-host"},{"kind":"code","literal":"[license] key_ref","chunkId":"licensing#configure-a-host"}]
sources: [{"chunkId":"licensing#configure-a-host","url":"/docs/licensing#configure-a-host","anchor":"configure-a-host"}]
---

Configure a host's license in config via a secret reference (secret://, op://, azkv://) or a literal key; never write or print the key. Then check the license state, expiry and trial status.
