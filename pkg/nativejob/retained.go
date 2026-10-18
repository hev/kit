package nativejob

import (
	"context"
	"errors"
	"time"
)

// RetainedExecutionOptions bounds the owned root process, not its descendants.
// The workload clock is independent of the admission/client context. No stopped
// supervisor or expired client is represented as workload termination.
type RetainedExecutionOptions struct{ WorkloadTimeout time.Duration }

const MaxRetainedWorkload = 24 * time.Hour
const WorkloadDeadlineEvidence = "native-workload-deadline/v1"

// ExecuteRetainedWithGate is explicit opt-in root-process deadline enforcement.
// It keeps the original exec.Cmd owner and Wait. Deadline kills only that owned
// root handle, then collects its actual exit/artifacts. Descendant cleanup and
// coding-turn outcome remain the foreground entrypoint owner's responsibility.
func (s *Store) ExecuteRetainedWithGate(ctx context.Context, id string, gate FinalStartGate, options RetainedExecutionOptions, onRunning func(StartedObservation)) (Job, error) {
	if options.WorkloadTimeout <= 0 || options.WorkloadTimeout > MaxRetainedWorkload {
		return Job{}, errors.New("positive bounded retained workload timeout required")
	}
	return s.executeWithGateObserved(ctx, id, gate, onRunning, options.WorkloadTimeout)
}

// ProcessBirth reads local kernel process ownership. It neither adopts a process
// nor grants execution/termination permission. Unavailable identity fails closed.
func ProcessBirth(pid int) (Birth, error) { return processBirth(pid) }
