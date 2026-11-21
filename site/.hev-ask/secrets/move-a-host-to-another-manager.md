---
id: "secrets#move-a-host-to-another-manager"
title: "Secrets"
heading: "Move a host to another manager"
group: "Operate"
order: 379
url: "/docs/secrets#move-a-host-to-another-manager"
anchor: "move-a-host-to-another-manager"
terms: ["move","host","another","manager","switching","between","1password","vault","only","change","store","secret","under","same","name","project","file","changes","secrets","status","quota","grep","read","label","stripe","explain","hevlayer","moving","order","script","hevlayerapikeyref","uses","item","titled","field","reads","named","credential","azure","table"]
hash: "c172fe70b28010aacf0c0674e82cb9ec7df38c497eda03e1f1922fd74a9039a1"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev secrets status --no-quota | grep 'secret manager'\nhev secrets read --label STRIPE_API_KEY --explain secret://stripe-api-key | wc -c","chunkId":"secrets#move-a-host-to-another-manager"},{"kind":"code","literal":".env","chunkId":"secrets#move-a-host-to-another-manager"},{"kind":"code","literal":"HEVLAYER_API_KEY_REF","chunkId":"secrets#move-a-host-to-another-manager"},{"kind":"code","literal":"secret://","chunkId":"secrets#move-a-host-to-another-manager"},{"kind":"code","literal":"[secrets]","chunkId":"secrets#move-a-host-to-another-manager"},{"kind":"value","literal":"set-up-1password","chunkId":"secrets#move-a-host-to-another-manager"}]
sources: [{"chunkId":"secrets#move-a-host-to-another-manager","url":"/docs/secrets#move-a-host-to-another-manager","anchor":"move-a-host-to-another-manager"}]
---

Switching a host between 1Password and Key Vault is a host-only change: store each secret under the same NAME in the new store and change the host's manager; no project file using secret:// changes.
