# Darwin native process bridge

The Darwin adapter uses the kernel's `kern.bootsessionuuid` and
`kern.proc.pid` creation timeval via pinned `golang.org/x/sys/unix` platform
layouts. It validates the returned PID, live state and creation timestamp. Missing
or restricted inspection fails closed; it never substitutes PID alone or an
estimated boot time. VM snapshot/restore still needs external incarnation/fence
reconciliation; local boot identity is not an anti-rollback oracle.

The same native job/gate/crash tests build on Linux and Darwin. Cross-builds:

```sh
GOOS=darwin GOARCH=arm64 go test -c -o nativejob-darwin-arm64.test ./pkg/nativejob
GOOS=darwin GOARCH=amd64 go test -c -o nativejob-darwin-amd64.test ./pkg/nativejob
```

These are compilation checks only. Actual native acceptance must use an isolated
source checkout on the enrolled Mac, record its exact SHA/OS/architecture, and run:

```sh
go test -race -count=1 ./pkg/nativejob
go build -o /absolute/owned/scratch/hev ./cmd/hev
python3 docs/native-jobs/example.py /absolute/owned/scratch/hev /absolute/new/scratch
```

Record actual boot/process birth, one child effect, artifact bytes/digests,
submit/result ACK loss, journal reopen, actual supervisor termination with a live
own child, Uncertain/no replay, inspection and final gate/cordon ordering. Only
signal owned fixture processes. Process crashes do not prove physical power-loss
storage behavior. Use canonical owned scratch paths for authority journals that
reject symlink aliases; on macOS `/var` may alias `/private/var`.

A separate exact-candidate public-facade/Kit adapter composite must prove the
actual Mac spawn under current chef/business/maintenance guards, receipt/Started
binding, parent/guest cordon, restart/ACK recovery and lock release before child
Wait. Passing facade-only tests or the Linux composite is not native Mac proof.
No shared daemon, installed binary, host enrollment or route is changed by source
validation. Windows journal execution remains separately disabled/unaccepted.
