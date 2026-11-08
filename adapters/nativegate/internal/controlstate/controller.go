package controlstate

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
)

const ControllerConsumer = "kitchen-controller/v1"
const ControllerDecisionKind = "controller-decision/v1"

var ErrContextLimit = errors.New("controller context exceeds budget; narrow durable source context")
var ErrControllerBusy = errors.New("controller step already active for this order")

// ControllerSnapshot is a bounded current-context projection, never a history dump.
// Cursor is per order; PostgreSQL allocates order event sequences under its row lock.
type ControllerSnapshot struct {
	Order      string            `json:"order"`
	UID        string            `json:"uid"`
	Generation int64             `json:"generation"`
	Scope      string            `json:"scope"`
	State      string            `json:"state"`
	Spec       json.RawMessage   `json:"spec"`
	Cursor     int64             `json:"cursor"`
	Events     []ControllerEvent `json:"events"`
	Pending    *ControllerAction `json:"pending,omitempty"`
	Lease      json.RawMessage   `json:"lease"`
}
type ControllerEvent struct {
	Seq     int64           `json:"seq"`
	Kind    string          `json:"kind"`
	Key     string          `json:"key"`
	Payload json.RawMessage `json:"payload"`
}
type ControllerAction struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
}

func ControllerLease(order string) string { return ControllerConsumer + ":" + order }

// ControllerRuntime requires both migrations and a function-only runtime role.
// Enrollment is deliberately not performed or made available by this path.
func (s *Store) ControllerRuntime(ctx context.Context, peer string) error {
	ok, err := scalar[bool](ctx, s, `SELECT
 (SELECT count(*)=2 FROM kitchen.schema_migrations WHERE version IN (1,2))
 AND NOT has_function_privilege(current_user,'kitchen.enroll_peer(text,text,text)','EXECUTE')
 AND NOT has_schema_privilege(current_user,'kitchen','CREATE')
 -- Effective ACL checks include inherited/PUBLIC and column-only grants.
 -- Relation OIDs avoid name/search_path ambiguity and include writable views
 -- and foreign/partitioned tables, not only pg_tables entries.
 AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_class c
   JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='kitchen' AND c.relkind IN ('r','p','v','m','f')
   AND (has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
     OR has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES')))
 -- USAGE permits nextval; UPDATE permits nextval/setval. SELECT alone is
 -- observational and does not permit either sequence mutation.
 AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_class c
   JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='kitchen' AND c.relkind='S'
   AND has_sequence_privilege(current_user,c.oid,'USAGE,UPDATE'))
 AND EXISTS (SELECT 1 FROM kitchen.peers WHERE peer_id::text=$1 AND kind='chef' AND revoked_at IS NULL)`, peer)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("controller requires schema 0001+0002, restricted runtime role and enrolled active chef peer")
	}
	return nil
}

// LockController serializes even two invocations using the same enrolled peer.
// Transaction-scoped advisory ownership is bounded by caller cancellation and
// automatically disappears on crash. It is not a second lease or scheduler.
func (s *Store) LockController(ctx context.Context, order string) error {
	if s.pool != nil {
		return errors.New("controller lock requires transaction")
	}
	h := sha256.Sum256([]byte(ControllerLease(order)))
	ok, err := scalar[bool](ctx, s, "SELECT pg_try_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(h[:8])))
	if err != nil {
		return err
	}
	if !ok {
		return ErrControllerBusy
	}
	return nil
}

func (s *Store) ControllerSnapshot(ctx context.Context, order string) (ControllerSnapshot, error) {
	var out ControllerSnapshot
	raw, err := scalar[json.RawMessage](ctx, s, `WITH current_order AS (
 SELECT order_id,uid,generation,scope_digest,state,spec FROM kitchen.orders WHERE order_id=$1
 ), c AS (SELECT coalesce((SELECT position FROM kitchen.cursors WHERE consumer=$2 AND stream=$1),0) AS position),
 e AS (SELECT seq,kind,dedupe_key AS key,CASE WHEN kind='controller-decision/v1' THEN '{}'::jsonb ELSE payload END AS payload FROM kitchen.order_events,c
 WHERE order_id=$1 AND seq>c.position ORDER BY seq LIMIT 32),
 snapshot AS (SELECT jsonb_build_object('order',o.order_id,'uid',o.uid,'generation',o.generation,
 'scope',o.scope_digest,'state',o.state,'spec',o.spec,'cursor',c.position,
 'events',coalesce((SELECT jsonb_agg(to_jsonb(e) ORDER BY seq) FROM e),'[]'::jsonb),
 'pending',(SELECT jsonb_build_object('id',action_id,'kind',kind,'key',idempotency_key,'state',state,'attempts',attempts)
 FROM kitchen.actions WHERE order_id=$1 AND state NOT IN ('done','reconciled','cancelled') ORDER BY created_at,action_id LIMIT 1),
 'lease',(SELECT jsonb_build_object('holder',holder,'fence',fence,'expires',expires_at::text,'active',expires_at>clock_timestamp())
 FROM kitchen.leases WHERE name=$3)) AS body FROM current_order o,c)
 SELECT CASE WHEN octet_length(body::text)<=24576 THEN body ELSE NULL END FROM snapshot`, order, ControllerConsumer, ControllerLease(order))
	if err != nil {
		return out, err
	}
	if len(raw) == 0 {
		return out, ErrContextLimit
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

// RecordControllerDecision uses the existing append_event row lock before
// checking the captured generation/state. All writes roll back on mismatch.
// The caller's transaction also owns the DB lease and advisory lock. No effects
// occur here. The cursor covers captured input only, never later arrivals.
func (s *Store) RecordControllerDecision(ctx context.Context, snap ControllerSnapshot, next int64, key string, receipt json.RawMessage, intent json.RawMessage, l Lease) (string, error) {
	if s.pool != nil {
		return "", errors.New("controller decision requires transaction")
	}
	if err := s.CheckFence(ctx, l); err != nil {
		return "", err
	}
	// append_event locks the order through the trusted API without granting runtime
	// table UPDATE privileges. A failed post-lock generation check rolls it back.
	if _, err := s.AppendEvent(ctx, snap.Order, key, ControllerDecisionKind, receipt, &l.Holder); err != nil {
		return "", err
	}
	valid, err := scalar[bool](ctx, s, `SELECT uid::text=$2 AND generation=$3 AND scope_digest=$4 AND state='open'
        FROM kitchen.orders WHERE order_id=$1`, snap.Order, snap.UID, snap.Generation, snap.Scope)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", ErrStaleGeneration
	}
	conflict, err := scalar[bool](ctx, s, `SELECT EXISTS(SELECT 1 FROM kitchen.actions WHERE order_id=$1
      AND state NOT IN ('done','reconciled','cancelled')) OR
      coalesce((SELECT position FROM kitchen.cursors WHERE consumer=$2 AND stream=$1),0)<>$3`, snap.Order, ControllerConsumer, snap.Cursor)
	if err != nil {
		return "", err
	}
	if conflict {
		return "", ErrConflict
	}
	id, err := s.IntendAction(ctx, snap.Order, ControllerDecisionKind, key, intent, nil, l)
	if err != nil {
		return "", err
	}
	ok, err := s.AdvanceCursor(ctx, ControllerConsumer, snap.Order, snap.Cursor, next, &l)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrConflict
	}
	// Recheck real server time immediately before handing control to COMMIT.
	if err = s.CheckFence(ctx, l); err != nil {
		return "", err
	}
	return id, nil
}
