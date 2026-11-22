// Site-wide links that more than one page or component reads.

// Pricing and sign-in / sign-up, from the build-time setting
// PUBLIC_KIT_PRICING_SIGNIN. Off unless it is "1" or "true": the header's
// "Sign up / Sign in" button and pricing link, the home page's Club Kit
// sign-up calls to action, the /pricing page itself and the docs' pricing
// links are built only when it is on.
export const PRICING_SIGNIN = /^(1|true)$/i.test(String(import.meta.env.PUBLIC_KIT_PRICING_SIGNIN ?? "").trim());

// The hosted app's origin (where Club Kit sign-in and sign-up live), from the
// build-time setting PUBLIC_KIT_APP_ORIGIN, for example https://app.example.test.
// There is no default: a build that doesn't set it links nowhere off-site.
const origin = String(import.meta.env.PUBLIC_KIT_APP_ORIGIN ?? "")
	.trim()
	.replace(/\/+$/, "");

// True when this build links to hosted sign-up: pricing and sign-in are on
// and the build knows the app's origin. An origin alone turns nothing on.
export const APP_LIVE = PRICING_SIGNIN && origin !== "";

// Without an app origin, every sign-up link goes to the pricing section that
// says hosted sign-up isn't live. With pricing off there is no /pricing page,
// so they go to the Club Kit section of the Upgrade docs (nothing renders them
// then, but nothing should point at a page that isn't built).
const NOT_LIVE_HREF = PRICING_SIGNIN ? "/pricing#start" : "/docs/upgrade#club-kit";

// The sign-in page; it offers sign-up too. The header button goes here.
export const SIGNIN_HREF = APP_LIVE ? `${origin}/` : NOT_LIVE_HREF;
// The sign-up form. "Sign up for Club Kit" and the pricing Start button go here.
export const SIGNUP_HREF = APP_LIVE ? `${origin}/?sign-up=1` : NOT_LIVE_HREF;
// "Have an invite code?" The app's /j page takes a code in its path (/j/CODE)
// and has nowhere to type one, so without a code in hand the link goes to
// sign-up.
export const INVITE_HREF = SIGNUP_HREF;
