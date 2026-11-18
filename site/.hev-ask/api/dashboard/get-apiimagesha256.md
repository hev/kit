---
id: "api/dashboard#get-apiimagesha256"
title: "Dashboard API"
heading: "GET /api/image/\\{sha256\\}"
group: "API"
order: 42
url: "/docs/api/dashboard#get-apiimagesha256"
anchor: "get-apiimagesha256"
terms: ["image","sha256","returns","bytes","transcript","hash","only","images","machine","captured","available","elsewhere","flagged","local","responses","immutable","cached","restrictive","security","headers","support","conditional","range","requests","missing","plain","text","curl","shot","http","8787","ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8","accept","ranges","cache","control","private","31536000","content","length"]
hash: "c9445b28b63efb6f81ffea25a9db0fdeabe35b933c99f8df7bfbf77143ed7bab"
mode: "source-primary"
facts: [{"kind":"code","literal":"curl -s -D - -o shot.png \\\n  http://127.0.0.1:8787/api/image/ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"HTTP/1.1 200 OK\nAccept-Ranges: bytes\nCache-Control: private, max-age=31536000, immutable\nContent-Length: 86\nContent-Security-Policy: default-src 'none'; sandbox\nContent-Type: image/png\nEtag: \"ea2680dca17929494ec08fc92c65c36b68d4977cef8b4c3ced91cbdd732507d8\"\nVary: Accept-Encoding\nX-Content-Type-Options: nosniff","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"local: false","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"url","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"\u003carchive dir\u003e/\u003cnamespace\u003e/blobs","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Content-Type","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"ETag","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Cache-Control: private, max-age=31536000, immutable","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Content-Security-Policy: default-src 'none'; sandbox","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"X-Content-Type-Options: nosniff","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"If-None-Match","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"Range","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"code","literal":"404 page not found","chunkId":"api/dashboard#get-apiimagesha256"},{"kind":"value","literal":"SHA-256","chunkId":"api/dashboard#get-apiimagesha256"}]
sources: [{"chunkId":"api/dashboard#get-apiimagesha256","url":"/docs/api/dashboard#get-apiimagesha256","anchor":"get-apiimagesha256"}]
---

Returns the bytes of one image from a transcript by its hash. Only images this machine captured are available; images captured elsewhere are flagged as not local and have no URL. Responses are immutable-cached with restrictive security headers and support conditional and range requests; a missing image is a plain-text 404.
