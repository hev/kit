---
id: "install#macos-and-linux"
title: "Install"
heading: "macOS and Linux"
group: "Start"
order: 188
url: "/docs/install#macos-and-linux"
anchor: "macos-and-linux"
terms: ["macos","linux","install","homebrew","formula","current","requires","trusting","before","installing","older","name","still","works","compatibility","alias","brew","trust","machine","verify","version","installed","stop","tell","docs","https","hevkit","pages","llms","remains"]
hash: "9756f26e6cb0fdd1e3b0da45b7331e3f8968fd3588d1628a136a81dabebf9804"
mode: "agent-primary"
facts: [{"kind":"code","literal":"brew tap hev/tap\nbrew trust hev/tap\nbrew install hev/tap/hev","chunkId":"install#macos-and-linux"},{"kind":"code","literal":"Install the hev CLI with Homebrew on this macOS or Linux machine: run brew tap hev/tap, brew trust hev/tap, then brew install hev/tap/hev. Verify with `hev version`. If Homebrew is not installed, stop and tell me.\nDocs: https://hevkit.com/docs/install#macos-and-linux (all pages: https://hevkit.com/llms.txt)","chunkId":"install#macos-and-linux"},{"kind":"code","literal":"hev/tap/kit","chunkId":"install#macos-and-linux"}]
sources: [{"chunkId":"install#macos-and-linux","url":"/docs/install#macos-and-linux","anchor":"macos-and-linux"}]
---

On macOS and Linux the install is a Homebrew formula from hev's own tap. Current Homebrew requires trusting the tap before installing, and the older formula name still works as a compatibility alias.
