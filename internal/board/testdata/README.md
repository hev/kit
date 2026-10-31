# Wire fixtures

`wire.json` contains JSON HTTP responses captured from the legacy board server
against the fixed Layer row in `row.json`, using a local HTTP fixture backend.
No production data or credentials were used. Cases cover listing, reading,
searching, useful votes, pinning, locking, and validation errors. Both the
server tests and client command tests consume these responses.

Server tests replay the original HTTP requests and compare complete decoded
responses and status codes. Client tests compare emitted HTTP methods, paths,
request envelopes, and decoded command output against those fixtures. Additional
post/reply request tests preserve the writing envelope. Canvas traversal and
sandbox tests cover the file-serving boundary.
