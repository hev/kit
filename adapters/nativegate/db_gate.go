package nativegate

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

// DBFacade holds one database transaction and existing business/node/parent
// guards through its synchronous callback, never through native process Wait.
type DBFacade interface {
	WithDBStart(context.Context, a.DBClaim, a.NativeBinding, a.Check, a.DBLaunch) (a.Started, error)
}

type DBGate struct {
	facade     DBFacade
	claim      a.DBClaim
	binding    a.NativeBinding
	provenance Provenance
	reference  url.URL
}

var _ n.FinalSpawnGate = (*DBGate)(nil)

func NewDBGate(f DBFacade, claim a.DBClaim, binding a.NativeBinding, p Provenance) (*DBGate, error) {
	if f == nil || (reflect.ValueOf(f).Kind() == reflect.Pointer && reflect.ValueOf(f).IsNil()) || claim.Lease == "" || claim.Holder == "" || claim.LeaseFence < 1 || claim.ActionID == "" || claim.Claimant == "" || claim.Attempt < 1 || claim.ClaimFence < 1 || binding.OrderUID == "" || binding.Generation < 1 || binding.ScopeDigest == "" || binding.JobID == "" || binding.RequestDigest == "" {
		return nil, a.ErrBinding
	}
	ref, err := url.Parse(p.JournalReference)
	if err != nil || ref.Scheme == "" || ref.User != nil || ref.RawQuery != "" || ref.Fragment != "" || len(p.JournalReference) > 1024 || strings.ContainsAny(p.JournalReference, "\r\n\x00") {
		return nil, a.ErrBinding
	}
	return &DBGate{facade: f, claim: claim, binding: binding, provenance: p, reference: *ref}, nil
}

// WithStart refuses older native receivers that cannot invoke the final checker
// after durable receipt persistence. No detached preflight can replace WithSpawn.
func (g *DBGate) WithStart(context.Context, n.StartCheck, func(n.StartReceipt) error) error {
	return n.ErrStartGate
}

func (g *DBGate) WithSpawn(ctx context.Context, c n.StartCheck, start func(n.StartReceipt, func(context.Context) error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if start == nil {
		return a.ErrBinding
	}
	check, err := MapCheck(c, g.provenance)
	if err != nil {
		return err
	}
	if c.JobID != g.binding.JobID || c.Request.PayloadDigest != g.binding.RequestDigest || c.Request.Identity.OrderUID != g.binding.OrderUID || c.Request.Identity.ScopeDigest != g.binding.ScopeDigest {
		return a.ErrBinding
	}
	var mu sync.Mutex
	active, called, violated := true, false, false
	var launchErr error
	var expected a.Started
	got, facadeErr := g.facade.WithDBStart(ctx, g.claim, g.binding, check, func(r a.Receipt, finalCheck func() error) (a.Started, error) {
		mu.Lock()
		defer mu.Unlock()
		if !active || called {
			violated = true
			return a.Started{}, a.ErrBinding
		}
		called = true
		if finalCheck == nil || r.Version != n.StartVersion || r.JobID != check.JobID || r.PayloadDigest != check.PayloadDigest || r.Authority != check.Authority || r.DecisionID != g.claim.ActionID || r.AuthorityRevision == "" {
			launchErr = a.ErrBinding
			return a.Started{}, launchErr
		}
		ar := r.Authority
		receipt := n.StartReceipt{Version: r.Version, JobID: r.JobID, PayloadDigest: r.PayloadDigest, Authority: n.StartAuthority{Version: ar.Version, NodeUID: ar.NodeUID, Incarnation: ar.Incarnation, AdmissionGeneration: ar.AdmissionGeneration, ReservationID: ar.ReservationID, AttemptAuthority: ar.AttemptAuthority}, DecisionID: r.DecisionID, AuthorityRevision: r.AuthorityRevision}
		ref := g.reference
		ref.Fragment = url.Values{"job_id": {r.JobID}, "payload_digest": {r.PayloadDigest}, "decision_id": {r.DecisionID}, "authority_revision": {r.AuthorityRevision}}.Encode()
		if len(ref.String()) > 2048 {
			launchErr = a.ErrBinding
			return a.Started{}, launchErr
		}
		launchErr = start(receipt, func(finalCtx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := finalCtx.Err(); err != nil {
				return err
			}
			if err := finalCheck(); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			return finalCtx.Err()
		})
		if launchErr != nil {
			return a.Started{}, launchErr
		}
		expected = a.Started{State: "NativeSupervisorStarted", JobID: r.JobID, PayloadDigest: r.PayloadDigest, DecisionID: r.DecisionID, AuthorityRevision: r.AuthorityRevision, Evidence: ref.String()}
		return expected, nil
	})
	mu.Lock()
	defer mu.Unlock()
	active = false
	if launchErr != nil {
		return launchErr
	}
	if facadeErr != nil {
		return facadeErr
	}
	if !called || violated || expected.State == "" || got != expected {
		return a.ErrBinding
	}
	return nil
}

// Start executes only the existing exact Pending job. Original issuance is read
// from its enrolled native owner, independently of DB numeric fences. Queries
// happen before entering either gate; native launch rechecks issuance again.
// This synchronous call waits for process completion after all start guards are
// released. A caller timeout or lost response is uncertainty, never replay.
func Start(ctx context.Context, store *n.Store, f DBFacade, claim a.DBClaim, binding a.NativeBinding, p Provenance) (n.Job, error) {
	return startObserved(ctx, store, f, claim, binding, p, 0, nil)
}
func startObserved(ctx context.Context, store *n.Store, f DBFacade, claim a.DBClaim, binding a.NativeBinding, p Provenance, timeout time.Duration, observer func(n.StartedObservation)) (n.Job, error) {
	if store == nil {
		return n.Job{}, a.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return n.Job{}, err
	}
	j, err := store.Query(binding.JobID)
	if err != nil {
		return n.Job{}, err
	}
	if j.ID != binding.JobID || j.State != "Pending" || j.Request.StartAuthority == nil || j.Request.PayloadDigest != binding.RequestDigest || n.RequestDigest(j.Request) != binding.RequestDigest {
		return n.Job{}, a.ErrBinding
	}
	i := j.Request.Identity
	p.OriginalFence, err = store.IssuedFence(i.OrderUID, i.ChefID, i.Fence)
	if err != nil {
		return n.Job{}, err
	}
	gate, err := NewDBGate(f, claim, binding, p)
	if err != nil {
		return n.Job{}, err
	}
	if timeout > 0 {
		return store.ExecuteRetainedWithGate(ctx, j.ID, gate, n.RetainedExecutionOptions{WorkloadTimeout: timeout}, observer)
	}
	return store.ExecuteWithGate(ctx, j.ID, gate)
}
