---
id: "secrets#1password-failures"
title: "Secrets"
heading: "1Password failures"
group: "Operate"
order: 248
url: "/docs/secrets#1password-failures"
anchor: "1password-failures"
terms: ["1password","failures","troubleshooting","table","reads","missing","wrongly","permissioned","token","file","fallback","direct","notice","unavailable","ping","meaning","daemon","serves","account","restart","hevd","make","sure","caller","share","resolution","failure","wrong","vault","item","field","access","exit","rate","limited","service","required","desktop","unlock","disabled"]
hash: "b2605cac632f6897423d9078b38985e6b01835efcb2e0f1263fa12c74d7379b2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"1Password service-account token is required; desktop unlock disabled","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"HEV_OP_TOKEN_FILE","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"~/.config/op/token","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"hev secrets: notice: fallback to direct 1Password read","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"unavailable","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"1Password secret resolution failed (service account required; desktop unlock disabled)","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"op","chunkId":"secrets#1password-failures"}]
sources: [{"chunkId":"secrets#1password-failures","url":"/docs/secrets#1password-failures","anchor":"1password-failures"}]
---

Troubleshooting table for 1Password reads: a missing or wrongly permissioned token file, a fallback-to-direct notice or unavailable ping meaning no daemon serves this account (restart hevd and make sure daemon and caller share the token), a resolution failure from a wrong vault, item or field or missing access, and exit 75 for a rate-limited account.
