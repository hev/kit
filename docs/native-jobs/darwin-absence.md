# Darwin process absence observation

The unchanged `nativejob.ProcessBirth(pid) (Birth, error)` API reads the kernel
boot-session UUID and process creation timeval. Live birth encoding is unchanged.
A failed or ambiguous lookup never authorizes PID adoption, termination, replay,
process-tree settlement or capacity release.

Darwin's scalar KinfoProc wrapper returns EIO when the kernel supplies an empty
PID reply. The adapter now preserves a successful empty query using the slice
wrapper and independently checks existence with signal zero. Only ESRCH after
that empty query wraps `os.ErrNotExist`; permission errors, other errors, and an
existing process without observable birth stay unknown. Signal zero does not
terminate a process. Invalid PIDs and exited-but-unreaped identities are not
promoted to definitive absence.

A native test starts an owned child, verifies its live birth, terminates it using
the retained process handle, waits/reaps, and verifies definite absence. Separate
negative checks keep permission and ambiguous observations unclassified. This
proves individual process absence only: the original harness group and escaped
or unattributed descendants require independent terminal/restart evidence.
