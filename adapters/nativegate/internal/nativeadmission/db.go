package nativeadmission

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/hev/kit/adapters/nativegate/internal/controlstate"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBClaim preserves the database lease and action claim as separate tuples.
// Neither numeric fence is a native receiver fence.
type DBClaim struct {
	Lease      string `json:"lease"`
	Holder     string `json:"holder"`
	LeaseFence int64  `json:"lease_fence"`
	ActionID   string `json:"action_id"`
	Claimant   string `json:"claimant"`
	Attempt    int    `json:"attempt"`
	ClaimFence int64  `json:"claim_fence"`
}

// NativeBinding identifies the original native request authorized by the
// database's trusted canonical part enrollment. It cannot carry executable data.
type NativeBinding struct {
	OrderUID      string `json:"order_uid"`
	Generation    int64  `json:"generation"`
	ScopeDigest   string `json:"scope_digest"`
	JobID         string `json:"native_job_id"`
	RequestDigest string `json:"request_digest"`
}

type DBConfig struct {
	Pool                        *pgxpool.Pool
	BusinessDir, MaintenanceDir string
	MaintenanceOwner            Lease
	Authority                   Authority
}

type DBGate struct{ config DBConfig }

// NewDB selects an explicit connection and existing admission journals. It
// creates no schema, enrollment, chef journal, decision or native issuance.
func NewDB(c DBConfig) (*DBGate, error) {
	if c.Pool == nil || c.Authority == nil {
		return nil, errors.New("explicit database pool and canonical native authority required")
	}
	for _, p := range []string{c.BusinessDir, c.MaintenanceDir} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return nil, errors.New("explicit canonical absolute admission directories required")
		}
	}
	return &DBGate{config: c}, nil
}

// DBLaunch runs inside the receiver's final gate. finalCheck MUST run after
// durable start-receipt persistence, immediately before actual cmd.Start. It
// revalidates the SAME transaction's live DB lease/claim and original binding.
// Return only after the native callback commits Running; never wait for the child.
type DBLaunch func(Receipt, func() error) (Started, error)

// WithDBStart holds the exact DB lease/order/action/binding exclusion across
// existing business and maintenance admission and the synchronous native start.
// The same SQL guard revalidates server-clock expiry after receipt persistence.
func (g *DBGate) WithDBStart(parent context.Context, claim DBClaim, binding NativeBinding, check Check, launch DBLaunch) (Started, error) {
	var ack Started
	if g == nil || launch == nil || claim.Lease == "" || claim.Holder == "" || claim.ActionID == "" || claim.Claimant != claim.Holder || claim.Attempt < 1 || claim.LeaseFence < 1 || claim.ClaimFence != claim.LeaseFence || binding.Generation < 1 {
		return ack, ErrBinding
	}
	if err := valid(check); err != nil {
		return ack, err
	}
	if binding.OrderUID != check.Identity.OrderUID || binding.ScopeDigest != check.Identity.ScopeDigest || binding.JobID != check.JobID || binding.RequestDigest != check.PayloadDigest {
		return ack, ErrBinding
	}
	if !time.Now().Before(check.NativeFenceExpires) {
		return ack, ErrUnsupportedPendingHandoff
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	store, err := controlstate.New(ctx, g.config.Pool)
	if err != nil {
		return ack, err
	}
	if err := store.ControllerRuntime(ctx, claim.Holder); err != nil {
		return ack, err
	}
	if err := store.NativeStartRuntime(ctx); err != nil {
		return ack, err
	}
	admission := Gate{config: Config{BusinessDir: g.config.BusinessDir, MaintenanceDir: g.config.MaintenanceDir, MaintenanceOwner: g.config.MaintenanceOwner, Authority: g.config.Authority}}
	err = store.Transaction(ctx, func(tx *controlstate.Store) error {
		if err := tx.NativeStartLimits(ctx); err != nil {
			return err
		}
		recheck := func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !time.Now().Before(check.NativeFenceExpires) {
				return ErrUnsupportedPendingHandoff
			}
			_, err := tx.LockNativeStart(ctx, controlstate.Lease{Name: claim.Lease, Holder: claim.Holder, Fence: claim.LeaseFence}, claim.ActionID, claim.Claimant, claim.Attempt, claim.ClaimFence, controlstate.NativeStartScope{OrderUID: binding.OrderUID, Generation: binding.Generation, ScopeDigest: binding.ScopeDigest}, binding.JobID, binding.RequestDigest)
			return err
		}
		if err := recheck(ctx); err != nil {
			return err
		}
		// These are authenticated by SQL as one stored start choice. Execution
		// reservations are checked independently by withAdmission.
		prefix := "control-state@" + hash(struct {
			Claim   DBClaim
			Binding NativeBinding
		}{claim, binding}) + ";"
		ack, err = admission.withAdmission(ctx, claim.ActionID, "admit-workflow", prefix, check, recheck, func(receipt Receipt) (Started, error) {
			var mu sync.Mutex
			active, used, violated := true, false, false
			var checked error
			finalCheck := func() error {
				mu.Lock()
				defer mu.Unlock()
				if !active || used {
					violated = true
					return ErrHeld
				}
				used = true
				checked = recheck(ctx)
				return checked
			}
			started, launchErr := launch(receipt, finalCheck)
			mu.Lock()
			defer mu.Unlock()
			active = false
			if launchErr != nil {
				return Started{}, launchErr
			}
			if !used || violated {
				return Started{}, ErrHeld
			}
			if checked != nil {
				return Started{}, checked
			}
			return started, nil
		})
		return err
	})
	if err != nil {
		return Started{}, err
	}
	return ack, nil
}

// DBReferences and DBResolver reuse the existing reference-only, verified-TLS
// connection contract. They do not grant enrollment or resolve ambient DSNs.
type DBReferences = controlstate.References
type DBResolver = controlstate.Resolver
type DBOPResolver = controlstate.OPResolver

// DBCachedResolver reads through the host's kit secrets daemon first.
type DBCachedResolver = controlstate.CachedResolver

// OpenDB resolves only explicitly supplied references with the maintained
// control-state connection implementation. The caller owns the returned pool.
func OpenDB(ctx context.Context, refs DBReferences, resolver DBResolver) (*pgxpool.Pool, error) {
	_, pool, err := controlstate.Open(ctx, refs, resolver)
	return pool, err
}
