---
id: "secrets#1password-failures"
title: "Secrets"
heading: "1Password failures"
group: "Operate"
order: 241
url: "/docs/secrets#1password-failures"
anchor: "1password-failures"
terms: ["1password","failures","common","errors","missing","badly","permissioned","service","account","token","notice","falling","back","direct","read","unavailable","ping","daemon","serves","restart","hevd","ensure","same","resolution","wrong","vault","item","field","access","exit","meaning","rate","limited","required","desktop","unlock","disabled","file","config","secrets"]
hash: "b2605cac632f6897423d9078b38985e6b01835efcb2e0f1263fa12c74d7379b2"
mode: "agent-primary"
facts: [{"kind":"code","literal":"1Password service-account token is required; desktop unlock disabled","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"OP_SERVICE_ACCOUNT_TOKEN","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"HEV_OP_TOKEN_FILE","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"~/.config/op/token","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"hev secrets: notice: fallback to direct 1Password read","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"hev secrets ping","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"unavailable","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"1Password secret resolution failed (service account required; desktop unlock disabled)","chunkId":"secrets#1password-failures"},{"kind":"code","literal":"op","chunkId":"secrets#1password-failures"}]
sources: [{"chunkId":"secrets#1password-failures","url":"/docs/secrets#1password-failures","anchor":"1password-failures"}]
---

Common 1Password errors: a missing or badly permissioned service-account token, a notice of falling back to a direct read or an unavailable ping when no daemon serves that account (restart hevd and ensure the same token), resolution failures from a wrong vault, item or field or missing vault access, and exit 75 meaning the account is rate limited.
