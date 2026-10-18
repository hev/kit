package nativejob

import (
	"context"
	"time"
)

const OwnerProofVersion = "native-owner-proof/v1"

// OwnerProof is original-journal metadata, not liveness, tree termination or
// permission to adopt a PID/replay/release capacity. Missing JobID authenticates
// only enrollment; all job facts remain absent. Arguments/environment are never
// part of this projection.
type OwnerProof struct {
	Version       string          `json:"version"`
	Enrollment    Enrollment      `json:"enrollment"`
	JobID         string          `json:"job_id,omitempty"`
	IntentKey     string          `json:"intent_key,omitempty"`
	PayloadDigest string          `json:"payload_digest,omitempty"`
	RequestDigest string          `json:"request_digest,omitempty"`
	Identity      *Identity       `json:"identity,omitempty"`
	Authority     *StartAuthority `json:"start_authority,omitempty"`
	State         string          `json:"state,omitempty"`
	StartReceipt  *StartReceipt   `json:"start_receipt,omitempty"`
	Completion    *Event          `json:"completion,omitempty"`
	Acknowledged  *bool           `json:"acknowledged,omitempty"`
	OwnerBirth    *Birth          `json:"owner_birth,omitempty"`
	ChildBirth    *Birth          `json:"child_birth,omitempty"`
	ObservedAt    time.Time       `json:"observed_at"`
}

// ReadOnlyProof never calls Open, initializes storage, runs recovery, authenticates
// a process tree or changes original job/fence/result state.
func ReadOnlyProof(ctx context.Context, root string, enrollment Enrollment, id string) (OwnerProof, error) {
	var out OwnerProof
	j, e := ReadOnlyOriginal(ctx, root, enrollment, id)
	if e != nil {
		return out, e
	}
	out = OwnerProof{Version: OwnerProofVersion, Enrollment: enrollment}
	if id != "" {
		requestDigest := RequestDigest(j.Request)
		if j.ID != id || j.Request.PayloadDigest != requestDigest {
			return OwnerProof{}, ErrConflict
		}
		out.JobID = j.ID
		out.IntentKey = j.Request.IntentKey
		out.PayloadDigest = j.Request.PayloadDigest
		out.RequestDigest = requestDigest
		out.Identity = &j.Request.Identity
		out.Authority = j.Request.StartAuthority
		out.State = j.State
		out.StartReceipt = j.StartReceipt
		out.Acknowledged = &j.Acknowledged
		out.OwnerBirth = j.Owner
		out.ChildBirth = j.Birth
		if j.State == "Completed" {
			if j.Result == nil || j.EventID == "" || Digest(*j.Result) != j.ResultDigest {
				return OwnerProof{}, ErrConflict
			}
			out.Completion = &Event{Version: Version, EventID: j.EventID, JobID: j.ID, Identity: j.Request.Identity, Result: *j.Result, ResultDigest: j.ResultDigest, StartReceipt: j.StartReceipt}
		}
	}
	if e = ctx.Err(); e != nil {
		return OwnerProof{}, e
	}
	out.ObservedAt = time.Now().UTC()
	return out, nil
}
