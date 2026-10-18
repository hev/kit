
Startup registration uses the fixed receiver registration projection without a
job descriptor. A ten-second total observation budget and four concurrent probes
bound startup. Each profile writes an exclusive `native-host-startup/v1` receipt
under its state root, keyed by current host birth and typed profile digest. Its
state is `Observed` or `Held`; neither means healthy, available capacity, or launch
permission. Held observations and receipt failures block new dispatch for that
host birth while the independent capture loop continues. New dispatch still
requires its current descriptor-bound projection and original final launch gates.
History is never overwritten. A reviewed profile change requires host reload.

The read-only verifier also returns `profile_digests`, mapping profile IDs to
`nativejob.Digest(Profile)`. This is typed metadata, not runtime health. Socket
recovery preserves live and unknown endpoints; only a refused connection followed
by an immediate unchanged-inode/mode/owner check permits stale unlink. The check
and unlink are separate filesystem operations, not an atomic cross-process CAS.
