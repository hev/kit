package controlstate

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type Lease struct {
	Name, Holder string
	Fence        int64
}

// AcquireLease returns nil when another live holder owns the lease. TTL is in seconds.
func (s *Store) AcquireLease(ctx context.Context, name, holder string, ttl int) (*Lease, error) {
	f, e := scalar[*int64](ctx, s, "SELECT kitchen.acquire_lease($1,$2,$3)", name, holder, ttl)
	if e != nil || f == nil {
		return nil, e
	}
	return &Lease{name, holder, *f}, nil
}
func (s *Store) RenewLease(ctx context.Context, l Lease, ttl int) (bool, error) {
	return scalar[bool](ctx, s, "SELECT kitchen.renew_lease($1,$2,$3,$4)", l.Name, l.Holder, l.Fence, ttl)
}
func (s *Store) ReleaseLease(ctx context.Context, l Lease) (bool, error) {
	return scalar[bool](ctx, s, "SELECT kitchen.release_lease($1,$2,$3)", l.Name, l.Holder, l.Fence)
}
func (s *Store) CheckFence(ctx context.Context, l Lease) error {
	return s.call(ctx, "SELECT kitchen.check_fence($1,$2,$3)", l.Name, l.Holder, l.Fence)
}
func (s *Store) PutOrder(ctx context.Context, id string, spec json.RawMessage, expect *int64) (int64, error) {
	return scalar[int64](ctx, s, "SELECT kitchen.put_order($1,$2,$3)", id, spec, expect)
}
func (s *Store) AppendEvent(ctx context.Context, order, key, kind string, payload json.RawMessage, peer *string) (int64, error) {
	return scalar[int64](ctx, s, "SELECT kitchen.append_event($1,$2,$3,$4,$5)", order, key, kind, payload, peer)
}
func (s *Store) AdvanceCursor(ctx context.Context, consumer, stream string, expect, next int64, l *Lease) (bool, error) {
	var name, holder, fence any
	if l != nil {
		name, holder, fence = l.Name, l.Holder, l.Fence
	}
	return scalar[bool](ctx, s, "SELECT kitchen.advance_cursor($1,$2,$3,$4,$5,$6,$7)", consumer, stream, expect, next, name, holder, fence)
}
func (s *Store) IntendAction(ctx context.Context, order, kind, key string, intent json.RawMessage, deadline *time.Time, l Lease) (string, error) {
	return scalar[string](ctx, s, "SELECT kitchen.intend_action($1,$2,$3,$4,$5,$6,$7,$8)::text", order, kind, key, intent, deadline, l.Name, l.Holder, l.Fence)
}

type ActionClaim struct {
	ID      string
	Attempt int
	Lease   Lease
}

// ClaimAction fetches the attempt in the same transaction as the claim. A
// nil claim means no due work. Never perform an effect before this commits.
func (s *Store) ClaimAction(ctx context.Context, l Lease, ttl int, order *string) (*ActionClaim, error) {
	// A data-modifying function's changes are not visible to the outer query's
	// snapshot. Read the updated row in a second command within one transaction.
	if s.pool != nil {
		var c *ActionClaim
		err := s.Transaction(ctx, func(tx *Store) error {
			var e error
			c, e = tx.ClaimAction(ctx, l, ttl, order)
			return e
		})
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	id, e := scalar[*string](ctx, s, "SELECT kitchen.claim_action($1,$2,$3,$4,$5)::text", l.Holder, l.Name, l.Fence, ttl, order)
	if e != nil || id == nil {
		return nil, e
	}
	n, e := scalar[int](ctx, s, "SELECT attempts FROM kitchen.actions WHERE action_id=$1", *id)
	if e != nil {
		return nil, e
	}
	return &ActionClaim{*id, n, l}, nil
}
func (s *Store) FinishAction(ctx context.Context, c ActionClaim, outcome string, effect *string, retrySeconds int) error {
	if c.Attempt < 1 {
		return ErrStaleFence
	}
	return s.call(ctx, "SELECT kitchen.finish_action($1,$2,$3,$4,$5,$6,$7)", c.ID, c.Lease.Holder, c.Lease.Fence, outcome, effect, retrySeconds, c.Attempt)
}
func (s *Store) SweepActions(ctx context.Context) (int, error) {
	return scalar[int](ctx, s, "SELECT kitchen.sweep_actions()")
}
func (s *Store) ReconcileAction(ctx context.Context, id string, happened bool, evidence string, effect *string) error {
	return s.call(ctx, "SELECT kitchen.reconcile_action($1,$2,$3,$4)", id, happened, evidence, effect)
}
func (s *Store) Heartbeat(ctx context.Context, peer string, ttl int, detail json.RawMessage) error {
	return s.call(ctx, "SELECT kitchen.heartbeat($1,$2,$3)", peer, ttl, detail)
}
func (s *Store) SendMessage(ctx context.Context, from, to string, order *string, key string, body json.RawMessage) (string, error) {
	return scalar[string](ctx, s, "SELECT kitchen.send_message($1,$2,$3,$4,$5)::text", from, to, order, key, body)
}
func (s *Store) AckMessage(ctx context.Context, id, peer string) (bool, error) {
	return scalar[bool](ctx, s, "SELECT kitchen.ack_message($1,$2)", id, peer)
}
func (s *Store) EnqueueOutbox(ctx context.Context, dest, key string, payload json.RawMessage, order *string) (int64, error) {
	return scalar[int64](ctx, s, "SELECT kitchen.enqueue_outbox($1,$2,$3,$4)", dest, key, payload, order)
}
func (s *Store) ClaimOutbox(ctx context.Context, dest string, ttl int) (*int64, error) {
	return scalar[*int64](ctx, s, "SELECT kitchen.claim_outbox($1,$2)", dest, ttl)
}
func (s *Store) FinishOutbox(ctx context.Context, id int64, effect string) error {
	return s.call(ctx, "SELECT kitchen.finish_outbox($1,$2)", id, effect)
}
func (s *Store) SweepOutbox(ctx context.Context) (int, error) {
	return scalar[int](ctx, s, "SELECT kitchen.sweep_outbox()")
}
func (s *Store) EnrollPeer(ctx context.Context, kind, label string, fingerprint *string) (string, error) {
	return scalar[string](ctx, s, "SELECT kitchen.enroll_peer($1,$2,$3)::text", kind, label, fingerprint)
}
func (s *Store) EnrollWorker(ctx context.Context, peer, harness string, model *string) (string, error) {
	return scalar[string](ctx, s, "SELECT kitchen.enroll_worker($1,$2,$3)::text", peer, harness, model)
}
func (s *Store) EnrollStation(ctx context.Context, name, host string, capabilities json.RawMessage) (string, error) {
	return scalar[string](ctx, s, "SELECT kitchen.enroll_station($1,$2,$3)::text", name, host, capabilities)
}
func (s *Store) EnrollObligation(ctx context.Context, id, order, condition string) error {
	return s.call(ctx, "SELECT kitchen.enroll_obligation($1,$2,$3)", id, order, condition)
}
func (s *Store) SubmitCandidate(ctx context.Context, id, candidate string) (int, error) {
	return scalar[int](ctx, s, "SELECT kitchen.submit_candidate($1,$2)", id, candidate)
}
func (s *Store) DecideCandidate(ctx context.Context, id, candidate, verdict, evidence string) error {
	return s.call(ctx, "SELECT kitchen.decide_candidate($1,$2,$3,$4)", id, candidate, verdict, evidence)
}

type Assignment struct {
	Assignment, Attempt string
	Fence               int64
}

func (s *Store) Assign(ctx context.Context, order, part string, station *string, worker string, ttl int, contextRef string) (*Assignment, error) {
	var a Assignment
	err := s.db.QueryRow(ctx, "SELECT assignment::text,attempt::text,fence FROM kitchen.assign($1,$2,$3,$4,$5,$6)", pgx.QueryExecModeExec, order, part, station, worker, ttl, contextRef).Scan(&a.Assignment, &a.Attempt, &a.Fence)
	if err != nil {
		return nil, safe(err)
	}
	return &a, nil
}
func (s *Store) ReportAttempt(ctx context.Context, id, worker string, fence int64, state string, result json.RawMessage) error {
	return s.call(ctx, "SELECT kitchen.report_attempt($1,$2,$3,$4,$5)", id, worker, fence, state, result)
}

// Read methods return bounded, ordered snapshots; JSON retains future additive fields.
func (s *Store) Events(ctx context.Context, order string, after int64, limit int) (json.RawMessage, error) {
	return scalar[json.RawMessage](ctx, s, "SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY seq),'[]'::jsonb) FROM (SELECT * FROM kitchen.order_events WHERE order_id=$1 AND seq>$2 ORDER BY seq LIMIT least(greatest($3,0),500)) e", order, after, limit)
}
func (s *Store) Inbox(ctx context.Context, peer string, after int64, limit int) (json.RawMessage, error) {
	return scalar[json.RawMessage](ctx, s, "SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY seq),'[]'::jsonb) FROM kitchen.inbox($1,$2,$3) m", peer, after, limit)
}
func (s *Store) Action(ctx context.Context, id string) (json.RawMessage, error) {
	return scalar[json.RawMessage](ctx, s, "SELECT to_jsonb(a) FROM kitchen.actions a WHERE action_id=$1", id)
}
func (s *Store) Outbox(ctx context.Context, id int64) (json.RawMessage, error) {
	return scalar[json.RawMessage](ctx, s, "SELECT to_jsonb(o) FROM kitchen.outbox o WHERE outbox_id=$1", id)
}
