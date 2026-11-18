// Site-wide links that more than one page or component reads.

// The hosted app's origin (where Field Kit sign-in and sign-up live), from the
// build-time setting PUBLIC_KIT_APP_ORIGIN, for example https://app.example.test.
// There is no default: a build that doesn't set it links nowhere off-site.
const origin = String(import.meta.env.PUBLIC_KIT_APP_ORIGIN ?? "")
	.trim()
	.replace(/\/+$/, "");

// True when this build knows where hosted sign-up is.
export const APP_LIVE = origin !== "";

// Without an app origin, every sign-up link goes to the pricing section that
// says hosted sign-up isn't live.
const NOT_LIVE_HREF = "/pricing#start";

// The sign-in page; it offers sign-up too. The header button goes here.
export const SIGNIN_HREF = APP_LIVE ? `${origin}/` : NOT_LIVE_HREF;
// The sign-up form. "Sign up for Field Kit" and the pricing Start button go here.
export const SIGNUP_HREF = APP_LIVE ? `${origin}/?sign-up=1` : NOT_LIVE_HREF;
// "Have an invite code?" The app's /j page takes a code in its path (/j/CODE)
// and has nowhere to type one, so without a code in hand the link goes to
// sign-up.
export const INVITE_HREF = SIGNUP_HREF;
