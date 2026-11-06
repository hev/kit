package controlstate

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// NativeStartScope is the immutable canonical snapshot captured with an assessed
// start decision. The database binds its selected part to a separately enrolled
// original native job. It contains no executable payload or native fence.
type NativeStartScope struct {
	OrderUID    string `json:"order_uid"`
	Generation  int64  `json:"generation"`
	ScopeDigest string `json:"scope_digest"`
}

// BindNativeStart derives the native binding from the persisted start choice
// and the trusted pre-enrolled part map, in the decision transaction. A caller
// cannot supply a replacement job, request digest, native issuance or argv.
func (s *Store) BindNativeStart(ctx context.Context, action string) error {
	if s.pool != nil {
		return ErrInvalidState
	}
	return s.call(ctx, "SELECT kitchen.bind_native_start($1)", action)
}

// NativeStartPart is a trusted, pre-enrolled canonical execution part. The
// receiver independently authenticates the retained original request/issuance.
type NativeStartPart struct {
	Order         string `json:"order_id"`
	Part          string `json:"part"`
	Lease         string `json:"lease_name"`
	OrderUID      string `json:"order_uid"`
	Generation    int64  `json:"generation"`
	ScopeDigest   string `json:"scope_digest"`
	JobID         string `json:"native_job_id"`
	RequestDigest string `json:"request_digest"`
}

type NativeStartBinding struct {
	NativeStartPart
	ActionID     string `json:"action_id"`
	IntentDigest string `json:"intent_digest"`
}

// NativeStartParts never accepts a model-selected request or looks up a current
// native fence. The bounded list comes only from the trusted enrollment table.
func (s *Store) NativeStartParts(ctx context.Context, snap ControllerSnapshot) ([]NativeStartPart, error) {
	raw, err := scalar[json.RawMessage](ctx, s, `SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY part),'[]'::jsonb)
 FROM (SELECT order_id,part,lease_name,order_uid,generation,scope_digest,native_job_id,request_digest
 FROM kitchen.native_start_enrollments WHERE order_id=$1 AND order_uid=$2 AND generation=$3
 AND scope_digest=$4 AND lease_name=$5 ORDER BY part LIMIT 17) p`, snap.Order, snap.UID, snap.Generation, snap.Scope, ControllerLease(snap.Order))
	if err != nil {
		return nil, err
	}
	var parts []NativeStartPart
	if len(raw) > 16384 || json.Unmarshal(raw, &parts) != nil || len(parts) > 16 {
		return nil, ErrContextLimit
	}
	return parts, nil
}

func (s *Store) NativeStartBinding(ctx context.Context, action string) (NativeStartBinding, error) {
	var binding NativeStartBinding
	raw, err := scalar[json.RawMessage](ctx, s, `SELECT to_jsonb(b) FROM kitchen.native_start_bindings b WHERE action_id=$1`, action)
	if err != nil {
		return binding, err
	}
	if len(raw) == 0 || len(raw) > 16384 || json.Unmarshal(raw, &binding) != nil || binding.ActionID != action {
		return binding, ErrConflict
	}
	return binding, nil
}

// NativeStartRuntime is an additional opt-in check. Legacy controller status and
// held decisions require only their existing schema and privilege contract.
func (s *Store) NativeStartRuntime(ctx context.Context) error {
	ok, err := scalar[bool](ctx, s, `SELECT
 EXISTS(SELECT 1 FROM kitchen.schema_migrations WHERE version=3)
 AND NOT has_function_privilege(current_user,'kitchen.enroll_native_start(text,text,text,uuid,bigint,text,text,text)','EXECUTE')
 AND has_function_privilege(current_user,'kitchen.bind_native_start(uuid)','EXECUTE')
 AND has_function_privilege(current_user,'kitchen.lock_native_start(text,uuid,bigint,uuid,uuid,integer,bigint,uuid,bigint,text,text,text)','EXECUTE')`)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidState
	}
	return nil
}

// NativeStartWindow uses database clock values; locks never freeze expiration.
type NativeStartWindow struct{ ServerNow, ValidUntil time.Time }

func (s *Store) LockNativeStart(ctx context.Context, lease Lease, action, claimant string, attempt int, claimFence int64, scope NativeStartScope, job, digest string) (NativeStartWindow, error) {
	var window NativeStartWindow
	if s.pool != nil {
		return window, ErrInvalidState
	}
	err := s.db.QueryRow(ctx, `SELECT server_now,valid_until FROM kitchen.lock_native_start($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, pgx.QueryExecModeExec,
		lease.Name, lease.Holder, lease.Fence, action, claimant, attempt, claimFence, scope.OrderUID, scope.Generation, scope.ScopeDigest, job, digest).Scan(&window.ServerNow, &window.ValidUntil)
	if err != nil {
		return window, safe(err)
	}
	if window.ServerNow.IsZero() || !window.ValidUntil.After(window.ServerNow) {
		return window, ErrStaleFence
	}
	return window, nil
}

func (s *Store) NativeStartLimits(ctx context.Context) error {
	if s.pool != nil {
		return ErrInvalidState
	}
	return s.call(ctx, `SELECT set_config('lock_timeout','2000ms',true),set_config('statement_timeout','3000ms',true),set_config('idle_in_transaction_session_timeout','10000ms',true)`)
}
