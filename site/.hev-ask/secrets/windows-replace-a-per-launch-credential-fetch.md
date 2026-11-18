---
id: "secrets#windows-replace-a-per-launch-credential-fetch"
title: "Secrets"
heading: "Windows: replace a per-launch credential fetch"
group: "Operate"
order: 332
url: "/docs/secrets#windows-replace-a-per-launch-credential-fetch"
anchor: "windows-replace-a-per-launch-credential-fetch"
terms: ["windows","replace","launch","credential","fetch","moving","wrapper","cache","keep","engagement","boundary","validating","account","store","rejecting","ambient","credentials","allowlisting","child","settings","selecting","only","references","replaces","fetching","machine","scoped","dpapi","blob","depends","protected","validate","intended","reject","allowlist","nonsecret","select","localmachine","decrypted","other"]
hash: "b34ede3933fd8f15248acaa4acabc3fbf1f21b8aeadc5c4d49421e9738fb2653"
mode: "agent-primary"
facts: []
sources: [{"chunkId":"secrets#windows-replace-a-per-launch-credential-fetch","url":"/docs/secrets#windows-replace-a-per-launch-credential-fetch","anchor":"windows-replace-a-per-launch-credential-fetch"}]
---

When moving a Windows wrapper to the cache, keep its engagement boundary: validating the account and store ACL, rejecting ambient credentials, allowlisting child settings and selecting only that engagement's references. The cache replaces fetching, not the boundary, and a machine-scoped DPAPI blob depends on its protected ACL.
