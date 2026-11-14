---
id: "join-a-host#4-wait-for-the-owner-to-accept"
title: "Join a host"
heading: "4. Wait for the owner to accept"
group: "Operate"
order: 201
url: "/docs/join-a-host#4-wait-for-the-owner-to-accept"
anchor: "4-wait-for-the-owner-to-accept"
terms: ["wait","owner","accept","after","accepts","request","list","nodes","find","host","alias","acceptance","status","install","member","skills","claude","code","codex","opencode","know","send","follow","runs","also","installs","team","repo","configured","leaves","existing","alone","should","rerun","joining","leaving","tell","node","joined","whether"]
hash: "c5d9582e414eb67d96e5be5757f72fe70b8d19639b6e9c35d00a321acda3937f"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes ls","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"Run `hev nodes ls` and tell me the hev-node-... alias of the host I joined and whether it shows as accepted. If it is not accepted yet, tell me to wait for the host owner.\nDocs: https://hevkit.com/docs/join-a-host#4-wait-for-the-owner-to-accept (all pages: https://hevkit.com/llms.txt)","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"hev skills setup","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"Run `hev skills setup` to install the hev member skills for Claude Code, Codex, pi and OpenCode. Tell me which folders it wrote to (such as ~/.claude/skills and ~/.agents/skills) and anything it left alone. Confirm a hev-member skill is now installed.\nDocs: https://hevkit.com/docs/join-a-host#4-wait-for-the-owner-to-accept (all pages: https://hevkit.com/llms.txt)","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"hev nodes accept","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"~/.claude/skills","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"},{"kind":"code","literal":"~/.agents/skills","chunkId":"join-a-host#4-wait-for-the-owner-to-accept"}]
sources: [{"chunkId":"join-a-host#4-wait-for-the-owner-to-accept","url":"/docs/join-a-host#4-wait-for-the-owner-to-accept","anchor":"4-wait-for-the-owner-to-accept"}]
---

After the owner accepts your request, list your nodes to find the host's alias and acceptance status. Then install the member skills so Claude Code, Codex, pi and OpenCode know how to send and follow runs; this also installs the team's own skills repo if configured, leaves your existing skills alone, and should be rerun after joining or leaving a host.
