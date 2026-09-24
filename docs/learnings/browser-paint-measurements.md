# Measure paint with the bundled browser

For repeatable dashboard paint checks, use Playwright's bundled headless
Chromium (`chromium.launch({headless:true})`) and record both a paint entry and
a requestAnimationFrame after rows enter the DOM. DOMContentLoaded alone is
not evidence that the chrome was painted.

On this laptop, the installed Chrome channel held the first frame for roughly
4.6 seconds even though the API completed and rows entered the DOM in under
1.2 seconds, with no long task. Bundled Chromium painted chrome in 120–196 ms
and rows in 883–1,110 ms against the same isolated server and real namespace.
Blocking web fonts did not remove the installed Chrome delay. Report the
browser used; this local baseline does not replace mini acceptance.
