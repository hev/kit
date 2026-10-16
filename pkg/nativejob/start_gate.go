package nativejob

import (
	"context"
	"errors"
)

const StartVersion = "native-start/v1"

var ErrStartGate = errors.New("native final start admission held")

// StartAuthority is immutable execution admission, separate from the chef fence.
// Incarnation and generation must come from the authoritative enrolled node, not
// a cached controller observation. AttemptAuthority binds the reviewed attempt.
type StartAuthority struct {
	Version             string `json:"version"`
	NodeUID             string `json:"node_uid"`
	Incarnation         string `json:"incarnation"`
	AdmissionGeneration uint64 `json:"admission_generation"`
	ReservationID       string `json:"reservation_id"`
	AttemptAuthority    string `json:"attempt_authority"`
}

func (a StartAuthority) valid() bool {
	return a.Version == StartVersion && a.NodeUID != "" && a.Incarnation != "" && a.AdmissionGeneration > 0 && a.ReservationID != "" && a.AttemptAuthority != ""
}

type StartCheck struct {
	Version string  `json:"version"`
	JobID   string  `json:"job_id"`
	Request Request `json:"request"`
}

// StartReceipt records the authoritative decision before spawn. Receipt fields
// are evidence, not a portable bearer token or permission to replay execution.
type StartReceipt struct {
	Version           string         `json:"version"`
	JobID             string         `json:"job_id"`
	PayloadDigest     string         `json:"payload_digest"`
	Authority         StartAuthority `json:"authority"`
	DecisionID        string         `json:"decision_id"`
	AuthorityRevision string         `json:"authority_revision"`
}

// FinalStartGate must validate the exact request/reservation/attempt against the
// authoritative node incarnation, admission generation and cordon/drain state.
// It must hold the SAME exclusion used by cordon/drain across invoking start
// and its return. A successful enqueue or a detached permit is insufficient.
// Invoke start once synchronously; return its error without swallowing it. Never
// retain the callback or call this native Store from another gate operation.
// The callback fsyncs the receipt, starts the child and commits Running. It does
// not wait for child completion. Transport/authority failure must fail closed.
type FinalStartGate interface {
	WithStart(context.Context, StartCheck, func(StartReceipt) error) error
}
