---
id: "steering"
title: "Steering"
heading: null
group: "Use"
order: 402
url: "/docs/steering"
anchor: null
terms: ["steer","changes","busy","agent","doing","reports","whether","message","reach","next","tool","call","hold","mutations","until","acknowledged","halt","redirect","receipts","works","dispatch","runs","orchestrators","also","cover","empty","case","note","turn","scope","changed","candidate","level","urgent","cancelled","upstream","switch","name","acknowledges","tells"]
hash: "e1bdf6d300af273894eb1b7ff2bfca107b39142d7fbbdde9e310645ca6f311f8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev steer fix-ci 'Also cover the empty case'                      # note: next turn\nhev steer fix-ci 'Scope changed: candidate B, not A' --level urgent\nhev steer fix-ci 'CI was cancelled upstream' --level halt\nhev steer fix-ci 'Switch to candidate C' --level redirect\nhev steer ls fix-ci","chunkId":"steering"},{"kind":"code","literal":"hev steer","chunkId":"steering"},{"kind":"code","literal":"o-NAME","chunkId":"steering"}]
sources: [{"chunkId":"steering","url":"/docs/steering","anchor":null}]
---

hev steer changes what a busy agent is doing and reports whether it saw the message: reach it at its next tool call, hold mutations until acknowledged, halt or redirect it, with receipts. Works on dispatch runs and orchestrators.
