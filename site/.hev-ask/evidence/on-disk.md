---
id: "evidence#on-disk"
title: "Evidence"
heading: "On disk"
group: "Use"
order: 206
url: "/docs/evidence#on-disk"
anchor: "on-disk"
terms: ["disk","evidence","lives","root","directory","config","default","folder","package","manifest","numbered","items","directories","private","0700","files","0600","writes","atomic","matches","shape","packages","move","unchanged","01jd2x8c7k4m5n6p7q8r9s0t1v","json","checkout","390px","submit","console","redacted","text","toml","evidencepackage","hevevidencedir","else","ev01jd2x8c7k4m5n6p7q8r9s0t1v","every","write","same"]
hash: "7bc57b6da32c71e8185afa332fc076893d43c6cbc42a273b6df7b19313d36d07"
mode: "agent-primary"
facts: [{"kind":"code","literal":"~/.hev/evidence/\n  ev_01jd2x8c7k4m5n6p7q8r9s0t1v/\n    manifest.json                # hev.evidence.package/v1\n    001-checkout-390px.png\n    002-checkout-submit.mp4\n    003-console.log              # redacted text","chunkId":"evidence#on-disk"},{"kind":"code","literal":"$HEV_EVIDENCE_DIR","chunkId":"evidence#on-disk"},{"kind":"code","literal":"[evidence] dir","chunkId":"evidence#on-disk"},{"kind":"code","literal":"~/.hev/config.toml","chunkId":"evidence#on-disk"},{"kind":"code","literal":"~/.hev/evidence","chunkId":"evidence#on-disk"},{"kind":"code","literal":"0700","chunkId":"evidence#on-disk"},{"kind":"code","literal":"0600","chunkId":"evidence#on-disk"},{"kind":"code","literal":"manifest.json","chunkId":"evidence#on-disk"},{"kind":"code","literal":"EvidencePackage","chunkId":"evidence#on-disk"},{"kind":"code","literal":"hev s","chunkId":"evidence#on-disk"},{"kind":"code","literal":"~/.hev","chunkId":"evidence#on-disk"}]
sources: [{"chunkId":"evidence#on-disk","url":"/docs/evidence#on-disk","anchor":"on-disk"}]
---

Evidence lives in a root directory set by env var, config, or the default kit folder; each package has a manifest and numbered items. Directories are private (0700), files 0600, writes atomic, and the manifest matches the Pro API shape so packages move to Pro unchanged.
