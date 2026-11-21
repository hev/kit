---
id: "api/peers#send-a-message"
title: "Peers API"
heading: "Send a message"
group: "API"
order: 60
url: "/docs/api/peers#send-a-message"
anchor: "send-a-message"
terms: ["send","message","posting","json","client","sender","text","recipient","messages","route","makes","idempotent","curl","https","hevmind","peers","mini","authorization","bearer","content","type","application","rebase","warning","docs","overhaul","20261119","about","force","push","hold","order","host","name","incarnation","thread","title","agent","desc","inbox"]
hash: "72241ecd4a5ce56e1ff1fe85ef40ee0e76d4b932c32d6908bf0a027e6c2acb4a"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s https://kit.hevmind.com/v1/peers/mini:foh-kit/messages \\\n  -H \"Authorization: Bearer $HEV_API_KEY\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\n    \"id\": \"rebase-warning-1\",\n    \"from\": \"mini/kit-docs-overhaul-20261119\",\n    \"message\": \"I am about to force-push d/kit-docs-overhaul-20261119. Hold your rebase.\",\n    \"order\": \"kit-docs-overhaul-20261119\"\n  }'","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"from","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"HOST/NAME","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"message","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"id","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"recipient_incarnation","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"409","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"order","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"run","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"thread","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"title","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"from_agent","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"from_cwd","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"from_desc","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"inbox.md","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"200","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"PeerMessage","chunkId":"api/peers#send-a-message"},{"kind":"code","literal":"header","chunkId":"api/peers#send-a-message"}]
sources: [{"chunkId":"api/peers#send-a-message","url":"/docs/api/peers#send-a-message","anchor":"send-a-message"}]
---

Send a message by posting JSON with a client id, sender, and message text to the recipient's messages route; the client id makes it idempotent.
