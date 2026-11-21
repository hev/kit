---
id: "notifications#summaries-and-redaction"
title: "Notifications"
heading: "Summaries and redaction"
group: "Use"
order: 282
url: "/docs/notifications#summaries-and-redaction"
anchor: "summaries-and-redaction"
terms: ["summaries","redaction","feed","come","anthropic","messages","directly","default","haiku","effort","timeout","never","harness","model","sees","prompt","reply","tails","orders","rounds","board","bodies","tool","calls","output","input","redacted","errors","compact","line","posted","claude","sender","called","second","runs","posts","about","itself","turn"]
hash: "241281904e2e540bc0b0d99411c8f40a13accaca338a5005795e6abab986f707"
mode: "agent-primary"
facts: [{"kind":"code","literal":"claude-haiku-5-5","chunkId":"notifications#summaries-and-redaction"},{"kind":"code","literal":"model_key","chunkId":"notifications#summaries-and-redaction"},{"kind":"code","literal":"~/.hev/feed/sender.log","chunkId":"notifications#summaries-and-redaction"}]
sources: [{"chunkId":"notifications#summaries-and-redaction","url":"/docs/notifications#summaries-and-redaction","anchor":"summaries-and-redaction"}]
---

Feed summaries come from the Anthropic Messages API directly (default Haiku, low effort, 20s timeout), never via a harness. The model sees prompt and reply tails, orders, rounds or board bodies but never tool calls or output; all input and output is redacted. On errors or no key a compact line is posted.
