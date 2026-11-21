---
id: "api/traces#read-a-session"
title: "Traces API"
heading: "Read a session"
group: "API"
order: 67
url: "/docs/api/traces#read-a-session"
anchor: "read-a-session"
terms: ["read","session","reading","harness","returns","metadata","plus","blocks","related","detail","019a","claude","order","docs","overhaul","20261119","role","user","block","type","text","evals","poor","false","summary","trace","context","4650e875","parent","span","9a1c6e4b","traces","sessions","tracesessiondetail","takes","sessionid","blocktype","tracecontext","traceid","parentspanid"]
hash: "dd730950de37a74b2d5c9d6030df95ba1f109a616140f2acc21ae721cdc6408e"
mode: "source-primary"
facts: [{"kind":"code","literal":"{\n  \"session\": { \"session_id\": \"019a…\", \"harness\": \"claude\", \"order\": \"kit-docs-overhaul-20261119\", \"...\": \"...\" },\n  \"blocks\": [ { \"seq\": 1, \"role\": \"user\", \"block_type\": \"text\", \"...\": \"...\" } ],\n  \"evals\": [ { \"poor\": false, \"summary\": \"…\" } ],\n  \"trace_context\": { \"trace_id\": \"4650e875…\", \"parent_span_id\": \"9a1c6e4b…\", \"order\": \"kit-docs-overhaul-20261119\" }\n}","chunkId":"api/traces#read-a-session"},{"kind":"code","literal":"GET /v1/traces/sessions/{id}","chunkId":"api/traces#read-a-session"},{"kind":"code","literal":"TraceSessionDetail","chunkId":"api/traces#read-a-session"},{"kind":"code","literal":"blocks","chunkId":"api/traces#read-a-session"},{"kind":"code","literal":"seq","chunkId":"api/traces#read-a-session"},{"kind":"code","literal":"trace_context","chunkId":"api/traces#read-a-session"}]
sources: [{"chunkId":"api/traces#read-a-session","url":"/docs/api/traces#read-a-session","anchor":"read-a-session"}]
---

Reading a session by harness session id returns session metadata plus its blocks and related detail.
