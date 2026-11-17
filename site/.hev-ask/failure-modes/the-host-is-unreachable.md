---
id: "failure-modes#the-host-is-unreachable"
title: "Failure modes"
heading: "The host is unreachable"
group: "Operate"
order: 112
url: "/docs/failure-modes#the-host-is-unreachable"
anchor: "the-host-is-unreachable"
terms: ["host","unreachable","failures","batch","mode","five","second","connect","timeout","strict","keys","never","prompt","mutating","commands","outcome","unknown","inspect","before","retrying","check","plain","works","same","account","error","exit","status","buildbox","port","operation","timed","mutations","named","true","hosts","verification","failed","known","command"]
hash: "1f5e97b93dfd81c49d99a5e4d2d5757bf30adda085f841e9e9276f7ca8107468"
mode: "agent-primary"
facts: [{"kind":"code","literal":"Error: ssh: exit status 255: ssh: connect to host buildbox port 22: Operation timed out; for mutations the outcome is unknown: inspect the named run before retrying","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"buildbox: unreachable: ssh: exit status 255: ...","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"ssh HOST true","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"hosts","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"ls","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"Host key verification failed.","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"known_hosts","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"exit status 127","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"hev: command not found","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"path_prefix","chunkId":"failure-modes#the-host-is-unreachable"},{"kind":"code","literal":"hev dispatch show NAME","chunkId":"failure-modes#the-host-is-unreachable"}]
sources: [{"chunkId":"failure-modes#the-host-is-unreachable","url":"/docs/failure-modes#the-host-is-unreachable","anchor":"the-host-is-unreachable"}]
---

SSH failures use batch mode, a five second connect timeout and strict host keys, and never prompt. For mutating commands the outcome is unknown, so inspect the run before retrying. Check that plain ssh to the host works from the same account.
