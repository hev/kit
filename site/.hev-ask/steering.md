---
id: "steering"
title: "Steering"
heading: null
group: "Use"
order: 341
url: "/docs/steering"
anchor: null
terms: ["steering","changes","busy","agent","doing","tells","whether","message","works","dispatch","runs","orchestrators","local","peer","sessions","four","levels","plain","note","urgent","halt","redirect","steer","also","cover","empty","case","next","turn","scope","changed","candidate","level","cancelled","upstream","switch","name","reach","tool","call"]
hash: "e1bdf6d300af273894eb1b7ff2bfca107b39142d7fbbdde9e310645ca6f311f8"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev steer fix-ci 'Also cover the empty case'                      # note: next turn\nhev steer fix-ci 'Scope changed: candidate B, not A' --level urgent\nhev steer fix-ci 'CI was cancelled upstream' --level halt\nhev steer fix-ci 'Switch to candidate C' --level redirect\nhev steer ls fix-ci","chunkId":"steering"},{"kind":"code","literal":"hev steer","chunkId":"steering"},{"kind":"code","literal":"o-NAME","chunkId":"steering"}]
sources: [{"chunkId":"steering","url":"/docs/steering","anchor":null}]
---

Steering changes what a busy agent is doing and tells you whether it saw the message. It works on dispatch runs, orchestrators and local peer sessions, at four levels from a plain note to an urgent message, a halt or a redirect.
