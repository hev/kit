---
id: "invited#4-wait-for-adam-to-accept"
title: "Your managed machine"
heading: "4. Wait for Adam to accept"
group: "Start"
order: 199
url: "/docs/invited#4-wait-for-adam-to-accept"
anchor: "4-wait-for-adam-to-accept"
terms: ["wait","adam","accept","after","adds","request","machine","list","nodes","find","alias","skills","setup","agents","member","team","existing","left","alone","should","rerun","joining","leaving","host","tell","node","joined","whether","shows","accepted","docs","https","hevkit","invited","pages","llms","install","claude","code","codex"]
hash: "d05661dcb9971e49e1d36a684bbf946cb66addb2d7636f484746ca1be3ca9c41"
mode: "agent-primary"
facts: [{"kind":"code","literal":"hev nodes ls","chunkId":"invited#4-wait-for-adam-to-accept"},{"kind":"code","literal":"Run `hev nodes ls` and tell me the hev-node-... alias of the host I joined and whether it shows as accepted. If it is not accepted yet, tell me to wait for Adam.\nDocs: https://hevkit.com/docs/invited#4-wait-for-adam-to-accept (all pages: https://hevkit.com/llms.txt)","chunkId":"invited#4-wait-for-adam-to-accept"},{"kind":"code","literal":"hev skills setup","chunkId":"invited#4-wait-for-adam-to-accept"},{"kind":"code","literal":"Run `hev skills setup` to install the hev member skills for Claude Code, Codex, pi and OpenCode. Tell me which folders it wrote to (such as ~/.claude/skills and ~/.agents/skills) and anything it left alone. Confirm a hev-member skill is now installed.\nDocs: https://hevkit.com/docs/invited#4-wait-for-adam-to-accept (all pages: https://hevkit.com/llms.txt)","chunkId":"invited#4-wait-for-adam-to-accept"},{"kind":"code","literal":"~/.claude/skills","chunkId":"invited#4-wait-for-adam-to-accept"},{"kind":"code","literal":"~/.agents/skills","chunkId":"invited#4-wait-for-adam-to-accept"}]
sources: [{"chunkId":"invited#4-wait-for-adam-to-accept","url":"/docs/invited#4-wait-for-adam-to-accept","anchor":"4-wait-for-adam-to-accept"}]
---

After Adam adds your request to the machine, list your nodes to find the machine's alias, then run skills setup so your agents get the member skills and any team skills. Existing skills are left alone, and setup should be rerun after joining or leaving a host.
