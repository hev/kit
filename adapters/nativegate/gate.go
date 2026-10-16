// Package nativegate maps the public Factory native-admission facade onto the
// nativejob final-spawn boundary. It never queries the native journal in a gate,
// schedules work, submits jobs, adopts old fences or waits for a process.
package nativegate

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/hev/factory/pkg/nativeadmission"
	"github.com/hev/kit/pkg/nativejob"
)

// Facade is the accepted public Factory API; no Factory internal type escapes
// into this package. A facade must hold its authoritative guards through launch.
type Facade interface {
	WithStart(context.Context, nativeadmission.SpawnIntent, nativeadmission.Check, func(nativeadmission.Receipt) (nativeadmission.Started, error)) (nativeadmission.Started, error)
}

// Provenance comes from the authenticated receiver/fence/enrollment owner, not
// from a job notification. Retain it durably across restart. JournalReference is
// an owner-provided durable journal URI, not a newly invented execution permit.
// Authority.Authorize must verify workspace/parent and original-fence provenance.
type Provenance struct {
	Workspace        nativeadmission.Workspace
	Parent           nativeadmission.Parent
	OriginalFence    nativejob.Fence
	JournalReference string
}

type Gate struct {
	facade     Facade
	spawn      nativeadmission.SpawnIntent
	provenance Provenance
	reference  url.URL
}

var _ nativejob.FinalStartGate = (*Gate)(nil)

// MapCheck verifies the original native wire digest, then copies DTO fields
// explicitly. The facade uses a different JSON shape/hash for its action; it is
// never a replacement for the original RequestDigest or immutable native tuple.
func MapCheck(c nativejob.StartCheck, p Provenance) (nativeadmission.Check, error) {
	var out nativeadmission.Check
	r := c.Request
	i := r.Identity
	a := r.StartAuthority
	if c.Version != nativejob.StartVersion || r.Version != nativejob.Version || c.JobID == "" || r.IntentKey == "" || r.PayloadDigest != nativejob.RequestDigest(r) || a == nil || a.Version != nativejob.StartVersion {
		return out, nativeadmission.ErrBinding
	}
	f := p.OriginalFence
	if f.OrderUID != i.OrderUID || f.ChefID != i.ChefID || f.Epoch != i.Fence || f.Expires.IsZero() {
		return out, fmt.Errorf("original authenticated native fence required: %w", nativeadmission.ErrBinding)
	}
	out = nativeadmission.Check{Version: c.Version, JobID: c.JobID, IntentKey: r.IntentKey, PayloadDigest: r.PayloadDigest,
		Identity:  nativeadmission.Identity{OrderUID: i.OrderUID, ScopeDigest: i.ScopeDigest, PlanDigest: i.PlanDigest, AuthorityDigest: i.AuthorityDigest, ExecutionEpoch: i.ExecutionEpoch, WorkflowUID: i.WorkflowUID, NodeID: i.NodeID, StepID: i.StepID, AttemptID: i.AttemptID, CandidateDigest: i.CandidateDigest, InputDigest: i.InputDigest, EnvironmentDigest: i.EnvironmentDigest, Operation: i.Operation, HostID: i.HostID, ExecutorID: i.ExecutorID, ChefID: i.ChefID, Fence: i.Fence},
		Authority: nativeadmission.StartAuthority{Version: a.Version, NodeUID: a.NodeUID, Incarnation: a.Incarnation, AdmissionGeneration: a.AdmissionGeneration, ReservationID: a.ReservationID, AttemptAuthority: a.AttemptAuthority},
		Workspace: p.Workspace, Parent: p.Parent, NativeFenceExpires: f.Expires}
	return out, nil
}

// New binds an already reviewed SpawnIntent to its receiver context. DecideSpawn
// and all native queries happen BEFORE ExecuteWithGate, never from this adapter.
func New(f Facade, spawn nativeadmission.SpawnIntent, p Provenance) (*Gate, error) {
	if f == nil || (reflect.ValueOf(f).Kind() == reflect.Pointer && reflect.ValueOf(f).IsNil()) || spawn.AttentionID == "" || spawn.Revision == "" {
		return nil, nativeadmission.ErrBinding
	}
	if p.OriginalFence.ChefID != spawn.Lease.Owner || p.OriginalFence.Epoch != spawn.Lease.Fence || p.OriginalFence.Expires.IsZero() || !time.Now().Before(p.OriginalFence.Expires) {
		return nil, nativeadmission.ErrUnsupportedPendingHandoff
	}
	ref, err := url.Parse(p.JournalReference)
	if err != nil || ref.Scheme == "" || ref.User != nil || ref.RawQuery != "" || ref.Fragment != "" || len(p.JournalReference) > 1024 || strings.ContainsAny(p.JournalReference, "\r\n\x00") {
		return nil, fmt.Errorf("durable owner journal reference required: %w", nativeadmission.ErrBinding)
	}
	return &Gate{facade: f, spawn: spawn, provenance: p, reference: *ref}, nil
}

func (g *Gate) WithStart(ctx context.Context, c nativejob.StartCheck, start func(nativejob.StartReceipt) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if start == nil {
		return nativeadmission.ErrBinding
	}
	check, err := MapCheck(c, g.provenance)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	active, called, violated := true, false, false
	var launchErr error
	var expected nativeadmission.Started
	got, facadeErr := g.facade.WithStart(ctx, g.spawn, check, func(r nativeadmission.Receipt) (nativeadmission.Started, error) {
		mu.Lock()
		defer mu.Unlock()
		if !active || called {
			violated = true
			return nativeadmission.Started{}, nativeadmission.ErrBinding
		}
		called = true
		if r.Version != nativejob.StartVersion || r.JobID != check.JobID || r.PayloadDigest != check.PayloadDigest || r.Authority != check.Authority || r.DecisionID != g.spawn.AttentionID || r.AuthorityRevision == "" {
			launchErr = nativeadmission.ErrBinding
			return nativeadmission.Started{}, launchErr
		}
		a := r.Authority
		receipt := nativejob.StartReceipt{Version: r.Version, JobID: r.JobID, PayloadDigest: r.PayloadDigest, Authority: nativejob.StartAuthority{Version: a.Version, NodeUID: a.NodeUID, Incarnation: a.Incarnation, AdmissionGeneration: a.AdmissionGeneration, ReservationID: a.ReservationID, AttemptAuthority: a.AttemptAuthority}, DecisionID: r.DecisionID, AuthorityRevision: r.AuthorityRevision}
		if err := ctx.Err(); err != nil {
			launchErr = err
			return nativeadmission.Started{}, err
		}
		ref := g.reference
		ref.Fragment = url.Values{"job_id": {r.JobID}, "payload_digest": {r.PayloadDigest}, "decision_id": {r.DecisionID}, "authority_revision": {r.AuthorityRevision}}.Encode()
		if len(ref.String()) > 2048 {
			launchErr = nativeadmission.ErrBinding
			return nativeadmission.Started{}, launchErr
		}
		// The accepted callback commits receipt, actual Start and Running before nil.
		// It does not guarantee the child is still alive or always has observed birth.
		if launchErr = start(receipt); launchErr != nil {
			return nativeadmission.Started{}, launchErr
		}
		expected = nativeadmission.Started{State: "NativeSupervisorStarted", JobID: r.JobID, PayloadDigest: r.PayloadDigest, DecisionID: r.DecisionID, AuthorityRevision: r.AuthorityRevision, Evidence: ref.String()}
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
		return nativeadmission.ErrBinding
	}
	return nil
}
