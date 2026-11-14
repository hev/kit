---
id: "api/dashboard#get-apiimagesha256"
title: "Dashboard API"
heading: "GET /api/image/\\{sha256\\}"
group: "API"
order: 42
url: "/docs/api/dashboard#get-apiimagesha256"
anchor: "get-apiimagesha256"
terms: ["image","sha256","serves","captured","transcript","only","machine","cached","immutably","sandboxed","supports","conditional","range","requests","images","other","machines","return","plain","text","curl","shot","http","8787","ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8","accept","ranges","bytes","cache","control","private","31536000","immutable","content","length","security","policy","default","none","sandbox"]
hash: "c9445b28b63efb6f81ffea25a9db0fdeabe35b933c99f8df7bfbf77143ed7bab"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -D - -o shot.png \\\n  http://127.0.0.1:8787/api/image/ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"HTTP/1.1 200 OK\nAccept-Ranges: bytes\nCache-Control: private, max-age=31536000, immutable\nContent-Length: 86\nContent-Security-Policy: default-src 'none'; sandbox\nContent-Type: image/png\nEtag: \"ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8\"\nVary: Accept-Encoding\nX-Content-Type-Options: nosniff","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"local: false","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"url","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"\u003carchive dir\u003e/\u003cnamespace\u003e/blobs","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Content-Type","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"ETag","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Cache-Control: private, max-age=31536000, immutable","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Content-Security-Policy: default-src 'none'; sandbox","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"X-Content-Type-Options: nosniff","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"If-None-Match","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Range","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"404 page not found","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"value","literal":"SHA-256","chunkId":"api/dashboard#get-apiimagesha256"}]
sources: [{"chunkId":"api/dashboard#get-apiimagesha256","url":"/docs/api/dashboard#get-apiimagesha256","anchor":"get-apiimagesha256"}]
---

Serves an image captured in a transcript by its SHA-256, only if this machine captured it; it is cached immutably, sandboxed, and supports conditional and range requests. Images from other machines return a plain-text 404.
