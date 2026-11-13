# Staged host changes: every host reads 1Password through kit's cache

Nothing in this directory is installed by kit or by this change. Each section is
a host-by-host step for an operator to apply, check, and undo. Apply them in
order, one host at a time, after the kit release containing `hev secrets read`
and `hev secrets env` is installed on that host (`hev secrets read --help`
lists `--cache-only`).

References in these files are placeholders (`op://example/...`). The install
steps carry the live references over from the file being replaced, so no real
reference has to be published here.

Each step backs up what it replaces as `NAME.pre-secrets-cache`; undo restores
that backup. None of the steps prints, stores or moves a token or a value.

Checks after each step need the account's quota. Make one deliberate request,
never a loop, and read `hev secrets status` and
`~/.hev/secrets/op-reads.log` rather than calling `op` to see what happened.

## 0. kit on the host (both hosts)

Install the kit release and restart hevd the usual way for that host. With the
token in `~/.config/op/token` (mode 0600, as on both hosts today), hevd's
secrets endpoint now serves the account with no configuration.

Check: `hev secrets ping` prints `up ...` and exits 0, with no 1Password
request. `hev secrets status` prints `daemon: up since ...` instead of
`local secrets daemon unavailable`, and makes one quota request
(`op service-account ratelimit`, logged as `kind=ratelimit`); add
`--no-quota` to skip it.

Effect with no other step: hevd's own Layer-key read goes through the cache,
and so do `hev board serve` and any kit process that needs the key while hevd
is up. The native gate's database references go through it whenever `hev` is
on the receiver's PATH. On Oven the receiver profile's PATH holds only the
fixture `op`, so nothing there changes.

Undo: reinstall the previous kit release and restart hevd.

## 1. Oven: hevd drop-in (optional, one line)

Only needed to point hevd at a token file other than `~/.config/op/token`.

```sh
install -m 0644 linux/hevd-secrets.conf ~/.config/systemd/user/hevd.service.d/secrets.conf
systemctl --user daemon-reload
# restart hevd when the operator schedules it
```

Undo: `rm ~/.config/systemd/user/hevd.service.d/secrets.conf && systemctl --user daemon-reload`, then restart hevd.

## 2. Oven: `with-op`

Replaces `~/.local/bin/with-op`. `with-op op read REF` (used by
`oven-with-claude` on every worker Claude launch and by agents) becomes a
cached, logged `hev secrets read` with a one-hour TTL. Everything else runs
exactly as before and is logged by `op` subcommand only.

```sh
sha256sum ~/.local/bin/with-op                   # record
cp -p ~/.local/bin/with-op ~/.local/bin/with-op.pre-secrets-cache
install -m 0755 linux/with-op ~/.local/bin/with-op
sha256sum ~/.local/bin/with-op
```

Check (one request at most): `~/.local/bin/oven-with-claude true && tail -1 ~/.hev/secrets/op-reads.log`.
A second run inside the hour adds no line.

Undo: `mv ~/.local/bin/with-op.pre-secrets-cache ~/.local/bin/with-op`.

Knobs: `WITH_OP_TTL` (default `1h`), `WITH_OP_LABEL` (label in the log),
`HEV_SECRETS_CALLER`.

## 3. Mini and laptop: `~/shell/op-env.sh` (repo `hev/shell`)

Fixes the lost exports (the old batch path exported inside a pipeline, which
bash runs in a subshell, so every bash caller lost all seven values and then
spent seven `op read`s) and reads the whole map in one cached batch with a
one-hour TTL. Commit it to `hev/shell` once; both hosts pull it.

In a `hev/shell` checkout:

```sh
cp op-env.sh op-env.sh.pre-secrets-cache
live_map=$(sed -n "/^  cat <<'MAP'\$/,/^MAP\$/p" op-env.sh)
staged=PATH/TO/kit/staged/host-secrets/shell/op-env.sh
{ sed -n "1,/^  cat <<'MAP'\$/p" "$staged" | sed '$d'
  printf '%s\n' "$live_map"
  sed -n '/^MAP$/,$p' "$staged" | sed '1d'; } > op-env.sh.new
diff <(sed -n "/^  cat <<'MAP'\$/,/^MAP\$/p" op-env.sh.new) <(printf '%s\n' "$live_map") && mv op-env.sh.new op-env.sh
rm op-env.sh.pre-secrets-cache   # once committed; git is the backup
```

The `diff` must print nothing: the map block is carried over byte for byte.

Check (one request at most): `bash -c '. ~/shell/op-env.sh; env | cut -d= -f1 | sort'`
lists every mapped variable; `tail -1 ~/.hev/secrets/op-reads.log` shows one
`caller=op-env.sh:...` line, and sourcing again inside the hour adds none.

Undo: `git revert` the commit (or restore `op-env.sh.pre-secrets-cache`).

Knob: `OP_ENV_TTL` (default `1h`, at most `24h`).

## 4. Mini: Immich job

Replaces `~/.local/share/lab/immich/manage.sh`. `up` exits before resolving
anything when every compose service is already running, so the 15-minute
launchd job (`com.hev.immich`, `StartInterval 900`) reads 1Password only when
it actually starts containers. Its extra key goes through `hev secrets read`.
The plist does not change.

```sh
cd ~/.local/share/lab/immich
cp -p manage.sh manage.sh.pre-secrets-cache
ref=$(grep -o "op://[^\"' )]*" manage.sh | head -1)
sed "s|op://example/claude-api-key/credential|$ref|" PATH/TO/kit/staged/host-secrets/macos/immich-manage.sh > manage.sh.new
grep -c "$ref" manage.sh.new        # must print 1
chmod 0755 manage.sh.new && mv manage.sh.new manage.sh
```

Check: with Immich up, `bash manage.sh up; echo $?` prints 0, and the read log
gets no line. If `compose config` cannot run without the secret variables, the
check falls through to the old start path; that costs what it cost before.

Undo: `mv manage.sh.pre-secrets-cache manage.sh`.

## 5. A host without hevd (laptop, if needed)

Run the endpoint alone.

Linux:

```sh
install -m 0644 linux/hev-secrets.service ~/.config/systemd/user/hev-secrets.service
systemctl --user daemon-reload && systemctl --user enable --now hev-secrets.service
```

Undo: `systemctl --user disable --now hev-secrets.service && rm ~/.config/systemd/user/hev-secrets.service && systemctl --user daemon-reload`.

macOS:

```sh
sed "s|HOME_DIR|$HOME|g" macos/com.hev.secrets.plist > ~/Library/LaunchAgents/com.hev.secrets.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.hev.secrets.plist
```

Undo: `launchctl bootout gui/$(id -u)/com.hev.secrets && rm ~/Library/LaunchAgents/com.hev.secrets.plist`.

A host that runs hevd must not run this as well; the second endpoint refuses
to start ("secrets endpoint already running").

## 6. The rest of the mini's readers

Swap each `op read REF` for `hev secrets read --label NAME --ttl 1h REF`
(same output, cached, logged): the dashboard launcher, `run-standup*.sh`, the
cadence `run.sh`/`publish.sh`, `tools/timesheet.py` (`subprocess` with
`["hev", "secrets", "read", ...]`), `~/.local/bin/claude`, and the CRM
`loops_key()`, which today reads once per request. Each is a one-line change
with the same backup-and-restore undo as above.
