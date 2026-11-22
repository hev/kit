# hevkit.com

The site and docs for hev kit, built with Astro. It is pnpm-managed: use
`pnpm`, never `npm`.

```sh
pnpm install --frozen-lockfile
pnpm dev      # http://localhost:4341
pnpm build    # dist/
```

A new page under `src/content/docs/` is listed in `src/lib/docs.ts` to appear
in the sidebar.
`.github/workflows/site.yml` builds the site and checks its docs links on
every pull request that touches `site/`. The private preview on the release
train is `scripts/docs-preview.sh`; see `docs/runbooks/release-train.md`.

## Build settings

Both are read from the environment when `pnpm build` (or `pnpm dev`) runs.

| Setting | What it does | Unset |
| --- | --- | --- |
| `PUBLIC_KIT_APP_ORIGIN` | The origin of the hosted app where Club Kit sign-in and sign-up live, for example `https://app.example.test`. The sign-up and sign-in links are built from it. | The links stay on the site, at `/pricing#start`, and the copy says hosted sign-up isn't live yet. |
| `ANTHROPIC_API_KEY` | Lets `@hevmind/ask` rebuild the docs digest during the build. | The committed `.hev-ask/` digest is used. |

### `PUBLIC_KIT_APP_ORIGIN`

There is no default in the source, so a build that doesn't set it never links
to a hosted app. `src/lib/site.ts` is the only place that reads it.

With it set to `ORIGIN`:

| Link | Goes to |
| --- | --- |
| Header "Sign in" | `ORIGIN/` (sign-in, which offers sign-up) |
| "Sign up for Club Kit" on the home page, Club Kit's "Start" and the start section's button on `/pricing` | `ORIGIN/?sign-up=1` |
| "Have an invite code?" on the home page | `ORIGIN/?sign-up=1`. The app's invite page takes the code in its path (`ORIGIN/j/CODE`) and has nowhere to type one, so a link without a code goes to sign-up. |

The pricing page's start section and the Upgrade docs page (through the
`SignupOpen` and `SignupClosed` components) then say sign-up is open and what
happens when you sign up. Each of these links sends PostHog's `trial_signup`
event with a `location`.

A trailing slash on the origin is ignored. It is a `PUBLIC_` setting, so its
value ends up in the built HTML; it is an address, not a secret.
