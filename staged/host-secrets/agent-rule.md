# Proposed rule for agents reading 1Password

Proposed text only; nothing here is installed. The first block is for kit's
bundled skills (the section every agent reads, for example in `hev-dispatch`'s
worker guidance); the second replaces the reading instructions in the
`onepassword` skill. FOH places both.

## For the bundled skill

```markdown
## Secrets: read through the cache, once

Every host shares one 1Password account quota (1,000 requests a day for every
service account on every host). When it runs out, every host loses its secrets
until it resets.

- Read secrets only through kit's cache:
  `x=$(hev secrets read --label NAME --ttl 1h op://vault/item/field)`, or
  `hev secrets run --ref NAME=op://… --ttl NAME=1h -- command`. On Oven,
  `with-op op read REF` does the same. Never run `op read`, `op inject`,
  `op run` or `op item get` yourself.
- One attempt. If the answer is a rate limit (exit 75, "rate limit; retry
  after …"), stop and report the retry time. Do not retry, loop, sleep and try
  again, switch to `op`, or try another host or token.
- Never put a secret loader in a shell profile (`.bashrc`, `.zshrc`, `.zshenv`,
  `.profile`) or anything that runs per shell or per command. Load secrets in
  the one job that needs them.
- Never print, echo, log, commit or pass a value as a command argument; pass it
  in the environment or on a pipe.
- To check whether the cache is up, run `hev secrets ping` (no 1Password
  request). A line starting `hev secrets: notice: fallback to direct` means
  the read bypassed the cache; say so in your report instead of repeating it.
- Before blaming a key, run `hev secrets status`: it shows the quota, the
  cooldown and who has been reading (`~/.hev/secrets/op-reads.log`).
```

## For the `onepassword` skill

```markdown
## Reading a secret

Use kit's cache, never `op` directly:

| Need | Command |
| --- | --- |
| One value | `x=$(hev secrets read --label NAME --ttl 1h op://VAULT/ITEM/FIELD)` |
| Several, in this shell | `eval "$(hev secrets env --ref A=op://… --ref B=op://… --ttl A=1h --ttl B=1h)"` |
| For one program | `hev secrets run --ref A=op://… --ttl A=1h -- program` |
| On Oven, existing scripts | `with-op op read op://…` (now cached) |

The daemon answers repeats from memory until the TTL ends (at most 24h), so a
second read costs nothing. Every real request is a line in
`~/.hev/secrets/op-reads.log` with your caller name; pass
`--caller what-you-are` so the next outage can be traced to you, or to not you.

Rate limits: one attempt, then stop and report the retry time printed on
stderr (exit 75). No loops, no retries, no `op` fallback, no second token.

`op` itself is for vault administration only (creating or editing items),
never for reading a value a job or session needs. `with-op op whoami` and
similar still spend a request each.

Do not add a bulk loader to any shell profile. Source `~/shell/op-env.sh` only
inside the job that needs those variables.
```
