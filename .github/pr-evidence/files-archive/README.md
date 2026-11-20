# Acceptance evidence: files-and-grep archive (kit 0.7)

Run on Oven (Linux, 32 cores) at the PR head with the scripts beside these
transcripts. `$WORK` is a scratch directory under /tmp.

| transcript | what |
| --- | --- |
| `accept-rg.txt` | clean HOME, no Docker on PATH, no hev layer key, `rg` on PATH: `hev up`, hevd captures a Claude and a Codex fixture, `hev traces ls/show/search/--also/--regex/--json`, `hev serve` answers `/api/search`, redaction delete removes the session's files |
| `accept-norg.txt` | the same with `rg` removed from PATH (the Go walk) |
| `systemd.txt` | a clean Linux user with a real systemd user manager (container, systemd as PID 1, no Docker inside): `hev up` installs `hevd.service` and `hev-serve.service`, the dashboard answers on :8099, `hev down` stops both |
| `migrate.txt` | kit main's keyless lane (Layer CE on Postgres in Docker) set up a machine; this branch's `hev up` then (2) starts the stopped containers and exports the rows, including a session whose transcript is gone, and (3) with no Docker on PATH re-indexes from disk and reports the sessions it could not recover |

The scripts never touch the host's own systemd units: the test HOMEs run with
no user bus (`XDG_RUNTIME_DIR` unset), and the systemd run is in a container.
