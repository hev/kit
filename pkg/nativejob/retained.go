package nativejob

import (
	"context"
	"errors"
	"time"
)

// RetainedExecutionOptions bounds the original owned workload. Unix retains
// root-only termination; a Windows backend must retain its per-workload Job
// through tree-empty terminal proof. The workload clock is independent of the
// admission/client context. No stopped
// supervisor or expired client is represented as workload termination.
type RetainedExecutionOptions struct{ WorkloadTimeout time.Duration }

const MaxRetainedWorkload = 24 * time.Hour
const WorkloadDeadlineEvidence = "native-workload-deadline/v1"

// ExecuteRetainedWithGate explicitly opts into owned-workload deadlines.
// It retains the platform's original process handles and Wait. Unix kills only
// the root; Windows requires owned job termination and tree-empty proof before
// a known terminal result. Coding-turn outcome remains the entrypoint owner's
// responsibility and is not inferred from root exit or tree cleanup.
func (s *Store) ExecuteRetainedWithGate(ctx context.Context, id string, gate FinalStartGate, options RetainedExecutionOptions, onRunning func(StartedObservation)) (Job, error) {
	if options.WorkloadTimeout <= 0 || options.WorkloadTimeout > MaxRetainedWorkload {
		return Job{}, errors.New("positive bounded retained workload timeout required")
	}
	return s.executeWithGateObserved(ctx, id, gate, onRunning, options.WorkloadTimeout)
}

// ProcessBirth reads local kernel process ownership. It neither adopts a process
// nor grants execution/termination permission. Unavailable identity fails closed.
func ProcessBirth(pid int) (Birth, error) { return processBirth(pid) }
