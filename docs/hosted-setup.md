# Hosted Field Kit setup

Create a named API key in Settings at https://kit.hevmind.com, then run:

```sh
brew install hev/tap/hev
export HEV_API_KEY=…
```

Replace `…` with the key shown once in Settings. No browser login is required.
Every hosted command resolves `HEV_API_KEY` first, then `[kit] key` in the
config, then no key. Without either key, existing local files, board and SSH
peers modes continue to work.

The service URL resolves from `hev login --service`, `HEV_KIT_SERVICE`,
`[kit] service`, then `https://kit.hevmind.com`. Existing stored `[kit] api`
continues to select the API for stored credentials. For a local fake or a
self-hosted service, set `HEV_KIT_SERVICE` to its origin; no extra variable is
needed for hosted Field Kit.

On first use of an exported key, the CLI makes one `/v1/whoami` request and
saves the verified key and identity in the private config (0600). This lets
hevd started by launchd/systemd use the key without inheriting your shell's
environment. A refused or unreachable key does not replace the saved key.
After adopting a new key, restart an already running daemon to switch its
in-memory client. Capture continues locally and queued batches survive 401s;
replace the key and restart the daemon to resume uploads.

`hev login` is an optional helper: it accepts `HEV_API_KEY`, `--key`, or a
key on stdin, verifies it with `/v1/whoami`, and stores it in the same private
config. Prefer env or stdin to putting a key in shell history. `hev login
--import` also imports existing history. `hev login --browser` explicitly
selects the retained legacy browser flow (including invite codes).

`hev logout` removes the stored key and restores the previous archive target;
it does not revoke the key in Settings. If `HEV_API_KEY` is exported it still
applies; unset it to stop using it. Revoke keys in Settings. The helper retains
the existing daemon writer coordination, including macOS launchctl bootout.
Never paste keys into logs, reports or issue descriptions. `hev config show`
redacts the hosted key. A 401 never clears credentials or deletes an outbox.

The base CLI has no hosted peers beacon/send, asks or usage routes. Its peers
commands use the existing local/SSH transport, including when a hosted key is
configured; no new API routes are implied by this setup.

Credential files are private: an existing config holding a hosted key is
narrowed to mode 0600 when read. If permissions cannot be narrowed, the
command refuses the file and names it; set its permissions to 0600 and retry.
An empty or whitespace-only HEV_API_KEY is unset; surrounding whitespace on
a real environment key is removed before use. Verified adoption and helper
login re-read the current document under a cross-process lock before saving,
so unrelated edits made during verification are preserved.
