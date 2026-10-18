package nativegate

import (
	"context"

	"github.com/hev/kit/pkg/nativejob"
)

const ReceiverVersion = "native-receiver/v1"

// Inspection contains original journal metadata, never executable arguments or
// environment. RequestDigest is recomputed from the complete original request;
// PayloadDigest is its separately retained declaration. Neither grants a start.
// Completion remains available after historical result acknowledgement.
type Inspection struct {
	Version       string                    `json:"version"`
	JobID         string                    `json:"job_id"`
	IntentKey     string                    `json:"intent_key"`
	PayloadDigest string                    `json:"payload_digest"`
	RequestDigest string                    `json:"request_digest"`
	Identity      nativejob.Identity        `json:"identity"`
	Authority     *nativejob.StartAuthority `json:"start_authority,omitempty"`
	State         string                    `json:"state"`
	StartReceipt  *nativejob.StartReceipt   `json:"start_receipt,omitempty"`
	Completion    *nativejob.Event          `json:"completion,omitempty"`
	Acknowledged  bool                      `json:"acknowledged"`
}

// Inspect reads one exact original job. It makes no admission decision, performs
// no recovery mutation and never infers absence of effects from an empty result.
func Inspect(ctx context.Context, store *nativejob.Store, id string) (Inspection, error) {
	var out Inspection
	if store == nil || id == "" {
		return out, nativejob.ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	j, err := store.Query(id)
	if err != nil {
		return out, err
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	digest := nativejob.RequestDigest(j.Request)
	if j.ID != id || digest != j.Request.PayloadDigest {
		return out, nativejob.ErrConflict
	}
	out = Inspection{Version: ReceiverVersion, JobID: j.ID, IntentKey: j.Request.IntentKey, PayloadDigest: j.Request.PayloadDigest, RequestDigest: digest, Identity: j.Request.Identity, Authority: j.Request.StartAuthority, State: j.State, StartReceipt: j.StartReceipt, Acknowledged: j.Acknowledged}
	if j.State == "Completed" {
		if j.Result == nil || j.EventID == "" || nativejob.Digest(*j.Result) != j.ResultDigest {
			return Inspection{}, nativejob.ErrConflict
		}
		out.Completion = &nativejob.Event{Version: nativejob.Version, EventID: j.EventID, JobID: j.ID, Identity: j.Request.Identity, Result: *j.Result, ResultDigest: j.ResultDigest, StartReceipt: j.StartReceipt}
	}
	return out, nil
}

// Acknowledge accepts only the exact original historical result tuple. It never
// acquires or renews execution authority and remains idempotent after fence loss.
func Acknowledge(ctx context.Context, store *nativejob.Store, id string, identity nativejob.Identity, eventID, digest string) error {
	if store == nil || id == "" || eventID == "" || digest == "" {
		return nativejob.ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Query rejects intent aliases before invoking the existing ACK SDK.
	j, err := store.Query(id)
	if err != nil {
		return err
	}
	if j.ID != id {
		return nativejob.ErrConflict
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return store.Ack(id, identity, eventID, digest)
}
