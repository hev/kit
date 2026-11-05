# RFC 0012: Receiver FIFO dispatch queue

Status: proposed

A full receiver should keep an order until capacity is available. The initial
scope is deliberately a simple FIFO without queue management policy.

## Storage and admission

Each receiver stores plain JSON requests in `~/dispatch/.queue/`, beside its
run directories. The existing receiver root file lock serializes enqueue,
cancel and launch admission on Unix and Windows. Requests are atomically
renamed into place. Queue age is the recorded enqueue time; equal timestamps
sort by name. Duplicate names are refused across both runs and queued orders.
No central scheduler, network listener, or additional daemon is introduced.

Enqueue validates the order and repository admission policy before accepting it.
Launch rechecks current receiver policy and capacity. Environment values are
resolved at launch, never serialized into queue files. Later arrivals join the
FIFO even if a slot just freed, so they cannot jump ahead of waiting orders.

## Starting and recovery

A finishing supervisor releases its reservation and sweeps the queue. The
existing hevd scan cycle also sweeps, before archive indexing, so an archive
failure cannot prevent that cycle's sweep. Owners using an existing scheduled
task can invoke `hev dispatch sweep`. Both paths use the same implementation on
Unix and Windows. Concurrent sweepers recheck the head under the admission lock.
A crashed supervisor's orphan harness is stopped before the next launch.

The queue entry stays until a run directory and admission reservation exist.
A crash at that handoff leaves the run visible for inspection; a later sweep
removes the duplicate queue marker without launching the name twice. Setup
failures remain failed run records and require explicit rework. A failed head
never silently becomes a successful run.

## Visibility and cancellation

Enqueue prints the order's position. `dispatch ls` includes queued orders,
position and age. `dispatch show NAME` explains the FIFO wait; `dispatch stop
NAME` atomically removes an order that is still queued. Once admitted, normal
run stop semantics apply.

Without a selected host, dispatch searches reachable free hosts before choosing
a reachable receiver's queue. The configured default host is preferred only
when capacity is available or when all reachable hosts are full.

## Deferred policy

Per-sender limits, orchestrator shares, round-robin scheduling, expiry,
queue opt-out and start notices are possible later work, outside this initial
FIFO change. The parallel nodes proposal (RFC 0011) owns authenticated sender
identity; future fairness policy should use its bound sender metadata rather
than introducing another caller-selected identity.
